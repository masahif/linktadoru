// Package crawler provides the core web crawling functionality.
// It implements a concurrent, queue-based crawler with rate limiting,
// robots.txt compliance, and comprehensive page analysis capabilities.
package crawler

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/masahif/linktadoru/internal/config"
)

// DefaultCrawler implements the Crawler interface
type DefaultCrawler struct {
	config       *config.CrawlConfig
	storage      Storage
	httpClient   *HTTPClient
	processor    PageProcessor
	rateLimiter  *RateLimiter
	robotsParser *RobotsParser
	allowedHosts []string // Hosts allowed for crawling (from seed URLs)

	// Include/exclude patterns compiled once at construction. Compiling here
	// (a) rejects an invalid pattern at startup instead of silently never
	// matching it, and (b) avoids recompiling every pattern for every URL.
	includePatterns []*regexp.Regexp
	excludePatterns []*regexp.Regexp

	// State
	stats      CrawlStats
	statsMutex sync.RWMutex
	// layerDepth is the depth the current round of workers is allowed to claim
	// from, used only by a bounded crawl. It is written between rounds, while
	// no worker is running, and read by the workers of the round it starts.
	layerDepth int
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup // tracks workers only (not the stats reporter)
}

// isTransientResult reports whether a result is an HTTP response the server
// asked us to come back for, rather than an answer about the resource.
func (c *DefaultCrawler) isTransientResult(result *PageResult) bool {
	return result.Page != nil && result.Error != nil && isTransientStatus(result.Page.StatusCode)
}

// failAttempt records one failed attempt at a URL and schedules the next one.
//
// Every failure goes through here so the pacing cannot be forgotten at a call
// site. A retryable failure — a transient HTTP status, or a transport failure
// that produced no response at all — is given a wait: whatever the server
// asked for via Retry-After, or a deterministic backoff when it asked for
// nothing. Without that wait a struggling host is hit again within
// milliseconds and the whole attempt budget is gone before it could recover,
// which defeats the point of retrying. Failures that are never retried get the
// zero time, meaning no wait.
//
// page carries the observation when there was one, and is nil when no response
// arrived.
func (c *DefaultCrawler) failAttempt(id int, item *URLItem, page *PageData, errorType, errorMessage string) {
	var retryAt time.Time
	if isRetryableErrorType(errorType) {
		var headers map[string]string
		if page != nil {
			headers = page.HTTPHeaders
		}
		attempt := item.RetryCount + 1
		retryAt = time.Now().Add(retryDelay(headers, attempt, defaultRetryBackoff, time.Now()))
	}

	if err := c.storage.SaveFailedAttempt(id, page, errorType, errorMessage, retryAt); err != nil {
		slog.Error("Worker failed to record a failed attempt", "url", item.URL, "error", err)
	}
	c.incrementErrorCount()
}

