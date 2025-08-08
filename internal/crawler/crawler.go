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

	// State
	stats         CrawlStats
	statsMutex    sync.RWMutex
	ctx           context.Context
	cancel        context.CancelFunc
	wg            sync.WaitGroup
	activeWorkers int
	workersMutex  sync.Mutex
}

// NewCrawler creates a new crawler instance with the provided configuration and storage.
// It initializes all necessary components including HTTP client, page processor,
// rate limiter, and robots.txt parser. The crawler is ready to start crawling
// after creation.
func NewCrawler(config *config.CrawlConfig, storage Storage) (*DefaultCrawler, error) {

	// Initialize HTTP client
	httpClient := NewHTTPClient(config.UserAgent, config.RequestTimeout)

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
	processor := NewPageProcessor(httpClient)
	rateLimiter := NewRateLimiter(time.Duration(config.RequestDelay * float64(time.Second)))
	robotsParser := NewRobotsParser(httpClient, config.IgnoreRobots)

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

	crawler := &DefaultCrawler{
		config:       config,
		storage:      storage,
		httpClient:   httpClient,
		processor:    processor,
		rateLimiter:  rateLimiter,
		robotsParser: robotsParser,
		allowedHosts: allowedHosts,
		stats: CrawlStats{
			StartTime: time.Now(),
		},
	}

	return crawler, nil
}

// isAllowedHost checks if the given URL's host is allowed for crawling
func (c *DefaultCrawler) isAllowedHost(targetURL string) bool {
	// If external hosts are allowed, accept any host
	if c.config.FollowExternalHosts {
		return true
	}

	parsedURL, err := url.Parse(targetURL)
	if err != nil {
		return false
	}

	targetHost := parsedURL.Scheme + "://" + parsedURL.Host

	// Check if the target host matches any of the allowed hosts
	for _, allowedHost := range c.allowedHosts {
		if targetHost == allowedHost {
			return true
		}
	}

	return false
}

// Start starts the crawling process
// Startup process:
// 1. Add seed URLs to queue with 'queued' status
// 2. Start configured number of workers
// 3. Workers compete for 'queued' items using atomic status updates
// 4. Continue until queue is empty or limits reached
func (c *DefaultCrawler) Start(ctx context.Context, seedURLs []string) error {
	c.ctx, c.cancel = context.WithCancel(ctx)
	defer c.cancel()

	if len(seedURLs) > 0 {
		slog.Info("Starting crawler", "seed_urls", len(seedURLs))

		// Step 1: Add seed URLs to queue first (before starting workers)
		var urls []string
		for i, seedURL := range seedURLs {
			if c.config.Limit > 0 && i >= c.config.Limit {
				break
			}
			urls = append(urls, seedURL)
		}

		err := c.storage.AddToQueue(urls)
		if err != nil {
			return fmt.Errorf("failed to add seed URLs to queue: %w", err)
		}
		slog.Info("Added seed URLs to queue", "count", len(urls))
	} else {
		slog.Info("Starting crawler - resuming from existing queue")
	}

	// Step 2: Start workers after queue is populated
	c.activeWorkers = c.config.Concurrency
	for i := 0; i < c.config.Concurrency; i++ {
		c.wg.Add(1)
		go c.worker(i)
	}

	// Start stats reporter
	c.wg.Add(1)
	go c.statsReporter()

	// Wait for completion or context cancellation
	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		slog.Info("Crawling completed")
	case <-c.ctx.Done():
		slog.Info("Crawling cancelled")
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
	defer c.handleWorkerShutdown(id)

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

// handleWorkerShutdown handles worker cleanup when shutting down
func (c *DefaultCrawler) handleWorkerShutdown(id int) {
	c.workersMutex.Lock()
	c.activeWorkers--
	if c.activeWorkers == 0 {
		// All workers are done, cancel context to stop stats reporter
		c.cancel()
	}
	c.workersMutex.Unlock()
	slog.Debug("Worker stopped", "worker_id", id)
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

// shouldExitOnEmptyQueue checks if worker should exit when queue is empty
func (c *DefaultCrawler) shouldExitOnEmptyQueue() bool {
	c.statsMutex.RLock()
	defer c.statsMutex.RUnlock()

	return c.stats.PagesCrawled > 0
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
	if c.config.IgnoreRobots {
		return true
	}

	allowed, err := c.robotsParser.IsAllowed(c.ctx, item.URL, c.config.UserAgent)
	if err != nil {
		slog.Warn("Worker robots.txt check failed", "worker_id", id, "url", item.URL, "error", err)
	}
	if !allowed {
		slog.Info("URL disallowed by robots.txt", "worker_id", id, "url", item.URL)
		if err := c.storage.SavePageError(item.ID, "robots_disallowed", "Disallowed by robots.txt"); err != nil {
			slog.Error("Worker failed to save robots error", "worker_id", id, "error", err)
		}
		c.workerSleep()
		return false
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
	// Save page result
	if result.Page != nil {
		if err := c.storage.SavePageResult(item.ID, result.Page); err != nil {
			slog.Error("Worker failed to save page", "worker_id", id, "url", item.URL, "error", err)
		} else {
			c.incrementCrawledCount()
		}
	}

	// Save all links in batch
	if err := c.storage.SaveLinks(result.Links); err != nil {
		slog.Error("Worker failed to save links", "worker_id", id, "url", item.URL, "error", err)
	}

	// Process new URLs for crawling
	c.processNewURLs(id, result.Links, item.URL)

	// Save error if any (separate from page processing errors)
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
func (c *DefaultCrawler) processNewURLs(id int, links []*LinkData, sourceURL string) {
	var newURLs []string
	for _, link := range links {
		if link.LinkType == "internal" && c.shouldCrawlURL(link.TargetURL) {
			if status, exists := c.storage.GetURLStatus(link.TargetURL); !exists {
				newURLs = append(newURLs, link.TargetURL)
			} else {
				_ = status // URL already tracked
			}
		}
	}

	if len(newURLs) > 0 {
		if err := c.storage.AddToQueue(newURLs); err != nil {
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
	defer c.wg.Done()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			// Get real-time queue status from database
			queued, processing, completed, errors, err := c.storage.GetQueueStatus()
			if err != nil {
				slog.Error("Failed to get queue status", "error", err)
				continue
			}

			stats := c.GetStats()
			slog.Info("Crawling stats", "crawled", stats.PagesCrawled, "queued", queued, "processing", processing, "completed", completed, "errors", errors, "duration", stats.Duration)
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
	if len(c.config.IncludePatterns) > 0 {
		matched := false
		for _, pattern := range c.config.IncludePatterns {
			if m, _ := regexp.MatchString(pattern, urlStr); m {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Check exclude patterns - URL must not match any
	for _, pattern := range c.config.ExcludePatterns {
		if matched, _ := regexp.MatchString(pattern, urlStr); matched {
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
