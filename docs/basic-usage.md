# Basic Usage Examples

This document provides practical examples of using LinkTadoru for web crawling and link analysis.

## Quick Start

### 1. Simple Site Crawl

Crawl a single website with default settings:

```bash
./linktadoru https://httpbin.org
```

### 2. Limited Crawl with Custom Settings

Crawl up to 10 pages with 2 concurrent workers and a 2-second delay (`--delay` takes seconds as a number):

```bash
./linktadoru --limit 10 --concurrency 2 --delay 2 https://httpbin.org
```

### 3. Seed Lists and One-Hop Crawls

Read generated targets from a file, or use `-` for standard input:

```bash
./linktadoru --seed-file urls.txt --max-depth 1 --limit 0
fetch-target-list | ./linktadoru --seed-file - --max-depth 1 --limit 0
```

Seed files contain one URL per line. Blank lines, surrounding whitespace, and
lines beginning with `#` are ignored. `--seed-file` cannot be combined with URL
arguments.

`max_depth: 0` is unlimited. A positive value admits links through that
discovery depth: seeds are depth 0, their links are depth 1, and so on. The
queue remains asynchronous, so one slow page does not block deeper work already
admitted by another worker. Until URL-policy restoration lands, every bounded
run requires explicit seeds and a fresh database.

Selected temporary responses (408, 429, 500, 502, 503, 504) are retained in the
database and retried after normal queue work, up to three total attempts.

### 4. Using Configuration File

Create a configuration file:

```yaml
# mysite-config.yml
concurrency: 3
request_delay: 1             # seconds (number)
request_timeout: "15s"       # Go duration string
user_agent: "MyBot/1.0"
ignore_robots_txt: false
limit: 50
database_path: "./mysite-crawl.db"

include_patterns:
  - "^https?://[^/]*httpbin\\.org/.*"

exclude_patterns:
  - "\\.pdf$"
  - "/admin/.*"
  - ".*\\?print=1"
```

Run with configuration:

```bash
./linktadoru --config mysite-config.yml https://httpbin.org
```

## Advanced Examples

### 1. Multi-Site Crawling

Crawl multiple related sites:

```bash
./linktadoru \
  --limit 100 \
  --include-patterns "^https?://[^/]*(site1|site2)\.com/.*" \
  https://site1.com \
  https://site2.com
```

### 2. Resume Previous Crawl

LinkTadoru automatically resumes from existing database:

```bash
# First run (interrupted)
./linktadoru --database mycrawl.db --limit 1000 https://httpbin.org

# Resume from where it left off
./linktadoru --database mycrawl.db
```

### 3. Aggressive Crawling (Ignore robots.txt)

```bash
./linktadoru \
  --ignore-robots-txt \
  --concurrency 20 \
  --delay 0.5 \
  https://httpbin.org
```

### 4. Focused Crawling with Patterns

Crawl only blog posts and articles:

```bash
./linktadoru \
  --include-patterns "^https?://[^/]*httpbin\.org/(blog|articles)/.*" \
  --exclude-patterns "\\.jpg$|\\.png$|\\.css$|\\.js$" \
  https://httpbin.org
```

## How Crawling Behaves

### Page Status Lifecycle

Every URL gets one row in the `pages` table; the `status` column tracks its lifecycle:

- `discovered` — found as a link on a crawled page; recorded for link analysis only, not queued for crawling
- `pending` — queued for crawling (seed URLs, and discovered links that pass the include/exclude filters)
- `processing` — currently being fetched by a worker
- `completed` — the fetch finished. Note: HTTP errors such as 404 are also `completed`; check the `status_code` column for the result
- `skipped` — blocked by robots.txt
- `error` — the fetch failed: transport-level failures (DNS, timeout, connection reset), a response body exceeding `max_response_size`, or a malformed URL

### Retries

After the queue drains, transient transport failures and HTTP 408, 429, 500,
502, 503, and 504 responses are requeued until each URL reaches 3 total
attempts (tracked in `retry_count`). Deterministic failures are not retried.
Retries still follow the per-domain request delay and robots.txt `Crawl-delay`,
but do not interpret `Retry-After` and may revisit a host sooner than requested.

### robots.txt Crawl-delay

A `Crawl-delay` in robots.txt is honored when it is slower than your configured `request_delay`. It only ever slows crawling down (never speeds it up), and is capped at 60 seconds.

### Interrupting and Resuming

Ctrl-C (SIGINT/SIGTERM) stops the crawl gracefully: in-flight state is persisted and the database is closed cleanly. Rerun with the same `--database` to resume — rows left in `processing` are automatically requeued at the next start. Bounded `max_depth` runs temporarily require explicit seeds and a fresh, empty database.

## Output Analysis

### Database Queries

After crawling, analyze results with SQL:

```sql
-- Top pages by response time
SELECT url, ttfb_ms, download_time_ms 
FROM pages 
WHERE status = 'completed'
ORDER BY ttfb_ms DESC 
LIMIT 10;

-- Link analysis
SELECT 
    link_type,
    COUNT(*) as count
FROM links 
GROUP BY link_type;

-- Find broken links
SELECT url, last_error_message
FROM pages 
WHERE status = 'error';
```

### Export Data

```bash
# Export to CSV
sqlite3 -header -csv linktadoru.db "SELECT * FROM pages WHERE status='completed';" > pages.csv
sqlite3 -header -csv linktadoru.db "SELECT * FROM links;" > links.csv
```

## Performance Tuning

See [Configuration Reference — Performance Tuning](configuration.md#performance-tuning) for recommended settings per site size and for respectful crawling.

## Troubleshooting

### Common Issues

1. **Database locked**: Stop other instances or use different database file
2. **Too many errors**: Increase timeout or reduce concurrency
3. **Blocked by robots.txt**: Use `--ignore-robots-txt` flag (use responsibly)
4. **Memory usage**: Reduce concurrency for large sites

### Monitoring Progress

```bash
# Check queue status while running
sqlite3 linktadoru.db "SELECT status, COUNT(*) FROM pages GROUP BY status;"

# View recent errors
sqlite3 linktadoru.db "SELECT url, error_message FROM crawl_errors ORDER BY occurred_at DESC LIMIT 5;"
```
