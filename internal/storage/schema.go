package storage

const schemaSQL = `
-- Pages table now serves as both queue and results storage
-- status column manages the lifecycle: queued -> processing -> completed
CREATE TABLE IF NOT EXISTS pages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    url TEXT UNIQUE NOT NULL,
    status TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'processing', 'completed', 'error')),
    
    -- Queue-related fields
    added_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    processing_started_at DATETIME,
    
    -- Crawl result fields (NULL until crawled)
    status_code INTEGER,
    title TEXT,
    meta_description TEXT,
    meta_robots TEXT,
    canonical_url TEXT,
    content_hash TEXT,
    ttfb_ms INTEGER,
    download_time_ms INTEGER,
    response_size_bytes INTEGER,
    content_type TEXT,
    content_length INTEGER,
    last_modified DATETIME,
    server TEXT,
    content_encoding TEXT,
    crawled_at DATETIME,
    
    -- Error tracking
    retry_count INTEGER DEFAULT 0,
    last_error_type TEXT,
    last_error_message TEXT
);

-- Indexes for efficient querying
CREATE INDEX IF NOT EXISTS idx_pages_status ON pages(status);
CREATE INDEX IF NOT EXISTS idx_pages_status_added ON pages(status, added_at);
CREATE INDEX IF NOT EXISTS idx_pages_url ON pages(url);
CREATE INDEX IF NOT EXISTS idx_pages_content_hash ON pages(content_hash) WHERE content_hash IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_pages_status_code ON pages(status_code) WHERE status = 'completed';

-- View for completed pages only (for analysis/reporting)
CREATE VIEW IF NOT EXISTS completed_pages AS
SELECT 
    id, url, status_code, title, meta_description, meta_robots,
    canonical_url, content_hash, ttfb_ms, download_time_ms,
    response_size_bytes, content_type, content_length,
    last_modified, server, content_encoding, crawled_at
FROM pages
WHERE status = 'completed';

-- View for queue management
CREATE VIEW IF NOT EXISTS queue_status AS
SELECT 
    status,
    COUNT(*) as count,
    MIN(added_at) as oldest_item,
    MAX(added_at) as newest_item
FROM pages
GROUP BY status;

-- Links table stores link relationships
CREATE TABLE IF NOT EXISTS links (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source_url TEXT NOT NULL,
    target_url TEXT NOT NULL,
    anchor_text TEXT,
    link_type TEXT,
    rel_attribute TEXT,
    crawled_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(source_url, target_url)
);

CREATE INDEX IF NOT EXISTS idx_links_source ON links(source_url);
CREATE INDEX IF NOT EXISTS idx_links_target ON links(target_url);

-- Separate errors table for detailed error tracking
CREATE TABLE IF NOT EXISTS crawl_errors (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    url TEXT NOT NULL,
    error_type TEXT NOT NULL,
    error_message TEXT,
    occurred_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_errors_url ON crawl_errors(url);
CREATE INDEX IF NOT EXISTS idx_errors_type ON crawl_errors(error_type);
CREATE INDEX IF NOT EXISTS idx_errors_occurred ON crawl_errors(occurred_at);

-- Crawl meta table stores metadata as key-value pairs
CREATE TABLE IF NOT EXISTS crawl_meta (
    key TEXT PRIMARY KEY NOT NULL,
    value TEXT NOT NULL
);
`