// NewCrawler creates a new crawler instance with the provided configuration and storage.
// It initializes all necessary components including HTTP client, page processor,
// rate limiter, and robots.txt parser. The crawler is ready to start crawling
// after creation.
func NewCrawler(config *config.CrawlConfig, storage Storage) (*DefaultCrawler, error) {

	// Initialize HTTP client
	httpClient := NewHTTPClient(config.UserAgent, config.RequestTimeout)
	httpClient.SetMaxResponseSize(config.MaxResponseSize)

	// Configure basic authentication if provided
	if config.Auth != nil {
		switch string(config.Auth.Type) {
		case "basic":
			if username, password := config.GetBasicAuthCredentials(); username != "" && password != "" {
				httpClient.SetBasicAuth(username, password)
			}
		case "bearer":
			if token := config.GetBearerToken(); token != "" {
				httpClient.SetBearerAuth(token)
			}
		case "api-key":
			if header, value := config.GetAPIKeyCredentials(); header != "" && value != "" {
				httpClient.SetAPIKeyAuth(header, value)
			}
		}
	}

	// Set custom headers if provided
	if len(config.Headers) > 0 {
		headerMap := make(map[string]string)
		for _, header := range config.Headers {
			// Parse "Key: Value" format
			colonIndex := strings.Index(header, ":")
			if colonIndex <= 0 {
				// Skip invalid headers - validation should have caught this
				slog.Warn("Skipping invalid header format", "header", header)
				continue
			}

			key := strings.TrimSpace(header[:colonIndex])
			value := strings.TrimSpace(header[colonIndex+1:])

			if key == "" || value == "" {
				// Skip empty key or value
				slog.Warn("Skipping header with empty key or value", "header", header)
				continue
			}

			headerMap[key] = value
		}

		if len(headerMap) > 0 {
			httpClient.SetCustomHeaders(headerMap)
			slog.Info("Set custom headers", "count", len(headerMap))
		}
	}

	// Initialize components
	processor := NewPageProcessorWithConfig(httpClient, config.AllowedSchemes, config.FollowExternalHosts)
	rateLimiter := NewRateLimiter(time.Duration(config.RequestDelay * float64(time.Second)))
	robotsParser := NewRobotsParser(httpClient, config.IgnoreRobotsTxt)

	// Extract allowed hosts from seed URLs for same-host filtering
	allowedHosts := make([]string, 0, len(config.SeedURLs))
	for _, seedURL := range config.SeedURLs {
		if parsedURL, err := url.Parse(seedURL); err == nil {
			host := parsedURL.Scheme + "://" + parsedURL.Host
			// Avoid duplicates
			found := false
			for _, existing := range allowedHosts {
				if existing == host {
					found = true
					break
				}
			}
			if !found {
				allowedHosts = append(allowedHosts, host)
			}
		}
	}

	// Compile URL filter patterns up front so an invalid regex fails the run
	// loudly instead of being silently ignored on every URL.
	includePatterns, err := compilePatterns(config.IncludePatterns)
	if err != nil {
		return nil, fmt.Errorf("invalid include_patterns: %w", err)
	}
	excludePatterns, err := compilePatterns(config.ExcludePatterns)
	if err != nil {
		return nil, fmt.Errorf("invalid exclude_patterns: %w", err)
	}

	crawler := &DefaultCrawler{
		config:          config,
		storage:         storage,
		httpClient:      httpClient,
		processor:       processor,
		rateLimiter:     rateLimiter,
		robotsParser:    robotsParser,
		allowedHosts:    allowedHosts,
		includePatterns: includePatterns,
		excludePatterns: excludePatterns,
		stats: CrawlStats{
			StartTime: time.Now(),
		},
	}

	// Redirects must clear the same host filter as newly discovered URLs.
	// Checking only the initial URL lets a 302 walk the crawler onto a host it
	// was never allowed to reach.
	httpClient.SetRedirectPolicy(func(u *url.URL) bool {
		return crawler.isAllowedHost(u.String())
	})

	return crawler, nil
}

// compilePatterns compiles a list of regex patterns, reporting the offending
// pattern on failure.
func compilePatterns(patterns []string) ([]*regexp.Regexp, error) {
	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			return nil, fmt.Errorf("pattern %q: %w", p, err)
		}
		compiled = append(compiled, re)
	}
	return compiled, nil
}

// isAllowedHost checks if the given URL's host is allowed for crawling
func (c *DefaultCrawler) isAllowedHost(targetURL string) bool {
	// First check if URL has an allowed scheme
	if !c.isAllowedScheme(targetURL) {
		return false
	}

	// If external hosts are allowed, accept any valid scheme
	if c.config.FollowExternalHosts {
		return true
	}

	// Check if URL starts with any allowed host prefix
	for _, allowedHost := range c.allowedHosts {
		// Allow exact match or prefix with trailing slash
		if targetURL == allowedHost || strings.HasPrefix(targetURL, allowedHost+"/") {
			return true
		}
	}

	return false
}

// isAllowedScheme checks if the URL has an allowed scheme
func (c *DefaultCrawler) isAllowedScheme(targetURL string) bool {
	// Use configured allowed schemes, fallback to defaults if empty
	allowedSchemes := c.config.AllowedSchemes
	if len(allowedSchemes) == 0 {
		allowedSchemes = []string{"https://", "http://"}
	}

	for _, scheme := range allowedSchemes {
		if strings.HasPrefix(targetURL, scheme) {
			return true
		}
	}

	return false
}

