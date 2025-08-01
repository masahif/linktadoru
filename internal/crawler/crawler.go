// Package crawler provides the core web crawling functionality.
// It implements a concurrent, queue-based crawler with rate limiting,
// robots.txt compliance, and comprehensive page analysis capabilities.
package crawler

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"sync"
	"time"
)

// DefaultCrawler implements the Crawler interface
type DefaultCrawler struct {
	config       *CrawlConfig
	storage      Storage
	httpClient   *HTTPClient
	processor    PageProcessor
	rateLimiter  *RateLimiter
	robotsParser *RobotsParser

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
func NewCrawler(config *CrawlConfig, storage Storage) (*DefaultCrawler, error) {

	// Initialize HTTP client
	httpClient := NewHTTPClient(config.UserAgent, config.RequestTimeout)

	// Initialize components
	processor := NewPageProcessor(httpClient)
	rateLimiter := NewRateLimiter(config.RequestDelay)
	robotsParser := NewRobotsParser(httpClient, !config.RespectRobots)

	crawler := &DefaultCrawler{
		config:       config,
		storage:      storage,
		httpClient:   httpClient,
		processor:    processor,
		rateLimiter:  rateLimiter,
		robotsParser: robotsParser,
		stats: CrawlStats{
			StartTime: time.Now(),
		},
	}

	return crawler, nil
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
		log.Printf("Starting crawler with %d seed URLs", len(seedURLs))

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
		log.Printf("Added %d seed URLs to queue", len(urls))
	} else {
		log.Printf("Starting crawler - resuming from existing queue")
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
		log.Println("Crawling completed")
	case <-c.ctx.Done():
		log.Println("Crawling cancelled")
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
	defer func() {
		c.workersMutex.Lock()
		c.activeWorkers--
		if c.activeWorkers == 0 {
			// All workers are done, cancel context to stop stats reporter
			c.cancel()
		}
		c.workersMutex.Unlock()
		log.Printf("Worker %d stopped", id)
	}()

	log.Printf("Worker %d started", id)

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
			// Check if we've reached the limit
			c.statsMutex.RLock()
			if c.config.Limit > 0 && c.stats.PagesCrawled >= c.config.Limit {
				c.statsMutex.RUnlock()
				log.Printf("Worker %d: reached limit", id)
				return
			}
			c.statsMutex.RUnlock()

			// Get next item from queue
			item, err := c.storage.GetNextFromQueue()
			if err != nil {
				log.Printf("Worker %d: failed to get from queue: %v", id, err)
				time.Sleep(c.config.RequestDelay)
				continue
			}

			if item == nil {
				// No items in queue, check if we should exit
				c.statsMutex.RLock()
				crawled := c.stats.PagesCrawled
				c.statsMutex.RUnlock()

				if crawled > 0 {
					// We've processed at least one page and queue is empty
					log.Printf("Worker %d: no more items in queue, exiting", id)
					return
				}

				// Wait and try again with configured delay
				time.Sleep(c.config.RequestDelay)
				continue
			}

			// Check robots.txt
			if c.config.RespectRobots {
				allowed, err := c.robotsParser.IsAllowed(c.ctx, item.URL, c.config.UserAgent)
				if err != nil {
					log.Printf("Worker %d: robots.txt check failed for %s: %v", id, item.URL, err)
				}
				if !allowed {
					log.Printf("Worker %d: %s disallowed by robots.txt", id, item.URL)
					// Mark as error and continue
					if err := c.storage.SavePageError(item.ID, "robots_disallowed", "Disallowed by robots.txt"); err != nil {
						log.Printf("Worker %d: failed to save robots error: %v", id, err)
					}
					time.Sleep(c.config.RequestDelay)
					continue
				}
			}

			// Rate limiting
			if err := c.rateLimiter.Wait(c.ctx, item.URL); err != nil {
				log.Printf("Worker %d: rate limiting error: %v", id, err)
				continue
			}

			// Process the page
			result, err := c.processor.Process(c.ctx, item.URL)
			if err != nil {
				log.Printf("Worker %d: failed to process %s: %v", id, item.URL, err)
				// Save page error
				if saveErr := c.storage.SavePageError(item.ID, "processing_error", err.Error()); saveErr != nil {
					log.Printf("Worker %d: failed to save processing error: %v", id, saveErr)
				}
				c.incrementErrorCount()
				time.Sleep(c.config.RequestDelay)
				continue
			}

			// Save page result
			if result.Page != nil {
				if err := c.storage.SavePageResult(item.ID, result.Page); err != nil {
					log.Printf("Worker %d: failed to save page %s: %v", id, item.URL, err)
				} else {
					c.incrementCrawledCount()
				}
			}

			// Save all links in batch (much faster than individual saves)
			if err := c.storage.SaveLinks(result.Links); err != nil {
				log.Printf("Worker %d: failed to save links for %s: %v", id, item.URL, err)
			}

			// Collect new URLs to queue
			var newURLs []string
			for _, link := range result.Links {
				// Add internal links to queue if not already exists and matches patterns
				if link.LinkType == "internal" && c.shouldCrawlURL(link.TargetURL) {
					// Check if URL already exists in any status
					if status, exists := c.storage.GetURLStatus(link.TargetURL); !exists {
						newURLs = append(newURLs, link.TargetURL)
					} else {
						// URL exists, can log if needed
						_ = status // URL already tracked
					}
				}
			}

			// Add new URLs to queue in batch
			if len(newURLs) > 0 {
				if err := c.storage.AddToQueue(newURLs); err != nil {
					log.Printf("Worker %d: failed to add URLs to queue: %v", id, err)
				}
			}

			// Save error if any (separate from page processing errors)
			if result.Error != nil {
				if err := c.storage.SaveError(result.Error); err != nil {
					log.Printf("Worker %d: failed to save error for %s: %v", id, item.URL, err)
				}
			}

			if result.Page != nil {
				log.Printf("Worker %d: processed %s (status: %d, links: %d)",
					id, item.URL, result.Page.StatusCode, len(result.Links))
			} else {
				log.Printf("Worker %d: processed %s (failed, links: %d)",
					id, item.URL, len(result.Links))
			}

			// Delay after processing to allow other workers to coordinate
			time.Sleep(c.config.RequestDelay)
		}
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
				log.Printf("Failed to get queue status: %v", err)
				continue
			}

			stats := c.GetStats()
			log.Printf("Stats: Crawled=%d, Queued=%d, Processing=%d, Completed=%d, Errors=%d, Duration=%v",
				stats.PagesCrawled, queued, processing, completed, errors, stats.Duration)
		}
	}
}

// Helper methods

// shouldCrawlURL determines if a URL should be crawled based on include/exclude patterns
func (c *DefaultCrawler) shouldCrawlURL(urlStr string) bool {
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
