// Package storage provides data persistence functionality for the crawler.
// It implements SQLite-based storage for pages, links, errors, and queue management.
package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/masahif/linktadoru/internal/crawler"
	// SQLite database driver (CGO-free)
	_ "modernc.org/sqlite"
)

// sqlTimeFormat is the layout used for every timestamp written to the database.
// It is UTC with a FIXED-WIDTH fractional second so that SQLite's string
// comparison (`processing_started_at < ?`, `ORDER BY added_at`) matches
// chronological order regardless of the machine's timezone or DST changes.
// time.RFC3339Nano is unsuitable here: it trims trailing zeros, which breaks
// lexicographic ordering ("...11.1Z" sorts after "...11.10001Z").
const sqlTimeFormat = "2006-01-02T15:04:05.000000000Z"

// sqlTime formats a timestamp for storage. Always bind times through this
// helper instead of passing time.Time to the driver, which would store Go's
// timezone-dependent time.String() form.
func sqlTime(t time.Time) string {
	return t.UTC().Format(sqlTimeFormat)
}

// SQLiteStorage implements the Storage interface using SQLite
type SQLiteStorage struct {
	db *sql.DB
}

// NewSQLiteStorage creates a new SQLite storage instance
func NewSQLiteStorage(dbPath string) (*SQLiteStorage, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool - single connection prevents lock conflicts
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(30 * time.Minute)

	storage := &SQLiteStorage{db: db}

	// Initialize schema
	if err := storage.InitSchema(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return storage, nil
}

// InitSchema creates the database schema
func (s *SQLiteStorage) InitSchema() error {
	// Enable foreign keys and WAL mode for better concurrent access
	pragmas := []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA cache_size = -64000", // 64MB cache
		"PRAGMA temp_store = MEMORY",
		"PRAGMA busy_timeout = 30000",  // 30 second timeout for locks
		"PRAGMA locking_mode = NORMAL", // Allow external monitoring processes
	}

	for _, pragma := range pragmas {
		if _, err := s.db.Exec(pragma); err != nil {
			return fmt.Errorf("failed to execute pragma %s: %w", pragma, err)
		}
	}

	// Migrate an existing pages table whose CHECK constraint predates the
	// 'discovered' status (see migratePagesAddDiscovered). No-op on a fresh DB.
	if err := s.migratePagesAddDiscovered(); err != nil {
		return fmt.Errorf("failed to migrate pages table: %w", err)
	}

	// Create schema (idempotent). After a migration this also recreates the
	// indexes and views that the table rebuild dropped.
	if _, err := s.db.Exec(schemaSQL); err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	return nil
}

// Close closes the database connection
func (s *SQLiteStorage) Close() error {
	return s.db.Close()
}

