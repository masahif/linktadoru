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
    FollowExternalHosts bool
    Limit               int
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

- Two failure classes are retryable: `network_error` (a transport failure that produced no response) and the transient HTTP responses `408`, `429`, `500`, `502`, `503` and `504`, stored as `http_NNN`
- A transient HTTP response is recorded in full — `status_code`, `response_http_headers`, timing, size — while the row stays in `error`. The observation must survive the decision to retry, or the evidence for that decision is lost with it
- Every other status is a permanent observation, stored as `completed` and never retried. A `404` or `410` is the deletion signal a snapshot comparison relies on
- Attempts are bounded to 3 per URL for the whole crawl of a database, tracked in `retry_count`. Because that column is persisted, a cancelled run hands its remaining attempts to the resume; an uninterrupted run spends the budget before returning rather than leaving attempts unmade
- Retry placement: after the queue drains for an unbounded crawl, and inside each depth layer for a bounded one, because a page that recovers on retry has children belonging to the very next layer
- Pacing applies to every retryable failure, `network_error` included: a transport failure has no Retry-After to honour but still needs a wait, or the budget is spent against a struggling host in milliseconds
- `Retry-After` is honored in both its delta-seconds and HTTP-date forms, capped at 60 seconds (matching the robots.txt crawl-delay cap) so an untrusted value cannot stall the crawl. Without the header the wait starts at 1 second and doubles per attempt to the same cap. The delay is a pure function of the attempt number — no jitter, so two runs over an unchanged site behave the same way
- `pages.retry_after` holds the earliest time a row may be attempted again; NULL means no wait. It is persisted rather than held in memory because a retry can outlive the run that scheduled it — a run interrupted mid-backoff must not resume by hammering the host
- All failures are written through a single storage path (`SaveFailedAttempt`), so a call site cannot fail a row and forget to pace it
- Each failed attempt appends to `crawl_errors` with its `status_code` and `attempt` number
- Deterministic failures (e.g. malformed URLs, oversized responses) are deliberately not retried
- Cancellation does not consume a retry credit: the row is left `processing` for the next run's stale-row cleanup

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
