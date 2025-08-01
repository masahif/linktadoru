# Basic Usage Examples

This document provides practical examples of using LinkTadoru for web crawling and link analysis.

## Quick Start

### 1. Simple Site Crawl

Crawl a single website with default settings:

```bash
./linktadoru https://example.com
```

### 2. Limited Crawl with Custom Settings

Crawl up to 10 pages with 2 concurrent workers:

```bash
./linktadoru --limit 10 --concurrency 2 --delay 2s https://example.com
```

### 3. Using Configuration File

Create a configuration file:

```yaml
# mysite-config.yaml
concurrency: 3
request_delay: 1s
request_timeout: 15s
user_agent: "MyBot/1.0"
respect_robots: true
limit: 50
database_path: "./mysite-crawl.db"

include_patterns:
  - "^https?://[^/]*example\.com/.*"

exclude_patterns:
  - "\\.pdf$"
  - "/admin/.*"
  - ".*\\?print=1"
```

Run with configuration:

```bash
./linktadoru --config mysite-config.yaml https://example.com
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
./linktadoru --database mycrawl.db --limit 1000 https://example.com

# Resume from where it left off
./linktadoru --database mycrawl.db
```

### 3. Aggressive Crawling (Ignore robots.txt)

```bash
./linktadoru \
  --ignore-robots \
  --concurrency 20 \
  --delay 500ms \
  https://example.com
```

### 4. Focused Crawling with Patterns

Crawl only blog posts and articles:

```bash
./linktadoru \
  --include-patterns "^https?://[^/]*example\.com/(blog|articles)/.*" \
  --exclude-patterns "\\.jpg$|\\.png$|\\.css$|\\.js$" \
  https://example.com
```


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
sqlite3 -header -csv crawl.db "SELECT * FROM pages WHERE status='completed';" > pages.csv
sqlite3 -header -csv crawl.db "SELECT * FROM links;" > links.csv
```

## Performance Tuning

### For Large Sites

```yaml
# high-performance.yaml
concurrency: 50
request_delay: 100ms
request_timeout: 10s
user_agent: "FastCrawler/1.0"
limit: 0  # unlimited
```

### For Respectful Crawling

```yaml
# respectful.yaml
concurrency: 2
request_delay: 5s
request_timeout: 30s
respect_robots: true
user_agent: "PoliteBot/1.0"
```

## Troubleshooting

### Common Issues

1. **Database locked**: Stop other instances or use different database file
2. **Too many errors**: Increase timeout or reduce concurrency
3. **Blocked by robots.txt**: Use `--ignore-robots` flag (use responsibly)
4. **Memory usage**: Reduce concurrency for large sites

### Monitoring Progress

```bash
# Check queue status while running
sqlite3 crawl.db "SELECT status, COUNT(*) FROM pages GROUP BY status;"

# View recent errors
sqlite3 crawl.db "SELECT url, error_message FROM crawl_errors ORDER BY occurred_at DESC LIMIT 5;"
```