package crawler

import (
	"context"
	"time"
)

// Crawler defines the main crawling interface
type Crawler interface {
	Start(ctx context.Context, seedURLs []string) error
	Stop() error
	GetStats() CrawlStats
}

// PageProcessor handles individual page processing
type PageProcessor interface {
	Process(ctx context.Context, url string) (*PageResult, error)
}

// Storage handles data persistence
type Storage interface {
	// Queue management (using pages table)
	AddToQueue(urls []string) error
	GetNextFromQueue() (*URLItem, error)
	UpdatePageStatus(id int, status string) error

	// Page results (updates existing queued entry)
	SavePageResult(id int, page *PageData) error
	// SaveFailedAttempt is the single path for failing a row: it records the
	// observed response when there was one, and always records when the row may
	// be attempted again. There is deliberately no unpaced alternative — see
	// the storage implementation.
	SaveFailedAttempt(id int, page *PageData, errorType, errorMessage string, retryAfter time.Time) error
	SavePageSkipped(id int, reason, message string) error

	// Link/Error results (separate tables)
	SaveLink(link *LinkData) error
	SaveLinks(links []*LinkData) error // Batch link saving
	SaveError(err *CrawlError) error

	// Queue status
	GetQueueStatus() (pending int, processing int, completed int, errors int, err error)
	CleanupStaleProcessing(timeout time.Duration) error
	HasQueuedItems() (bool, error) // Check if queue has any work items (pending or processing)

	// Retry management
	GetRetryablePages(maxRetries int) ([]URLItem, error)
	RequeueErrorPages(maxRetries int) (int, error)
	EarliestRetryTime(maxRetries int) (*time.Time, error)

	// Run reporting
	GetRunSummary() (RunSummary, error)

	// Depth-bounded queue management, used only when --max-depth is set. These
	// sit alongside the methods above rather than replacing them: an unbounded
	// crawl keeps its original, depth-unaware path.
	AddToQueueWithDepth(urls []string, depth int) error
	GetNextFromQueueAtDepth(depth int) (*URLItem, error)
	// GetNextFromQueueByDepthPriority is the max_depth 1 claim: shallowest
	// pending row first, without waiting on anything still in flight.
	GetNextFromQueueByDepthPriority() (*URLItem, error)
	MinUnfinishedDepth(maxRetries int) (*int, error)
	HasQueuedItemsAtDepth(depth int) (bool, error)
	HasRetryablePagesAtDepth(maxRetries, depth int) (bool, error)
	RequeueErrorPagesAtDepth(maxRetries, depth int) (int, error)
	EarliestRetryTimeAtDepth(maxRetries, depth int) (*time.Time, error)
	HasDepthlessWork(maxRetries int) (bool, error)

	// Meta-data management
	GetMeta(key string) (string, error)
	SetMeta(key, value string) error

	// URL status check (any status)
	GetURLStatus(url string) (status string, exists bool)

	// Database lifecycle
	Close() error
}

// RunSummary is the end-of-run account of what happened to every URL.
//
// Unavailable and Unreachable are kept apart from each other and from a page
// simply being absent, because a comparison between two crawls has to be able
// to say "we could not find out" rather than "it is gone". An explicit 404 is
// a Completed observation and remains the deletion signal.
type RunSummary struct {
	Completed   int // an HTTP response we accepted as the answer, 404 included
	Unavailable int // the server responded, transiently, and retries ran out
	Unreachable int // no response ever arrived (DNS, timeout, connection failure)
	Skipped     int // not fetched by policy, e.g. robots.txt
	Unfinished  int // still queued or in flight when the run ended
	Discovered  int // known from the link graph, never queued for crawling
}

// CrawlStats represents crawling statistics
type CrawlStats struct {
	PagesCrawled int
	PagesQueued  int
	ErrorCount   int
	StartTime    time.Time
	Duration     time.Duration
}

// PageResult represents the result of processing a single page
type PageResult struct {
	Page  *PageData
	Links []*LinkData
	Error *CrawlError
}
