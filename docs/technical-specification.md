# Technical Specification

## Overview

LinkTadoru is a high-performance, concurrent web crawler designed for SEO analysis. Built in Go, it leverages goroutines for parallel processing while maintaining politeness through rate limiting and robots.txt compliance.

## Architecture

### Core Design Principles

1. **Modular Architecture**: Clear separation of concerns with well-defined interfaces
2. **Concurrent Processing**: Worker pool pattern for scalable crawling
3. **Memory Efficiency**: Streaming processing and bounded queues
4. **Fault Tolerance**: Retry mechanisms and graceful error handling
5. **Extensibility**: Interface-based design for easy component replacement

### Component Overview

```
┌─────────────────┐     ┌──────────────┐     ┌──────────────┐
│   CLI/Config    │────▶│   Crawler    │────▶│   Storage    │
└─────────────────┘     └──────────────┘     └──────────────┘
                               │
                    ┌──────────┴──────────┐
                    │                     │
              ┌─────▼─────┐        ┌─────▼─────┐
              │   HTTP    │        │   Queue   │
              │  Client   │        │  Manager  │
              └─────┬─────┘        └───────────┘
                    │
              ┌─────▼─────┐
              │   Page    │
              │ Processor │
              └─────┬─────┘
                    │
              ┌─────▼─────┐
              │   HTML    │
              │  Parser   │
              └───────────┘
```

## Implementation Details

### 1. Configuration Management

**Package**: `internal/config`

The configuration system follows a hierarchical priority:
1. CLI flags (highest)
2. Environment variables (LT_*)
3. Configuration file (config.yaml)
4. Default values (lowest)

```go
type CrawlConfig struct {
    SeedURLs        []string      
    Concurrency     int           
    RequestDelay    time.Duration 
    RequestTimeout  time.Duration 
    UserAgent       string        
    RespectRobots   bool          
    IncludePatterns []string      
    ExcludePatterns []string      
    DatabasePath    string        
    Limit           int
}
```

### 2. Crawler Engine

**Package**: `internal/crawler`

The crawler implements a worker pool pattern with unified SQLite-based queue system:

- **Unified Pages Table**: Single table serves as both queue and results storage
- **Worker Pool**: Configurable number of concurrent workers
- **Status-Based Management**: Comprehensive lifecycle tracking via status column
- **Rate Limiting**: Token bucket algorithm per domain
- **Exclusive Control**: Multi-process safety via atomic SQL queries
- **Duplicate Prevention**: URL uniqueness enforced at database level

#### Worker Lifecycle

1. Atomically acquire URL from unified pages table
2. Check robots.txt compliance
3. Apply rate limiting
4. Fetch and process page
5. Update page record with crawl results
6. Extract links and add new URLs to queue
7. Mark page as completed

#### Unified Queue Architecture

The pages table serves dual purposes:

**Queue Management:**
- URLs start with `status='queued'`
- Workers atomically claim items: `queued` → `processing`
- Completion updates: `processing` → `completed` or `error`

**Results Storage:**
- Crawl result fields remain `NULL` until processed
- Atomic updates ensure data consistency
- Views provide clean interfaces for analysis

#### Exclusive Control Mechanism

Queue exclusive control uses a single atomic SQL query:

```sql
UPDATE pages 
SET status = 'processing', processing_started_at = ? 
WHERE id = (
    SELECT id FROM pages 
    WHERE status = 'queued' 
    ORDER BY added_at ASC 
    LIMIT 1
) AND status = 'queued'
RETURNING id, url
```

**Key Benefits:**
- **No Duplicate URLs**: `INSERT OR IGNORE` prevents queue pollution  
- **Race Condition Prevention**: Atomic operations ensure exclusive access
- **Inter-process Safety**: SQLite transaction-based automatic locking
- **High Performance**: Single query for acquire and update
- **State Tracking**: Clear transitions: `queued` → `processing` → `completed`/`error`
- **Resumability**: Persistent state survives process interruptions

### 3. HTTP Client

**Package**: `internal/crawler/http_client.go`

Features:
- Custom User-Agent support
- Configurable timeouts
- Automatic retry with exponential backoff
- Connection pooling
- Response size limits
- Performance metric collection (TTFB, download time)

### 4. HTML Parser

**Package**: `internal/parser`

Extracts:
- Title tags
- Meta descriptions
- Meta robots directives
- Canonical URLs
- All links (href attributes)
- Content for duplicate detection

Uses `golang.org/x/net/html` for robust HTML parsing.

### 5. Storage Layer

**Package**: `internal/storage`

SQLite-based storage with:
- Connection pooling
- Prepared statements
- Transaction support
- Concurrent access handling
- Index optimization

#### Database Schema

**Unified Pages Table (Queue + Results):**

```sql
-- Pages table serves as both queue and results storage
CREATE TABLE pages (
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
```

**Supporting Tables:**

```sql
-- Links table  
CREATE TABLE links (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    source_url TEXT NOT NULL,
    target_url TEXT NOT NULL,
    anchor_text TEXT,
    link_type TEXT,
    rel_attribute TEXT,
    crawled_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(source_url, target_url)
);

-- Separate errors table for detailed error tracking
CREATE TABLE crawl_errors (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    url TEXT NOT NULL,
    error_type TEXT NOT NULL,
    error_message TEXT,
    occurred_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Metadata table
CREATE TABLE crawl_meta (
    key TEXT PRIMARY KEY NOT NULL,
    value TEXT NOT NULL
);
```

