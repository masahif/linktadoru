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
      --auth-password string       Basic auth password
      --auth-password-env string   Environment variable for password (default "LT_AUTH_PASSWORD")
      --auth-username string       Basic auth username
      --auth-username-env string   Environment variable for username (default "LT_AUTH_USERNAME")
  -c, --concurrency int           Number of concurrent workers (default 10)
      --config string             Config file path (default "./config.yaml")
  -d, --database string           SQLite database path (default "./crawl.db")
  -r, --delay duration            Delay between requests (default 1s)
      --exclude-patterns strings  Regex patterns for URLs to exclude
  -h, --help                      help for linktadoru
      --ignore-robots             Ignore robots.txt rules
      --include-patterns strings  Regex patterns for URLs to include
  -l, --limit int                Stop after N pages (0=unlimited)
  -t, --timeout duration         HTTP request timeout (default 30s)
  -u, --user-agent string        HTTP User-Agent (default "LinkTadoru/1.0")
  -v, --version                  version for linktadoru
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

# Authentication (NOT RECOMMENDED - use environment variables instead)
# auth_username: "myuser"
# auth_password: "mypass"

# URL filtering
include_patterns:
  - "^https?://[^/]*httpbin\.org/.*"
  - "^https?://[^/]*subdomain\.httpbin\.org/.*"

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

# Authentication (recommended method)
export LT_AUTH_USERNAME="myuser"
export LT_AUTH_PASSWORD="mypass"

./linktadoru https://httpbin.org
```

## Configuration Options

| Option | CLI Flag | Environment Variable | Default | Description |
|--------|----------|---------------------|---------|-------------|
| auth_username | `--auth-username` | `LT_AUTH_USERNAME` | "" | HTTP Basic Auth username |
| auth_password | `--auth-password` | `LT_AUTH_PASSWORD` | "" | HTTP Basic Auth password |
| auth_username_env | `--auth-username-env` | - | LT_AUTH_USERNAME | Environment variable for username |
| auth_password_env | `--auth-password-env` | - | LT_AUTH_PASSWORD | Environment variable for password |
| concurrency | `-c, --concurrency` | `LT_CONCURRENCY` | 10 | Number of concurrent workers |
| request_delay | `-r, --delay` | `LT_REQUEST_DELAY` | 1s | Delay between requests per domain |
| request_timeout | `-t, --timeout` | `LT_REQUEST_TIMEOUT` | 30s | HTTP request timeout |
| user_agent | `-u, --user-agent` | `LT_USER_AGENT` | LinkTadoru/1.0 | HTTP User-Agent header |
| respect_robots | `--ignore-robots` | `LT_RESPECT_ROBOTS` | true | Respect robots.txt (CLI flag inverts) |
| limit | `-l, --limit` | `LT_LIMIT` | 0 | Maximum pages to crawl (0=unlimited) |
| database_path | `-d, --database` | `LT_DATABASE_PATH` | ./crawl.db | SQLite database file path |
| include_patterns | `--include-patterns` | `LT_INCLUDE_PATTERNS` | [] | URL patterns to include (regex) |
| exclude_patterns | `--exclude-patterns` | `LT_EXCLUDE_PATTERNS` | [] | URL patterns to exclude (regex) |

## Authentication

LinkTadoru supports HTTP Basic Authentication for accessing password-protected websites.

### Using Environment Variables (Recommended)

```bash
# Set credentials using default environment variables
export LT_AUTH_USERNAME="myuser"
export LT_AUTH_PASSWORD="mypass"
./linktadoru https://protected.httpbin.org

# Using custom environment variables
export MY_USER="myuser"
export MY_PASS="mypass"
./linktadoru --auth-username-env MY_USER --auth-password-env MY_PASS https://protected.httpbin.org
```

### Using CLI Flags (Not Recommended for Production)

```bash
./linktadoru --auth-username myuser --auth-password mypass https://protected.httpbin.org
```

### Using Configuration File (Not Recommended)

```yaml
# config.yaml - NOT recommended for security reasons
auth_username: "myuser"
auth_password: "mypass"
```

**Security Best Practice**: Always use environment variables for authentication credentials rather than CLI flags or configuration files. Environment variables are not logged in shell history or visible in process lists.

## Pattern Matching

### Include Patterns
Only URLs matching at least one include pattern will be crawled:

```yaml
include_patterns:
  - "^https?://[^/]*httpbin\.org/.*"     # Main domain
  - "^https?://[^/]*\.httpbin\.org/.*"   # All subdomains
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
user_agent: "PoliteBot/1.0 (https://httpbin.org/bot)"
```