// Start starts the crawling process
// Startup process:
//  1. Add seed URLs to queue with 'pending' status
//  2. Start configured number of workers
//  3. Workers compete for 'pending' items using atomic status updates
//  4. Continue until queue is empty or limits reached, then retry
//     transient failures once before returning
func (c *DefaultCrawler) Start(ctx context.Context, seedURLs []string) error {
	c.ctx, c.cancel = context.WithCancel(ctx)
	defer c.cancel()

	// Checked before anything is queued or fetched, so a database that cannot
	// honour the bound is rejected rather than half-crawled.
	if c.bounded() {
		if err := c.requireDepthTracking(); err != nil {
			return err
		}
	}

	// Reset rows left in 'processing' by a previous interrupted run back to
	// 'pending'. No workers are running yet, so every 'processing' row is stale.
	// This both re-queues interrupted URLs and prevents a stale 'processing' row
	// from keeping HasQueuedItems() perpetually true, which would otherwise stop
	// shouldExitOnEmptyQueue from ever firing and hang every worker (issue #46
	// review follow-up). Passing 0 treats all current 'processing' rows as stale.
	if err := c.storage.CleanupStaleProcessing(0); err != nil {
		if c.bounded() {
			// Fail closed. Those stale rows belong to a depth, and the layer
			// barrier decides what to open next from the state of the queue —
			// if that state could not be established, a shallow layer may look
			// finished when it is not, and the run would step past it.
			return fmt.Errorf("failed to reset stale processing rows: %w", err)
		}
		slog.Error("Failed to reset stale processing rows", "error", err)
	}

	if len(seedURLs) > 0 {
		slog.Info("Starting crawler", "seed_urls", len(seedURLs))

		// Every seed is queued. Limit caps how many pages are crawled, not how
		// many starting points the run is allowed to know about; truncating the
		// seed list here silently narrowed the crawl instead, which a long list
		// from --seed-file makes invisible.
		var err error
		if c.bounded() {
			err = c.storage.AddToQueueWithDepth(seedURLs, 0)
		} else {
			err = c.storage.AddToQueue(seedURLs)
		}
		if err != nil {
			return fmt.Errorf("failed to add seed URLs to queue: %w", err)
		}
		slog.Info("Added seed URLs to queue", "count", len(seedURLs))
	} else {
		slog.Info("Starting crawler - resuming from existing queue")
	}

	// The stats reporter lives outside the worker WaitGroup: it only exits on
	// context cancellation, so tracking it in c.wg would keep wg.Wait from
	// ever returning on natural completion. (An earlier design had the last
	// exiting worker cancel the context to stop the reporter — but that made
	// Start unable to tell natural completion from cancellation, so the retry
	// phase below was unreachable and retries never ran.)
	statsDone := make(chan struct{})
	go func() {
		defer close(statsDone)
		c.statsReporter()
	}()

	var runErr error
	if c.strictLayers() {
		runErr = c.runBoundedCrawl()
	} else {
		// Both the unbounded crawl and max_depth 1 come through here, differing
		// only in which rows a worker may claim (see nextQueueItem).
		//
		// Always wait for the workers themselves. On external cancellation they
		// exit promptly (in-flight requests carry c.ctx), and waiting here is what
		// makes shutdown graceful: Start does not return while a worker may still
		// be writing to the database.
		c.runWorkers()

		if c.ctx.Err() != nil {
			slog.Info("Crawling cancelled")
		} else {
			slog.Info("Crawling completed - checking for retries")
			if err := c.performRetries(); err != nil {
				slog.Error("Error during retry processing", "error", err)
			}
		}
	}

	// Report before the reporter is stopped, and before returning either way:
	// a run that ended badly is exactly when the breakdown matters.
	c.logRunSummary()

	// Stop the stats reporter and wait for it before returning.
	c.cancel()
	<-statsDone

	return runErr
}

// logRunSummary reports what became of every URL the run touched.
//
// The categories are kept apart on purpose. A page the server answered
// transiently until the retries ran out is unavailable — we do not know its
// current state — and that is a different fact from a page that returned 404,
// which is a real observation. A summary that merged them would let a snapshot
// comparison report an outage as a deletion.
func (c *DefaultCrawler) logRunSummary() {
	summary, err := c.storage.GetRunSummary()
	if err != nil {
		slog.Error("Failed to summarise the run", "error", err)
		return
	}

	slog.Info("Run summary",
		"completed", summary.Completed,
		"unavailable", summary.Unavailable,
		"unreachable", summary.Unreachable,
		"skipped", summary.Skipped,
		"unfinished", summary.Unfinished,
		"discovered_not_crawled", summary.Discovered)

	if summary.Unavailable > 0 || summary.Unreachable > 0 {
		slog.Warn("Some URLs could not be fetched - their current state is unknown, not absent",
			"unavailable", summary.Unavailable, "unreachable", summary.Unreachable)
	}
}

