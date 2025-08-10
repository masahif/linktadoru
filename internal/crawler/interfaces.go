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
	SavePageError(id int, errorType, errorMessage string) error
	SavePageSkipped(id int, reason, message string) error

	// Link/Error results (separate tables)
	SaveLink(link *LinkData) error
	SaveLinks(links []*LinkData) error // Batch link saving
	SaveError(err *CrawlError) error

	// Queue status
	GetQueueStatus() (pending int, processing int, completed int, errors int, err error)
	GetProcessingItems() ([]URLItem, error)
	CleanupStaleProcessing(timeout time.Duration) error
	HasQueuedItems() (bool, error) // Check if queue has any work items (pending or processing)

	// Retry management
	GetRetryablePages(maxRetries int) ([]URLItem, error)
	RequeueErrorPages(maxRetries int) (int, error)

	// Meta-data management
	GetMeta(key string) (string, error)
	SetMeta(key, value string) error

	// URL status check (any status)
	GetURLStatus(url string) (status string, exists bool)

	// Database lifecycle
	Close() error
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
