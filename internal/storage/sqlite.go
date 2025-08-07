// Package storage provides data persistence functionality for the crawler.
// It implements SQLite-based storage for pages, links, errors, and queue management.
package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"linktadoru/internal/crawler"
	// SQLite database driver (CGO-free)
	_ "modernc.org/sqlite"
)

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

	// Configure connection pool
	db.SetMaxOpenConns(5)
	db.SetMaxIdleConns(5)
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
	// Enable foreign keys and WAL mode for better performance
	pragmas := []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA cache_size = -64000", // 64MB cache
		"PRAGMA temp_store = MEMORY",
	}

	for _, pragma := range pragmas {
		if _, err := s.db.Exec(pragma); err != nil {
			return fmt.Errorf("failed to execute pragma %s: %w", pragma, err)
		}
	}

	// Create schema
	if _, err := s.db.Exec(schemaSQL); err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	return nil
}

// Close closes the database connection
func (s *SQLiteStorage) Close() error {
	return s.db.Close()
}

// AddToQueue adds URLs to the queue (pages table with status='queued')
// Uses INSERT OR IGNORE to prevent duplicates
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
		INSERT OR IGNORE INTO pages (url, status, added_at) 
		VALUES (?, 'queued', ?)
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

	now := time.Now()
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
			WHERE status = 'queued' 
			ORDER BY added_at ASC 
			LIMIT 1
		) AND status = 'queued'
		RETURNING id, url
	`, time.Now()).Scan(&item.ID, &item.URL)

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
		page.CrawledAt,
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

// SaveLink saves a single link relationship (legacy method)
func (s *SQLiteStorage) SaveLink(link *crawler.LinkData) error {
	query := `
		INSERT OR IGNORE INTO links (
			source_url, target_url, anchor_text, link_type, 
			rel_attribute, crawled_at
		) VALUES (?, ?, ?, ?, ?, ?)
	`

	_, err := s.db.Exec(query,
		link.SourceURL,
		link.TargetURL,
		link.AnchorText,
		link.LinkType,
		link.RelAttribute,
		link.CrawledAt,
	)

	if err != nil {
		return fmt.Errorf("failed to save link: %w", err)
	}
	return nil
}

// SaveLinks saves multiple link relationships in a single transaction (batch operation)
func (s *SQLiteStorage) SaveLinks(links []*crawler.LinkData) error {
	if len(links) == 0 {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare(`
		INSERT OR IGNORE INTO links (
			source_url, target_url, anchor_text, link_type, 
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
		if _, err := stmt.Exec(
			link.SourceURL,
			link.TargetURL,
			link.AnchorText,
			link.LinkType,
			link.RelAttribute,
			link.CrawledAt,
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
		crawlErr.OccurredAt,
	)

	if err != nil {
		return fmt.Errorf("failed to save error: %w", err)
	}
	return nil
}

// GetQueueStatus returns counts by status
func (s *SQLiteStorage) GetQueueStatus() (queued int, processing int, completed int, errors int, err error) {
	query := `
		SELECT 
			SUM(CASE WHEN status = 'queued' THEN 1 ELSE 0 END) as queued,
			SUM(CASE WHEN status = 'processing' THEN 1 ELSE 0 END) as processing,
			SUM(CASE WHEN status = 'completed' THEN 1 ELSE 0 END) as completed,
			SUM(CASE WHEN status = 'error' THEN 1 ELSE 0 END) as errors
		FROM pages
	`

	err = s.db.QueryRow(query).Scan(&queued, &processing, &completed, &errors)
	if err != nil {
		return 0, 0, 0, 0, fmt.Errorf("failed to get queue status: %w", err)
	}

	return queued, processing, completed, errors, nil
}

// HasQueuedItems checks if there are any items available for processing (queued or processing status)
func (s *SQLiteStorage) HasQueuedItems() (bool, error) {
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*) 
		FROM pages 
		WHERE status IN ('queued', 'processing')
	`).Scan(&count)
	
	if err != nil {
		return false, fmt.Errorf("failed to check queued items: %w", err)
	}
	
	return count > 0, nil
}

// GetProcessingItems returns currently processing items
func (s *SQLiteStorage) GetProcessingItems() ([]crawler.URLItem, error) {
	query := `
		SELECT id, url 
		FROM pages 
		WHERE status = 'processing' 
		ORDER BY processing_started_at DESC
	`

	rows, err := s.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to query processing items: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			// Log error but don't fail the operation
			_ = err
		}
	}()

	var items []crawler.URLItem
	for rows.Next() {
		var item crawler.URLItem
		if err := rows.Scan(&item.ID, &item.URL); err != nil {
			return nil, fmt.Errorf("failed to scan processing item: %w", err)
		}
		items = append(items, item)
	}

	return items, nil
}

// CleanupStaleProcessing resets processing items that have been stuck
func (s *SQLiteStorage) CleanupStaleProcessing(timeout time.Duration) error {
	cutoff := time.Now().Add(-timeout)

	_, err := s.db.Exec(`
		UPDATE pages 
		SET status = 'queued', processing_started_at = NULL 
		WHERE status = 'processing' 
		AND processing_started_at < ?
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
		// Log error but return false
		return "", false
	}
	return status, true
}