// performRetries retries failed pages once the normal queue has drained.
//
// It loops until nothing is retryable, rather than making a single pass. The
// attempt budget is MaxRetries per URL across the whole crawl of a database —
// retry_count is persisted, so a run cancelled mid-budget hands the remainder
// to the resume rather than starting over. What the loop changes is that an
// uninterrupted run spends the budget before returning instead of leaving
// attempts unmade: with a database created fresh per crawl there is no later
// run to spend them, and the transient failure they would have recovered from
// becomes indistinguishable from a page that is genuinely gone. Honouring a
// Retry-After also needs a next pass to happen at all.
func (c *DefaultCrawler) performRetries() error {
	const maxRetries = MaxRetries

	for {
		if c.ctx.Err() != nil {
			return nil
		}

		due, err := c.storage.EarliestRetryTime(maxRetries)
		if err != nil {
			return fmt.Errorf("failed to find pages for retry: %w", err)
		}
		if due == nil {
			slog.Info("No pages available for retry")
			return nil
		}
		if !c.waitUntil(*due) {
			return nil // cancelled while waiting
		}

		requeued, err := c.storage.RequeueErrorPages(maxRetries)
		if err != nil {
			return fmt.Errorf("failed to requeue error pages: %w", err)
		}
		if requeued == 0 {
			// The wait above means the rows were due, so a requeue that moves
			// nothing is a real disagreement between the two queries rather
			// than a timing artefact. Looping would spin.
			return fmt.Errorf("pages are reported as retryable but none could be requeued; refusing to continue")
		}

		slog.Info("Requeued error pages for retry", "count", requeued)
		c.runWorkers()
	}
}

// waitUntil blocks until t, returning false if the run is cancelled first.
// A time in the past returns immediately.
func (c *DefaultCrawler) waitUntil(t time.Time) bool {
	wait := time.Until(t)
	if wait <= 0 {
		return c.ctx.Err() == nil
	}

	slog.Info("Waiting before the next retry attempt", "wait", wait)
	timer := time.NewTimer(wait)
	defer timer.Stop()

	select {
	case <-c.ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// Stop stops the crawling process
func (c *DefaultCrawler) Stop() error {
	if c.cancel != nil {
		c.cancel()
	}
	c.httpClient.Close()
	return nil
}

// GetStats returns current crawling statistics
func (c *DefaultCrawler) GetStats() CrawlStats {
	c.statsMutex.RLock()
	defer c.statsMutex.RUnlock()

	stats := c.stats
	stats.Duration = time.Since(stats.StartTime)
	return stats
}

// worker processes URLs from the queue
// Termination conditions:
// 1. Context cancelled (graceful shutdown)
// 2. Reached configured limit of pages
// 3. No queued items available (SELECT returns empty result)
func (c *DefaultCrawler) worker(id int) {
	defer c.wg.Done()
	defer slog.Debug("Worker stopped", "worker_id", id)

	slog.Debug("Worker started", "worker_id", id)

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
			if c.shouldStopWorker(id) {
				return
			}

			item, err := c.nextQueueItem()
			if err != nil {
				slog.Error("Worker failed to get from queue", "worker_id", id, "error", err)
				c.workerSleep()
				continue
			}

			if item == nil {
				if c.shouldExitOnEmptyQueue() {
					slog.Debug("Worker no more items in queue, exiting", "worker_id", id)
					return
				}
				c.workerSleep()
				continue
			}

			c.processURLItem(id, item)
		}
	}
}

// shouldStopWorker checks if worker should stop due to limit reached
func (c *DefaultCrawler) shouldStopWorker(id int) bool {
	c.statsMutex.RLock()
	defer c.statsMutex.RUnlock()

	if c.config.Limit > 0 && c.stats.PagesCrawled >= c.config.Limit {
		slog.Info("Worker reached limit", "worker_id", id)
		return true
	}
	return false
}

