// Package crawler provides the core web crawling functionality.
// It implements a concurrent, queue-based crawler with rate limiting,
// robots.txt compliance, and comprehensive page analysis capabilities.
package crawler

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"
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
	urlPolicy    *urlPolicy

	// State
	stats      CrawlStats
	statsMutex sync.RWMutex
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup // tracks workers only (not the stats reporter)
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
	processor := NewPageProcessorWithConfig(httpClient, config.AllowedSchemes, true)
	rateLimiter := NewRateLimiter(time.Duration(config.RequestDelay * float64(time.Second)))
	robotsParser := NewRobotsParser(httpClient, config.IgnoreRobotsTxt)

	policy, err := newURLPolicy(
		config.AllowedSchemes,
		config.IncludePatterns,
		config.ExcludePatterns,
		config.FollowExternalHosts,
	)
	if err != nil {
		return nil, fmt.Errorf("invalid URL policy: %w", err)
	}
	if len(config.IncludePatterns) > 0 {
		if config.FollowExternalHosts {
			slog.Warn("follow_external_hosts allows every URL with an allowed scheme; include_patterns do not narrow this mode; use exclude_patterns to restrict it")
		} else {
			slog.Warn("include_patterns add URL ranges; seed origins remain allowed; use exclude_patterns to narrow them")
		}
	}

	crawler := &DefaultCrawler{
		config:       config,
		storage:      storage,
		httpClient:   httpClient,
		processor:    processor,
		rateLimiter:  rateLimiter,
		robotsParser: robotsParser,
		urlPolicy:    policy,
		stats: CrawlStats{
			StartTime: time.Now(),
		},
	}

	// Redirects use the same URL policy as claimed and discovered URLs.
	httpClient.SetRedirectPolicy(func(u *url.URL) bool {
		return crawler.urlPolicy.allows(u.String())
	})

	return crawler, nil
}

