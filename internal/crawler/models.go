package crawler

import "time"

// URLItem represents an item in the crawl queue
type URLItem struct {
	ID  int    // Queue item ID for tracking
	URL string // URL to be processed
}

// PageData represents crawled page information
type PageData struct {
	URL          string
	StatusCode   int           // HTTP status code (200, 404, 500, etc.)
	Title        string        // HTML <title> tag content
	MetaDesc     string        // HTML <meta name="description"> content
	MetaRobots   string        // HTML <meta name="robots"> content
	CanonicalURL string        // HTML <link rel="canonical"> href attribute
	ContentHash  string        // Hash of page content for duplicate detection
	TTFB         time.Duration // Time to First Byte
	DownloadTime time.Duration // Total download time
	ResponseSize int64         // Response body size in bytes
	HTTPHeaders  map[string]string // All HTTP response headers
	CrawledAt    time.Time     // Timestamp when crawled (UTC)
}

// LinkData represents link relationships
type LinkData struct {
	SourceURL    string    // URL of the page containing the link
	TargetURL    string    // URL that the link points to
	AnchorText   string    // Text content of the <a> tag
	LinkType     string    // 'internal' (same domain) or 'external' (different domain)
	RelAttribute string    // Value of rel attribute ('nofollow', 'sponsored', etc.)
	CrawledAt    time.Time // Timestamp when link was discovered
}

// CrawlError represents crawling errors
type CrawlError struct {
	URL          string    // URL where error occurred
	ErrorType    string    // Error type (timeout, dns_error, connection_failed, etc.)
	ErrorMessage string    // Detailed error message
	OccurredAt   time.Time // Error occurrence timestamp (UTC)
}

// CrawlState represents the current crawling state for resume functionality
type CrawlState struct {
	QueueURLs    []string  // Queue of pending URLs
	PagesCrawled int       // Number of pages crawled so far
	UpdatedAt    time.Time // Last update timestamp (UTC)
}