// shouldExitOnEmptyQueue checks if a worker should exit when GetNextFromQueue
// returned nothing. It exits only when no crawlable work remains: no 'pending'
// rows and no 'processing' rows that another worker might still turn into new
// 'pending' links.
//
// This deliberately does NOT key off PagesCrawled. 'discovered' link-graph nodes
// (issue #46) are not crawlable, so a resume against a database that holds only
// 'discovered' rows must terminate instead of spinning forever — and a run whose
// seeds all errored (PagesCrawled == 0) must end rather than hang.
func (c *DefaultCrawler) shouldExitOnEmptyQueue() bool {
	var (
		hasItems bool
		err      error
	)
	if c.strictLayers() {
		// Scoped to the current layer: work waiting at a deeper layer is not
		// this round's to do, and treating it as "still busy" would keep the
		// workers alive past the barrier.
		hasItems, err = c.storage.HasQueuedItemsAtDepth(c.layerDepth)
	} else {
		hasItems, err = c.storage.HasQueuedItems()
	}
	if err != nil {
		slog.Error("Worker failed to check queued items", "error", err)
		return false
	}
	return !hasItems
}

// nextQueueItem claims the next URL a worker should process. The three
// scheduling modes differ here and almost nowhere else.
func (c *DefaultCrawler) nextQueueItem() (*URLItem, error) {
	switch {
	case c.strictLayers():
		// Only the layer currently open. This is what the barrier is.
		return c.storage.GetNextFromQueueAtDepth(c.layerDepth)
	case c.bounded():
		// max_depth 1: shallowest pending row first, but nothing is off limits.
		// A depth-0 row still being fetched is not a reason to leave a worker
		// idle when depth-1 rows are ready to go.
		return c.storage.GetNextFromQueueByDepthPriority()
	default:
		return c.storage.GetNextFromQueue()
	}
}

// workerSleep applies the configured delay between requests
func (c *DefaultCrawler) workerSleep() {
	time.Sleep(time.Duration(c.config.RequestDelay * float64(time.Second)))
}

// processURLItem processes a single URL item from the queue
func (c *DefaultCrawler) processURLItem(id int, item *URLItem) {
	// Check robots.txt
	if !c.shouldProcessURL(id, item) {
		return
	}

	// Rate limiting
	if err := c.rateLimiter.Wait(c.ctx, item.URL); err != nil {
		slog.Error("Worker rate limiting error", "worker_id", id, "error", err)
		// A non-cancellation error here (e.g. a malformed URL that fails to parse)
		// would otherwise leave the row in 'processing' forever and hang the
		// HasQueuedItems()-based worker exit. Mark it terminal. On context
		// cancellation we leave the row alone — the run is ending and a later
		// resume's CleanupStaleProcessing resets it back to 'pending'.
		if c.ctx.Err() == nil {
			c.failAttempt(item.ID, item, nil, "rate_limit_error", err.Error())
		}
		return
	}

	// Process the page
	result, err := c.processor.Process(c.ctx, item.URL)
	if err != nil {
		c.handleProcessingError(id, item, err)
		return
	}

	c.handleProcessingResult(id, item, result)
}

// shouldProcessURL checks if URL should be processed (robots.txt check)
func (c *DefaultCrawler) shouldProcessURL(id int, item *URLItem) bool {
	if c.config.IgnoreRobotsTxt {
		return true
	}

	allowed, err := c.robotsParser.IsAllowed(c.ctx, item.URL, c.config.UserAgent)
	if err != nil {
		slog.Warn("Worker robots.txt check failed", "worker_id", id, "url", item.URL, "error", err)
	}
	if !allowed {
		slog.Info("URL disallowed by robots.txt", "worker_id", id, "url", item.URL)
		if err := c.storage.SavePageSkipped(item.ID, "robots_txt_disallow", "Disallowed by robots.txt"); err != nil {
			slog.Error("Worker failed to save robots skip", "worker_id", id, "error", err)
		}
		c.workerSleep()
		return false
	}

	// Honor the robots.txt Crawl-delay directive when it asks for a slower
	// pace than our configured default, capped so a hostile or mistyped value
	// (e.g. Crawl-delay: 86400) cannot park the crawl for hours per request.
	// The cap never goes below the user's configured delay — robots.txt may
	// only slow us down, never speed us up. SetDomainDelay reports whether it
	// actually changed anything, so the warning fires once per domain rather
	// than on every URL.
	const maxCrawlDelay = 60 * time.Second
	if u, err := url.Parse(item.URL); err == nil {
		defaultDelay := time.Duration(c.config.RequestDelay * float64(time.Second))
		if d := c.robotsParser.GetCrawlDelay(u.Host); d > defaultDelay {
			limit := time.Duration(maxCrawlDelay)
			if defaultDelay > limit {
				limit = defaultDelay
			}
			capped := d > limit
			if capped {
				d = limit
			}
			if c.rateLimiter.SetDomainDelay(u.Host, d) && capped {
				slog.Warn("Capped excessive robots.txt crawl-delay", "domain", u.Host, "applied", d)
			}
		}
	}

	return true
}

