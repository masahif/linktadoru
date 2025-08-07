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
    
    -- HTTP headers stored as JSON with generated columns for common headers
    response_http_headers JSON,
    content_type TEXT GENERATED ALWAYS AS (json_extract(response_http_headers, '$.content-type')) STORED,
    content_length INTEGER GENERATED ALWAYS AS (
        CASE 
            WHEN json_extract(response_http_headers, '$.content-length') IS NOT NULL 
            THEN CAST(json_extract(response_http_headers, '$.content-length') AS INTEGER)
            ELSE NULL 
        END
    ) STORED,
    last_modified DATETIME GENERATED ALWAYS AS (
        CASE 
            WHEN json_extract(response_http_headers, '$.last-modified') IS NOT NULL 
            THEN datetime(json_extract(response_http_headers, '$.last-modified'))
            ELSE NULL 
        END
    ) STORED,
    server TEXT GENERATED ALWAYS AS (json_extract(response_http_headers, '$.server')) STORED,
    content_encoding TEXT GENERATED ALWAYS AS (json_extract(response_http_headers, '$.content-encoding')) STORED,
    x_cache TEXT GENERATED ALWAYS AS (json_extract(response_http_headers, '$.x-cache')) STORED,
    
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

-- Indexes for generated columns from JSON headers
CREATE INDEX IF NOT EXISTS idx_pages_content_type ON pages(content_type) WHERE content_type IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_pages_server ON pages(server) WHERE server IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_pages_content_length ON pages(content_length) WHERE content_length IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_pages_x_cache ON pages(x_cache) WHERE x_cache IS NOT NULL;

-- View for completed pages only (for analysis/reporting)
CREATE VIEW IF NOT EXISTS completed_pages AS
SELECT 
    id, url, status_code, title, meta_description, meta_robots,
    canonical_url, content_hash, ttfb_ms, download_time_ms,
    response_size_bytes, response_http_headers, content_type, content_length,
    last_modified, server, content_encoding, x_cache, crawled_at
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

-- Link relationships table stores normalized link data using page IDs
-- NOTE: UNIQUE constraint on (source_page_id, target_page_id) ensures no duplicate relationships.
-- If the same link is found multiple times with different anchor_text or rel_attribute,
-- only the first occurrence is stored (subsequent duplicates are ignored via INSERT OR IGNORE).
CREATE TABLE IF NOT EXISTS link_relations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source_page_id INTEGER NOT NULL,
    target_page_id INTEGER NOT NULL,
    anchor_text TEXT,
    link_type TEXT,
    rel_attribute TEXT,
    crawled_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (source_page_id) REFERENCES pages(id),
    FOREIGN KEY (target_page_id) REFERENCES pages(id),
    UNIQUE(source_page_id, target_page_id)
);

-- Indexes for efficient querying of link relationships
CREATE INDEX IF NOT EXISTS idx_link_relations_source ON link_relations(source_page_id);
CREATE INDEX IF NOT EXISTS idx_link_relations_target ON link_relations(target_page_id);
CREATE INDEX IF NOT EXISTS idx_link_relations_type ON link_relations(link_type);

-- User-friendly view that presents links with URLs (maintains compatibility)
CREATE VIEW IF NOT EXISTS links AS
SELECT 
    lr.id,
    p1.url AS source_url,
    p2.url AS target_url,
    lr.anchor_text,
    lr.link_type,
    lr.rel_attribute,
    lr.crawled_at
FROM link_relations lr
JOIN pages p1 ON lr.source_page_id = p1.id
JOIN pages p2 ON lr.target_page_id = p2.id;

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
