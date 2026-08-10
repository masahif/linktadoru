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
3. Configuration file (linktadoru.yml)
4. Default values (lowest)

```go
type CrawlConfig struct {
    SeedURLs            []string
    Concurrency         int
    RequestDelay        float64       // seconds
    RequestTimeout      time.Duration
    UserAgent           string
    IgnoreRobotsTxt     bool
    Limit               int
    MaxDepth            int           // 0 = unlimited
    MaxResponseSize     int64         // bytes
    Auth                *Auth
    IncludePatterns     []string
    ExcludePatterns     []string
    AllowedSchemes      []string
    Headers             []string
    DatabasePath        string
    // ... logging options (LogLevel, LogFile, ...)
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
- Link-graph nodes discovered on crawled pages start as `status='discovered'` (recorded, not crawled)
- URLs selected for crawling (seeds, or discovered links passing the include/exclude filters) are promoted to `status='pending'`
- Workers atomically claim items: `pending` → `processing`
- Completion updates: `processing` → `completed`, `skipped` (robots.txt), or `error`
- Note: `completed` means the fetch finished; HTTP errors such as 404 are still `completed` with the `status_code` column recording the result

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
    WHERE status = 'pending' 
    ORDER BY added_at ASC 
    LIMIT 1
) AND status = 'pending'
RETURNING id, url
```

**Key Benefits:**
- **No Duplicate URLs**: `INSERT OR IGNORE` prevents queue pollution  
- **Race Condition Prevention**: Atomic operations ensure exclusive access
- **Inter-process Safety**: SQLite transaction-based automatic locking
- **High Performance**: Single query for acquire and update
- **State Tracking**: Clear transitions: `discovered` → `pending` → `processing` → `completed`/`skipped`/`error`
- **Resumability**: Persistent state survives process interruptions

#### Retry Handling

Retries are driven by the crawl loop and the storage layer, not by the HTTP client:

- After the queue drains, transient transport failures and HTTP 408, 429, 500, 502, 503, and 504 responses are requeued until the attempt cap is reached
- Attempts are bounded to 3 in total per URL across runs, tracked in the `retry_count` column
- There is no additional backoff and `Retry-After` is not interpreted; the normal per-domain request delay and robots.txt `Crawl-delay` still apply
- Deterministic failures (e.g. malformed URLs) are deliberately not retried

### 3. HTTP Client

**Package**: `internal/crawler/http_client.go`

Features:
- Custom User-Agent support
- Configurable timeouts
- Connection pooling
- Response size limits
- Performance metric collection (TTFB, download time)

The HTTP client performs a single fetch per request; retries are handled at the crawl level (see Retry Handling above).

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

The authoritative schema is defined in [`internal/storage/schema.go`](../internal/storage/schema.go) and is created automatically on first run. Rather than duplicating the SQL here, these are the design decisions behind it:

- **Unified `pages` table**: one row per URL serves as both queue entry and crawl result. The `status` column drives the lifecycle described above; crawl-result columns stay `NULL` until the page is fetched.
- **HTTP headers as JSON with generated columns**: the full response headers are stored once in `response_http_headers` (JSON). Frequently queried headers — `content_type`, `content_length`, `last_modified`, `server`, `content_encoding`, `x_cache` — are exposed as `GENERATED ALWAYS ... STORED` columns, so they can be indexed and queried like ordinary columns without duplicating write logic.
- **Normalized link graph**: `link_relations` stores edges as page-ID pairs with a `UNIQUE(source_page_id, target_page_id)` constraint (if the same link is found multiple times with different anchor text, only the first is kept). The `links` view re-exposes edges as URL pairs for convenient analysis.
- **Analysis views**: `completed_pages` (fetched pages only) and `queue_status` (per-status counts with oldest/newest timestamps) provide stable query interfaces over the unified table.
- **Supporting tables**: `crawl_errors` records every error occurrence for diagnostics; `crawl_meta` stores key-value crawl metadata.
- **Index strategy**: queue operations are backed by indexes on `status` and `(status, added_at)`; analysis columns use partial indexes (e.g. `WHERE content_hash IS NOT NULL`) so queue writes stay cheap.

### 6. Rate Limiter

**Package**: `internal/crawler/rate_limiter.go`

Implementation:
- Per-domain rate limiting
- Token bucket algorithm
- Configurable delays
- Non-blocking design

robots.txt `Crawl-delay` directives are honored when they are slower than the configured request delay, capped at 60 seconds.