**Optimized Indexes:**

```sql
-- Critical indexes for queue operations
CREATE INDEX idx_pages_status ON pages(status);
CREATE INDEX idx_pages_status_added ON pages(status, added_at);
CREATE INDEX idx_pages_url ON pages(url);

-- Conditional indexes for completed data only
CREATE INDEX idx_pages_content_hash ON pages(content_hash) WHERE content_hash IS NOT NULL;
CREATE INDEX idx_pages_status_code ON pages(status_code) WHERE status = 'completed';
```

**Analysis Views:**

```sql
-- View for completed pages only (for analysis/reporting)
CREATE VIEW completed_pages AS
SELECT id, url, status_code, title, meta_description, meta_robots,
       canonical_url, content_hash, ttfb_ms, download_time_ms,
       response_size_bytes, content_type, content_length,
       last_modified, server, content_encoding, crawled_at
FROM pages WHERE status = 'completed';

-- View for queue management
CREATE VIEW queue_status AS
SELECT status, COUNT(*) as count,
       MIN(added_at) as oldest_item,
       MAX(added_at) as newest_item
FROM pages GROUP BY status;
```

### 6. Rate Limiter

**Package**: `internal/crawler/rate_limiter.go`

Implementation:
- Per-domain rate limiting
- Token bucket algorithm
- Configurable delays
- Non-blocking design

### 7. Robots.txt Parser

**Package**: `internal/crawler/robots.go`

Features:
- RFC-compliant parsing
- User-Agent matching
- Crawl-delay support
- Caching for performance
- Fallback on parse errors

## Performance Characteristics

### Concurrency Model

- **Workers**: Configurable 1-100 (default: 10)
- **Queue Size**: 1000 URLs buffered
- **Memory Usage**: ~50MB base + ~1KB per URL

### Throughput

Expected performance (varies by site):
- 10 workers: ~100-200 pages/minute
- 20 workers: ~200-400 pages/minute
- 50 workers: ~500-1000 pages/minute

### Bottlenecks

1. **Network I/O**: Primary limiting factor
2. **Database Writes**: Batched for efficiency
3. **HTML Parsing**: Optimized with streaming
4. **Memory**: Bounded queues prevent unbounded growth

## Error Handling

### Retry Strategy

- Network errors: 3 retries with exponential backoff
- HTTP 5xx: 2 retries with delay
- HTTP 429: Respect Retry-After header
- Parse errors: Log and continue

### Failure Modes

1. **Connection refused**: Mark as error, continue
2. **Timeout**: Retry with longer timeout
3. **Invalid HTML**: Extract what's possible
4. **Database error**: Log and attempt recovery

## Security Considerations

1. **User-Agent**: Identifies crawler honestly
2. **Rate Limiting**: Prevents DoS behavior
3. **Robots.txt**: Respects exclusion rules
4. **URL Validation**: Prevents SSRF attacks
5. **Size Limits**: Prevents memory exhaustion

## Monitoring

### Metrics

- Pages crawled per second
- Error rates by type
- Queue depth
- Worker utilization
- Response time percentiles

### Logging

- Structured logging with levels
- Per-worker identification
- Error context preservation
- Performance metrics

## Future Enhancements

### Planned Features

1. **Distributed Crawling**: Multiple crawler coordination
2. **JavaScript Rendering**: Headless browser integration
3. **API Endpoints**: REST API for remote control
4. **Real-time Monitoring**: WebSocket status updates
5. **Plugin System**: Custom processors and extractors

### Performance Improvements

1. **Bloom Filters**: Memory-efficient duplicate detection
2. **Persistent Queue**: Resume capability
3. **Compression**: Reduce storage requirements
4. **Parallel DB Writes**: Increase throughput

## Testing Strategy

### Unit Tests

- HTTP client with mock server
- Parser with fixture HTML
- Rate limiter timing verification
- Storage with in-memory SQLite

### Integration Tests

- Full crawl simulation
- Concurrent access patterns
- Error injection
- Performance benchmarks

### Load Tests

- High concurrency scenarios
- Large site crawling
- Memory leak detection
- Database performance

## Dependencies

### Direct Dependencies

- `github.com/spf13/cobra`: CLI framework
- `github.com/spf13/viper`: Configuration management
- `golang.org/x/net`: HTML parsing
- `github.com/mattn/go-sqlite3`: SQLite driver
- `golang.org/x/time/rate`: Rate limiting

### Development Dependencies

- `golangci-lint`: Code quality
- `go test`: Testing framework
- `pprof`: Performance profiling

## Deployment

### System Requirements

- **OS**: Linux, macOS, Windows
- **RAM**: Minimum 512MB, recommended 2GB+
- **Disk**: 10GB+ for large crawls
- **Network**: Stable internet connection

### Configuration Tuning

For different scenarios:

#### Small Sites (<1000 pages)
- Concurrency: 5
- Delay: 1s
- Max pages: 1000

#### Medium Sites (1000-50000 pages)
- Concurrency: 10-20
- Delay: 500ms-1s
- Max pages: 50000

#### Large Sites (>50000 pages)
- Concurrency: 20-50
- Delay: 200ms-500ms
- Max pages: unlimited
- Batch processing recommended