// Start starts the crawling process
// Startup process:
//  1. Add seed URLs to queue with 'pending' status
//  2. Start configured number of workers
//  3. Workers compete for 'pending' items using atomic status updates
//  4. Continue until queue is empty or limits reached, then retry transient
//     failures until each URL reaches the three-attempt cap
func (c *DefaultCrawler) Start(ctx context.Context, seedURLs []string) error {
	c.ctx, c.cancel = context.WithCancel(ctx)
	defer c.cancel()

	if err := c.urlPolicy.validateExplicitURLs(seedURLs); err != nil {
		return err
	}
	if len(seedURLs) == 0 && c.hasConfiguredCredentials() {
		return fmt.Errorf("seedless resume cannot use authentication or custom headers; supply the seed URLs again")
	}

	// Reset rows left in 'processing' by a previous interrupted run back to
	// 'pending'. No workers are running yet, so every 'processing' row is stale.
	// This both re-queues interrupted URLs and prevents a stale 'processing' row
	// from keeping HasQueuedItems() perpetually true, which would otherwise stop
	// shouldExitOnEmptyQueue from ever firing and hang every worker (issue #46
	// review follow-up). Passing 0 treats all current 'processing' rows as stale.
	if err := c.storage.CleanupStaleProcessing(0); err != nil {
		return fmt.Errorf("failed to reset stale processing rows: %w", err)
	}
	if err := c.storage.ValidateDepthTracking(seedURLs); err != nil {
		return err
	}

	if len(seedURLs) > 0 {
		slog.Info("Starting crawler", "seed_urls", len(seedURLs))

		// Step 1: Add seed URLs to queue first (before starting workers)
		err := c.storage.AddSeeds(seedURLs)
		if err != nil {
			return fmt.Errorf("failed to add seed URLs to queue: %w", err)
		}
		slog.Info("Added seed URLs to queue", "count", len(seedURLs))
	} else {
		slog.Info("Starting crawler - resuming from existing queue")
	}

	depthZeroURLs, err := c.storage.GetDepthZeroURLs()
	if err != nil {
		return fmt.Errorf("failed to restore URL policy roots: %w", err)
	}
	if err := c.urlPolicy.setImplicitOrigins(depthZeroURLs); err != nil {
		return fmt.Errorf("failed to restore URL policy roots: %w", err)
	}
	credentialOrigins, err := c.urlPolicy.origins(seedURLs)
	if err != nil {
		return fmt.Errorf("failed to build credential policy: %w", err)
	}
	// This policy is installed before workers start and is read-only afterward.
	c.httpClient.SetCredentialPolicy(func(u *url.URL) bool {
		_, origin, err := c.urlPolicy.parse(u.String())
		if err != nil {
			return false
		}
		_, ok := credentialOrigins[origin]
		return ok
	})

	// Step 2: Start workers after queue is populated
	for i := 0; i < c.config.Concurrency; i++ {
		c.wg.Add(1)
		go c.worker(i)
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

	// Always wait for the workers themselves. On external cancellation they
	// exit promptly (in-flight requests carry c.ctx), and waiting here is what
	// makes shutdown graceful: Start does not return while a worker may still
	// be writing to the database.
	c.wg.Wait()

	if c.ctx.Err() != nil {
		slog.Info("Crawling cancelled")
	} else {
		slog.Info("Crawling completed - checking for retries")
		if err := c.performRetries(); err != nil {
			slog.Error("Error during retry processing", "error", err)
		}
	}

	// Stop the stats reporter and wait for it before returning.
	c.cancel()
	<-statsDone

	return nil
}

// performRetries handles retry logic for error status pages
func (c *DefaultCrawler) performRetries() error {
	for c.ctx.Err() == nil {
		requeued, err := c.storage.RequeueErrorPages(MaxRetryAttempts)
		if err != nil {
			return fmt.Errorf("failed to requeue error pages: %w", err)
		}
		if requeued == 0 {
			return nil
		}
		slog.Info("Retrying failed pages", "count", requeued)
		for i := 0; i < c.config.Concurrency; i++ {
			c.wg.Add(1)
			go c.worker(i)
		}
		c.wg.Wait()
	}
	return nil
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

			item, err := c.storage.GetNextFromQueue()
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
	hasItems, err := c.storage.HasQueuedItems()
	if err != nil {
		slog.Error("Worker failed to check queued items", "error", err)
		return false
	}
	return !hasItems
}

// workerSleep applies the configured delay between requests
func (c *DefaultCrawler) workerSleep() {
	time.Sleep(time.Duration(c.config.RequestDelay * float64(time.Second)))
}

// processURLItem processes a single URL item from the queue
func (c *DefaultCrawler) processURLItem(id int, item *URLItem) {
	// Authorization precedes robots.txt because that lookup is itself a
	// network request to the claimed URL's origin.
	if !c.urlPolicy.allows(item.URL) {
		slog.Info("URL denied by URL policy", "worker_id", id, "url", item.URL)
		if err := c.storage.SavePageSkipped(item.ID, "url_policy_denied", "Denied by URL policy"); err != nil {
			slog.Error("Worker failed to save URL policy skip", "worker_id", id, "error", err)
		}
		return
	}

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
			if serr := c.storage.SavePageError(item.ID, "rate_limit_error", err.Error()); serr != nil {
				slog.Error("Worker failed to mark rate-limit error", "worker_id", id, "url", item.URL, "error", serr)
			}
			c.incrementErrorCount()
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
	if saveErr := c.storage.SavePageError(item.ID, "processing_error", err.Error()); saveErr != nil {
		slog.Error("Worker failed to save processing error", "worker_id", id, "error", saveErr)
	}
	c.incrementErrorCount()
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

	// Move this page out of 'processing' to a terminal state.
	if result.Page != nil {
		var err error
		if result.Error != nil {
			err = c.storage.SavePageResponseError(item.ID, result.Page, result.Error.ErrorType, result.Error.ErrorMessage)
		} else {
			err = c.storage.SavePageResult(item.ID, result.Page)
		}
		if err != nil {
			slog.Error("Worker failed to save page", "worker_id", id, "url", item.URL, "error", err)
		} else if result.Error == nil {
			c.incrementCrawledCount()
		} else {
			c.incrementErrorCount()
		}
	} else {
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
		if err := c.storage.SavePageError(item.ID, errType, errMsg); err != nil {
			slog.Error("Worker failed to mark page error", "worker_id", id, "url", item.URL, "error", err)
		}
		c.incrementErrorCount()
	}

	// Save error details to the crawl_errors table (separate from the pages row).
	if result.Error != nil {
		if err := c.storage.SaveError(result.Error); err != nil {
			slog.Error("Worker failed to save error", "worker_id", id, "url", item.URL, "error", err)
		}
	}

	// Log processing result
	c.logProcessingResult(id, item.URL, result)

	// Delay after processing
	c.workerSleep()
}

// processNewURLs collects and queues new URLs from links
func (c *DefaultCrawler) processNewURLs(id int, links []*LinkData, parent *URLItem) {
	childDepth := parent.Depth + 1
	if c.config.MaxDepth > 0 && childDepth > c.config.MaxDepth {
		return
	}

	var newURLs []string
	for _, link := range links {
		if !c.urlPolicy.allows(link.TargetURL) {
			continue
		}
		newURLs = append(newURLs, link.TargetURL)
	}

	if len(newURLs) > 0 {
		if err := c.storage.AddToQueue(newURLs, childDepth); err != nil {
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

func (c *DefaultCrawler) hasConfiguredCredentials() bool {
	if len(c.config.Headers) > 0 {
		return true
	}
	if username, password := c.config.GetBasicAuthCredentials(); username != "" || password != "" {
		return true
	}
	if c.config.GetBearerToken() != "" {
		return true
	}
	header, value := c.config.GetAPIKeyCredentials()
	return header != "" || value != ""
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
