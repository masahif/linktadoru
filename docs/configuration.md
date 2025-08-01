# Configuration Reference

LinkTadoru can be configured through multiple methods with the following priority:

1. Command-line flags (highest priority)
2. Environment variables (prefix: `LT_`)
3. Configuration file (`config.yaml`)
4. Default values (lowest priority)

## Command-Line Flags

```bash
./linktadoru --help

Flags:
  -c, --concurrency int       Number of concurrent workers (default 10)
      --config string         Config file path (default "./config.yaml")
  -d, --database string       SQLite database path (default "./crawl.db")
  -r, --delay duration        Delay between requests (default 1s)
      --exclude-patterns strings  Regex patterns for URLs to exclude
  -h, --help                  help for linktadoru
      --ignore-robots         Ignore robots.txt rules
      --include-patterns strings  Regex patterns for URLs to include
  -l, --limit int            Stop after N pages (0=unlimited)
  -t, --timeout duration     HTTP request timeout (default 30s)
  -u, --user-agent string    HTTP User-Agent (default "LinkTadoru/1.0")
  -v, --version              version for linktadoru
```

## Configuration File

Create a `config.yaml` file:

```yaml
# Basic crawling parameters
concurrency: 10
request_delay: 1s
request_timeout: 30s
user_agent: "LinkTadoru/1.0"
respect_robots: true
limit: 0

# URL filtering
include_patterns:
  - "^https?://[^/]*example\.com/.*"
  - "^https?://[^/]*subdomain\.example\.com/.*"

exclude_patterns:
  - "\.pdf$"
  - "/admin/.*"
  - ".*#.*"

# Storage configuration
database_path: "./crawl.db"
```

## Environment Variables

All configuration options can be set via environment variables:

```bash
export LT_CONCURRENCY=5
export LT_REQUEST_DELAY=2s
export LT_REQUEST_TIMEOUT=30s
export LT_USER_AGENT="MyBot/1.0"
export LT_RESPECT_ROBOTS=true
export LT_DATABASE_PATH="./mysite.db"
export LT_LIMIT=1000

./linktadoru https://example.com
```

## Configuration Options

| Option | CLI Flag | Environment Variable | Default | Description |
|--------|----------|---------------------|---------|-------------|
| concurrency | `-c, --concurrency` | `LT_CONCURRENCY` | 10 | Number of concurrent workers |
| request_delay | `-r, --delay` | `LT_REQUEST_DELAY` | 1s | Delay between requests per domain |
| request_timeout | `-t, --timeout` | `LT_REQUEST_TIMEOUT` | 30s | HTTP request timeout |
| user_agent | `-u, --user-agent` | `LT_USER_AGENT` | LinkTadoru/1.0 | HTTP User-Agent header |
| respect_robots | `--ignore-robots` | `LT_RESPECT_ROBOTS` | true | Respect robots.txt (CLI flag inverts) |
| limit | `-l, --limit` | `LT_LIMIT` | 0 | Maximum pages to crawl (0=unlimited) |
| database_path | `-d, --database` | `LT_DATABASE_PATH` | ./crawl.db | SQLite database file path |
| include_patterns | `--include-patterns` | `LT_INCLUDE_PATTERNS` | [] | URL patterns to include (regex) |
| exclude_patterns | `--exclude-patterns` | `LT_EXCLUDE_PATTERNS` | [] | URL patterns to exclude (regex) |

## Pattern Matching

### Include Patterns
Only URLs matching at least one include pattern will be crawled:

```yaml
include_patterns:
  - "^https?://[^/]*example\.com/.*"     # Main domain
  - "^https?://[^/]*\.example\.com/.*"   # All subdomains
  - ".*/products/.*"                     # Specific path
```

### Exclude Patterns
URLs matching any exclude pattern will be skipped:

```yaml
exclude_patterns:
  - "\.pdf$"          # Skip PDFs
  - "\.jpg$"          # Skip images
  - "/admin/.*"       # Skip admin section
  - ".*\?.*"          # Skip URLs with query strings
  - ".*#.*"           # Skip URLs with fragments
```

## Performance Tuning

### Small Sites (< 1,000 pages)
```yaml
concurrency: 5
request_delay: 1s
```

### Medium Sites (1,000 - 50,000 pages)
```yaml
concurrency: 10-20
request_delay: 500ms-1s
```

### Large Sites (> 50,000 pages)
```yaml
concurrency: 20-50
request_delay: 200ms-500ms
```

### Respectful Crawling
```yaml
concurrency: 2
request_delay: 5s
respect_robots: true
user_agent: "PoliteBot/1.0 (https://example.com/bot)"
```