// handleProcessingError handles errors during page processing
func (c *DefaultCrawler) handleProcessingError(id int, item *URLItem, err error) {
	slog.Error("Worker failed to process URL", "worker_id", id, "url", item.URL, "error", err)
	c.failAttempt(item.ID, item, nil, "processing_error", err.Error())
	c.workerSleep()
}

// handleProcessingResult handles successful page processing results
func (c *DefaultCrawler) handleProcessingResult(id int, item *URLItem, result *PageResult) {
	// A fetch aborted by run cancellation is not a page failure: leave the row
	// 'processing' (the next run's CleanupStaleProcessing requeues it) instead
	// of burning a retry credit and recording a phantom "context canceled"
	// page error. Page == nil implies there are no links to save either.
	if result.Page == nil && c.ctx.Err() != nil {
		return
	}

	// The same reasoning covers a transient response that arrives as the run is
	// being cancelled: it would consume a retry credit for a failure the run
	// never really got to judge. Page != nil here, so the guard above misses it.
	transient := c.isTransientResult(result)
	if transient && c.ctx.Err() != nil {
		return
	}

	// Save links and queue newly discovered URLs BEFORE marking this page
	// completed. While this runs, item.ID is still 'processing', so
	// HasQueuedItems() stays true across the whole window — an idle sibling
	// worker cannot observe an empty queue and exit early before the freshly
	// discovered links are promoted to 'pending'. Reversing this order would
	// open a brief pending+processing==0 window on sparse graphs (no data loss,
	// but lost parallelism).
	if err := c.storage.SaveLinks(result.Links); err != nil {
		slog.Error("Worker failed to save links", "worker_id", id, "url", item.URL, "error", err)
	}
	c.processNewURLs(id, result.Links, item)

	// Move this page out of 'processing'.
	switch {
	case transient:
		// The response is kept — status code, headers, timing — but the row
		// stays retryable rather than becoming a completed observation. A 503
		// recorded as 'completed' would both go unretried and, because nothing
		// was parsed from it, silently drop everything that page links to.
		c.failAttempt(item.ID, item, result.Page, result.Error.ErrorType, result.Error.ErrorMessage)
		// Deliberately not "will retry": on the last attempt of the budget
		// there is no retry to come, and the log would be promising something
		// the run never does.
		slog.Info("Transient response recorded",
			"worker_id", id, "url", item.URL,
			"status", result.Page.StatusCode,
			"attempt", item.RetryCount+1, "max_attempts", MaxRetries)

	case result.Page != nil:
		if err := c.storage.SavePageResult(item.ID, result.Page); err != nil {
			slog.Error("Worker failed to save page", "worker_id", id, "url", item.URL, "error", err)
		} else {
			c.incrementCrawledCount()
		}

	default:
		// No page was produced — e.g. a transport/network failure that the
		// processor encodes in result.Error (Page == nil, no Go error, so it
		// lands here rather than in handleProcessingError). Mark the row 'error'
		// so it leaves 'processing'. Otherwise the row would stay 'processing'
		// forever and the HasQueuedItems()-based worker exit could never fire —
		// every worker would sleep-loop indefinitely.
		errType, errMsg := "processing_error", "no page result"
		if result.Error != nil {
			errType, errMsg = result.Error.ErrorType, result.Error.ErrorMessage
		}
		// network_error lands here, and it is paced like a transient HTTP
		// status: a host that times out will time out again immediately, so
		// retrying without a wait spends the budget without giving it a chance
		// to recover.
		c.failAttempt(item.ID, item, nil, errType, errMsg)
	}

	// Save error details to the crawl_errors table (separate from the pages row).
	// One row per attempt, so a URL that recovers on its third try still shows
	// what the first two saw.
	if result.Error != nil {
		result.Error.Attempt = item.RetryCount + 1
		if err := c.storage.SaveError(result.Error); err != nil {
			slog.Error("Worker failed to save error", "worker_id", id, "url", item.URL, "error", err)
		}
	}

	// Log processing result
	c.logProcessingResult(id, item.URL, result)

	// Delay after processing
	c.workerSleep()
}