// AddToQueue queues URLs for crawling by setting their status to 'pending'.
//
// It is an upsert that owns the "promote to pending" responsibility:
//   - a brand new URL is inserted with status='pending'
//   - a URL already known only as a link-graph node ('discovered') is promoted
//     to 'pending' so it gets crawled
//   - URLs already 'pending'/'processing'/'completed'/'skipped'/'error' are left
//     untouched (no re-queue; retries are handled by RequeueErrorPages)
//
// Callers are expected to have applied include/exclude filtering before calling
// this; that is what makes the patterns take effect (see processNewURLs).
//
// On promotion, added_at is refreshed to the enqueue time so the URL joins the
// tail of the added_at-ordered queue. This is intentional: it preserves
// breadth-first crawl order (a node discovered earlier but only now selected for
// crawling is queued at the moment of selection, not its discovery time).
func (s *SQLiteStorage) AddToQueue(urls []string) error {
	if len(urls) == 0 {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare(`
		INSERT INTO pages (url, status, added_at)
		VALUES (?, 'pending', ?)
		ON CONFLICT(url) DO UPDATE SET
			status = 'pending',
			added_at = excluded.added_at
		WHERE pages.status = 'discovered'
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer func() {
		if err := stmt.Close(); err != nil {
			// Log error but don't fail the operation
			_ = err
		}
	}()

	now := sqlTime(time.Now())
	for _, url := range urls {
		if _, err := stmt.Exec(url, now); err != nil {
			return fmt.Errorf("failed to insert URL %s: %w", url, err)
		}
	}

	return tx.Commit()
}

// GetNextFromQueue atomically gets and marks the next URL for processing
func (s *SQLiteStorage) GetNextFromQueue() (*crawler.URLItem, error) {
	var item crawler.URLItem

	err := s.db.QueryRow(`
		UPDATE pages 
		SET status = 'processing', processing_started_at = ? 
		WHERE id = (
			SELECT id FROM pages 
			WHERE status = 'pending' 
			ORDER BY added_at ASC 
			LIMIT 1
		) AND status = 'pending'
		RETURNING id, url
	`, sqlTime(time.Now())).Scan(&item.ID, &item.URL)

	if err == sql.ErrNoRows {
		return nil, nil // No items in queue
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get next from queue: %w", err)
	}

	return &item, nil
}

// UpdatePageStatus updates the status of a page
func (s *SQLiteStorage) UpdatePageStatus(id int, status string) error {
	_, err := s.db.Exec(`
		UPDATE pages SET status = ? WHERE id = ?
	`, status, id)

	if err != nil {
		return fmt.Errorf("failed to update page status: %w", err)
	}
	return nil
}

// SavePageResult saves the crawl results for a page
func (s *SQLiteStorage) SavePageResult(id int, page *crawler.PageData) error {
	// Serialize HTTP headers to JSON
	var headersJSON []byte
	var err error
	if page.HTTPHeaders != nil {
		headersJSON, err = json.Marshal(page.HTTPHeaders)
		if err != nil {
			return fmt.Errorf("failed to marshal HTTP headers: %w", err)
		}
	}

	query := `
		UPDATE pages SET
			status = 'completed',
			status_code = ?,
			title = ?,
			meta_description = ?,
			meta_robots = ?,
			canonical_url = ?,
			content_hash = ?,
			ttfb_ms = ?,
			download_time_ms = ?,
			response_size_bytes = ?,
			response_http_headers = ?,
			crawled_at = ?
		WHERE id = ?
	`

	_, err = s.db.Exec(query,
		page.StatusCode,
		page.Title,
		page.MetaDesc,
		page.MetaRobots,
		page.CanonicalURL,
		page.ContentHash,
		page.TTFB.Milliseconds(),
		page.DownloadTime.Milliseconds(),
		page.ResponseSize,
		string(headersJSON),
		sqlTime(page.CrawledAt),
		id,
	)

	if err != nil {
		return fmt.Errorf("failed to save page result: %w", err)
	}
	return nil
}

// SavePageError marks a page as errored with error details
func (s *SQLiteStorage) SavePageError(id int, errorType, errorMessage string) error {
	_, err := s.db.Exec(`
		UPDATE pages SET 
			status = 'error',
			last_error_type = ?,
			last_error_message = ?,
			retry_count = retry_count + 1
		WHERE id = ?
	`, errorType, errorMessage, id)

	if err != nil {
		return fmt.Errorf("failed to save page error: %w", err)
	}
	return nil
}

// SavePageSkipped marks a page as skipped (e.g., robots.txt disallow)
func (s *SQLiteStorage) SavePageSkipped(id int, reason, message string) error {
	_, err := s.db.Exec(`
		UPDATE pages SET 
			status = 'skipped',
			last_error_type = ?,
			last_error_message = ?
		WHERE id = ?
	`, reason, message, id)

	if err != nil {
		return fmt.Errorf("failed to save page as skipped: %w", err)
	}
	return nil
}

// SaveLink saves a single link relationship using page IDs
func (s *SQLiteStorage) SaveLink(link *crawler.LinkData) error {
	// Get or create page IDs for source and target URLs
	sourceID, err := s.getOrCreatePageID(link.SourceURL)
	if err != nil {
		return fmt.Errorf("failed to get source page ID for %s: %w", link.SourceURL, err)
	}

	targetID, err := s.getOrCreatePageID(link.TargetURL)
	if err != nil {
		return fmt.Errorf("failed to get target page ID for %s: %w", link.TargetURL, err)
	}

	query := `
		INSERT OR IGNORE INTO link_relations (
			source_page_id, target_page_id, anchor_text, link_type, 
			rel_attribute, crawled_at
		) VALUES (?, ?, ?, ?, ?, ?)
	`

	_, err = s.db.Exec(query,
		sourceID,
		targetID,
		link.AnchorText,
		link.LinkType,
		link.RelAttribute,
		sqlTime(link.CrawledAt),
	)

	if err != nil {
		return fmt.Errorf("failed to save link: %w", err)
	}
	return nil
}

// SaveLinks saves multiple links in batches to avoid memory issues and large transactions
func (s *SQLiteStorage) SaveLinks(links []*crawler.LinkData) error {
	if len(links) == 0 {
		return nil
	}

	// Process links in smaller batches to avoid memory pressure and long transactions
	const batchSize = 100
	for i := 0; i < len(links); i += batchSize {
		end := i + batchSize
		if end > len(links) {
			end = len(links)
		}

		batch := links[i:end]
		if err := s.saveLinksBatch(batch); err != nil {
			return fmt.Errorf("failed to save links batch %d-%d: %w", i, end-1, err)
		}
	}

	return nil
}

// saveLinksBatch saves a batch of links in a single transaction using page IDs
func (s *SQLiteStorage) saveLinksBatch(links []*crawler.LinkData) error {
	if len(links) == 0 {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// Pre-fetch or create all page IDs in bulk to improve performance
	urlSet := make(map[string]bool)
	for _, link := range links {
		urlSet[link.SourceURL] = true
		urlSet[link.TargetURL] = true
	}

	urlToID := make(map[string]int)
	for url := range urlSet {
		// Use the same getOrCreatePageID logic but within the transaction
		var id int
		err := tx.QueryRow("SELECT id FROM pages WHERE url = ?", url).Scan(&id)
		if err == nil {
			urlToID[url] = id
			continue
		}
		if err != sql.ErrNoRows {
			return fmt.Errorf("failed to query page ID for %s: %w", url, err)
		}

		// Create page if it doesn't exist. New nodes are 'discovered' (link-graph
		// only), NOT 'pending' — otherwise every discovered link would be queued
		// for crawling, bypassing include/exclude filtering (issue #46).
		result, err := tx.Exec(
			"INSERT OR IGNORE INTO pages (url, status, added_at) VALUES (?, 'discovered', ?)",
			url, sqlTime(time.Now()),
		)
		if err != nil {
			return fmt.Errorf("failed to insert page %s: %w", url, err)
		}

		id64, err := result.LastInsertId()
		if err != nil {
			return fmt.Errorf("failed to get last insert ID for %s: %w", url, err)
		}

		if id64 == 0 {
			// Race condition, get the existing ID
			err := tx.QueryRow("SELECT id FROM pages WHERE url = ?", url).Scan(&id)
			if err != nil {
				return fmt.Errorf("failed to get existing page ID for %s: %w", url, err)
			}
			urlToID[url] = id
		} else {
			urlToID[url] = int(id64)
		}
	}

	// Now insert all links using the pre-fetched IDs
	stmt, err := tx.Prepare(`
		INSERT OR IGNORE INTO link_relations (
			source_page_id, target_page_id, anchor_text, link_type, 
			rel_attribute, crawled_at
		) VALUES (?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer func() {
		if err := stmt.Close(); err != nil {
			// Log error but don't fail the operation
			_ = err
		}
	}()

	for _, link := range links {
		sourceID := urlToID[link.SourceURL]
		targetID := urlToID[link.TargetURL]

		if _, err := stmt.Exec(
			sourceID,
			targetID,
			link.AnchorText,
			link.LinkType,
			link.RelAttribute,
			sqlTime(link.CrawledAt),
		); err != nil {
			return fmt.Errorf("failed to insert link %s -> %s: %w", link.SourceURL, link.TargetURL, err)
		}
	}

	return tx.Commit()
}

// SaveError saves crawl error details
func (s *SQLiteStorage) SaveError(crawlErr *crawler.CrawlError) error {
	query := `
		INSERT INTO crawl_errors (
			url, error_type, error_message, occurred_at
		) VALUES (?, ?, ?, ?)
	`

	_, err := s.db.Exec(query,
		crawlErr.URL,
		crawlErr.ErrorType,
		crawlErr.ErrorMessage,
		sqlTime(crawlErr.OccurredAt),
	)

	if err != nil {
		return fmt.Errorf("failed to save error: %w", err)
	}
	return nil
}

// GetQueueStatus returns counts by status
func (s *SQLiteStorage) GetQueueStatus() (pending int, processing int, completed int, errors int, err error) {
	query := `
		SELECT 
			SUM(CASE WHEN status = 'pending' THEN 1 ELSE 0 END) as pending,
			SUM(CASE WHEN status = 'processing' THEN 1 ELSE 0 END) as processing,
			SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) as completed,
			SUM(CASE WHEN status = 'error' THEN 1 ELSE 0 END) as errors
		FROM pages
	`

	err = s.db.QueryRow(query).Scan(&pending, &processing, &completed, &errors)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("failed to get queue status: %w", err)
	}

	return pending, processing, completed, errors, nil
}

// HasQueuedItems checks if there are any items available for processing (pending or processing status)
func (s *SQLiteStorage) HasQueuedItems() (bool, error) {
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*) 
		FROM pages 
		WHERE status IN ('pending', 'processing')
	`).Scan(&count)

	if err != nil {
		return false, fmt.Errorf("failed to check pending items: %w", err)
	}

	return count > 0, nil
}

// retryableErrorTypes lists the last_error_type values eligible for retry.
// These MUST match the strings the crawler actually writes via SavePageError:
// 'network_error' (transient transport failure, see page_processor.go) is
// worth retrying. Deliberately excluded: 'processing_error' and
// 'rate_limit_error' (malformed URLs — retrying reproduces the same failure)
// and 'response_too_large' (deterministic — the page will exceed the limit
// again).
const retryableErrorTypes = `('network_error')`

// GetRetryablePages returns pages with error status that can be retried
func (s *SQLiteStorage) GetRetryablePages(maxRetries int) ([]crawler.URLItem, error) {
	rows, err := s.db.Query(`
		SELECT id, url, retry_count, last_error_type
		FROM pages
		WHERE status = 'error'
		  AND retry_count < ?
		  AND last_error_type IN `+retryableErrorTypes+`
		ORDER BY retry_count ASC, added_at ASC
	`, maxRetries)
	if err != nil {
		return nil, fmt.Errorf("failed to get retryable pages: %w", err)
	}
	defer rows.Close()

	var items []crawler.URLItem
	for rows.Next() {
		var item crawler.URLItem
		var errorType string
		var retryCount int
		if err := rows.Scan(&item.ID, &item.URL, &retryCount, &errorType); err != nil {
			return nil, fmt.Errorf("failed to scan retryable page: %w", err)
		}
		items = append(items, item)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate retryable pages: %w", err)
	}

	return items, nil
}

// RequeueErrorPages moves error status pages back to pending for retry
func (s *SQLiteStorage) RequeueErrorPages(maxRetries int) (int, error) {
	result, err := s.db.Exec(`
		UPDATE pages
		SET status = 'pending', processing_started_at = NULL
		WHERE status = 'error'
		  AND retry_count < ?
		  AND last_error_type IN `+retryableErrorTypes+`
	`, maxRetries)
	if err != nil {
		return 0, fmt.Errorf("failed to requeue error pages: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get affected rows: %w", err)
	}

	return int(rowsAffected), nil
}

// CleanupStaleProcessing resets processing items that have been stuck back to
// 'pending'. A row with a NULL processing_started_at is always reset: `NULL < ?`
// is never true in SQL, so without the explicit IS NULL clause such a row would
// survive cleanup and keep HasQueuedItems() perpetually true, hanging the
// crawler. The timestamp should never be NULL on the normal path, but the
// invariant is cheap to enforce here.
func (s *SQLiteStorage) CleanupStaleProcessing(timeout time.Duration) error {
	cutoff := sqlTime(time.Now().Add(-timeout))

	_, err := s.db.Exec(`
		UPDATE pages
		SET status = 'pending', processing_started_at = NULL
		WHERE status = 'processing'
		AND (processing_started_at < ? OR processing_started_at IS NULL)
	`, cutoff)

	if err != nil {
		return fmt.Errorf("failed to cleanup stale processing: %w", err)
	}
	return nil
}

// GetMeta retrieves a metadata value
func (s *SQLiteStorage) GetMeta(key string) (string, error) {
	var value string
	err := s.db.QueryRow("SELECT value FROM crawl_meta WHERE key = ?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to get meta: %w", err)
	}
	return value, nil
}

// SetMeta stores a metadata value
func (s *SQLiteStorage) SetMeta(key, value string) error {
	_, err := s.db.Exec(
		"INSERT OR REPLACE INTO crawl_meta (key, value) VALUES (?, ?)",
		key, value,
	)
	if err != nil {
		return fmt.Errorf("failed to set meta: %w", err)
	}
	return nil
}

// GetURLStatus checks if a URL exists and returns its status
func (s *SQLiteStorage) GetURLStatus(url string) (status string, exists bool) {
	err := s.db.QueryRow("SELECT status FROM pages WHERE url = ?", url).Scan(&status)
	if err == sql.ErrNoRows {
		return "", false
	}
	if err != nil {
		// A real DB failure must not pass silently as "URL not found" — the
		// caller would then re-queue a URL that may already be tracked.
		slog.Error("Failed to query URL status", "url", url, "error", err)
		return "", false
	}
	return status, true
}

// getOrCreatePageID gets the page ID for a URL, creating it if it doesn't exist
func (s *SQLiteStorage) getOrCreatePageID(url string) (int, error) {
	// First try to get existing page ID
	var id int
	err := s.db.QueryRow("SELECT id FROM pages WHERE url = ?", url).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return 0, fmt.Errorf("failed to query page ID: %w", err)
	}

	// Page doesn't exist, create it as a 'discovered' link-graph node (not
	// 'pending') so it is not crawled unless AddToQueue promotes it (issue #46).
	result, err := s.db.Exec(
		"INSERT OR IGNORE INTO pages (url, status, added_at) VALUES (?, 'discovered', ?)",
		url, sqlTime(time.Now()),
	)
	if err != nil {
		return 0, fmt.Errorf("failed to insert page: %w", err)
	}

	// Get the ID of the inserted row
	id64, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("failed to get last insert ID: %w", err)
	}

	// If INSERT OR IGNORE didn't insert (URL already exists), get the existing ID
	if id64 == 0 {
		err := s.db.QueryRow("SELECT id FROM pages WHERE url = ?", url).Scan(&id)
		if err != nil {
			return 0, fmt.Errorf("failed to get existing page ID after race condition: %w", err)
		}
		return id, nil
	}

	return int(id64), nil
}