// processNewURLs collects and queues new URLs from links.
//
// In a bounded crawl the children of the page just crawled sit one hop further
// out. Children past the bound are not queued, but SaveLinks has already
// recorded them, so the link graph still shows the frontier the crawl stopped
// at — the bound limits what is fetched, not what is known.
func (c *DefaultCrawler) processNewURLs(id int, links []*LinkData, parent *URLItem) {
	childDepth := parent.Depth + 1
	if c.bounded() && childDepth > c.config.MaxDepth {
		return
	}

	var newURLs []string
	for _, link := range links {
		if link.LinkType != "internal" || !c.shouldCrawlURL(link.TargetURL) {
			continue
		}
		// Queue the URL when it is brand new, or when it currently exists only as
		// a 'discovered' link-graph node (created by SaveLinks). AddToQueue inserts
		// or promotes it to 'pending'. URLs already pending/processing/completed/
		// skipped/error are left untouched.
		if status, exists := c.storage.GetURLStatus(link.TargetURL); !exists || status == "discovered" {
			newURLs = append(newURLs, link.TargetURL)
		}
	}

	if len(newURLs) > 0 {
		var err error
		if c.bounded() {
			err = c.storage.AddToQueueWithDepth(newURLs, childDepth)
		} else {
			err = c.storage.AddToQueue(newURLs)
		}
		if err != nil {
			slog.Error("Worker failed to add URLs to queue", "worker_id", id, "error", err)
		}
	}
}

// logProcessingResult logs the result of URL processing
func (c *DefaultCrawler) logProcessingResult(id int, url string, result *PageResult) {
	if result.Page != nil {
		slog.Info("Worker processed URL", "worker_id", id, "url", url, "status", result.Page.StatusCode, "links", len(result.Links))
	} else {
		slog.Info("Worker processed URL (failed)", "worker_id", id, "url", url, "links", len(result.Links))
	}
}

// statsReporter periodically reports crawling statistics
func (c *DefaultCrawler) statsReporter() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			// Get real-time queue status from database
			pending, processing, completed, errors, err := c.storage.GetQueueStatus()
			if err != nil {
				slog.Error("Failed to get queue status", "error", err)
				continue
			}

			stats := c.GetStats()
			slog.Info("Crawling stats", "crawled", stats.PagesCrawled, "pending", pending, "processing", processing, "completed", completed, "errors", errors, "duration", stats.Duration)
		}
	}
}

// Helper methods

// shouldCrawlURL determines if a URL should be crawled based on include/exclude patterns
func (c *DefaultCrawler) shouldCrawlURL(urlStr string) bool {
	// First check if the host is allowed for crawling
	if !c.isAllowedHost(urlStr) {
		return false
	}

	// If include patterns are specified, URL must match at least one
	if len(c.includePatterns) > 0 {
		matched := false
		for _, re := range c.includePatterns {
			if re.MatchString(urlStr) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Check exclude patterns - URL must not match any
	for _, re := range c.excludePatterns {
		if re.MatchString(urlStr) {
			return false
		}
	}

	return true
}

func (c *DefaultCrawler) incrementCrawledCount() {
	c.statsMutex.Lock()
	defer c.statsMutex.Unlock()
	c.stats.PagesCrawled++
}

// Note: Queue counts are now managed by the database
// These methods are kept for compatibility but could be removed
// as queue status comes directly from database queries

func (c *DefaultCrawler) incrementErrorCount() {
	c.statsMutex.Lock()
	defer c.statsMutex.Unlock()
	c.stats.ErrorCount++
}
