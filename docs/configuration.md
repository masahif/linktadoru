# Configuration Reference

LinkTadoru can be configured through multiple methods with the following priority:

1. Command-line flags (highest priority)
2. Environment variables (prefix: `LT_`)
3. Configuration file (`linktadoru.yml`)
4. Default values (lowest priority)

### Seed URLs

Seed URLs are named on the command line in one of two mutually exclusive ways:

- `--seed-file PATH` — read the list from a file (`-` reads standard input)
- URL arguments — list the URLs on the command line

The two cannot be combined; passing both is an error, because merging them would
leave the resulting crawl scope to guesswork. Neither ranks above the other:
whichever one is used takes precedence over `seed_urls` in the configuration
file, replacing it entirely rather than adding to it. When the command line
names no seeds, `seed_urls` is used. See
[Basic Usage](basic-usage.md#3-seed-urls-from-a-file-or-standard-input) for the
seed file format.

## Configuration Options

This table is the authoritative reference for all options. Run `./linktadoru --help` for the current flag list and `./linktadoru --show-config` to inspect the effective configuration.

| Option | CLI Flag | Environment Variable | Default | Description |
|--------|----------|---------------------|---------|-------------|
| **Basic Settings** |
| concurrency | `-c, --concurrency` | `LT_CONCURRENCY` | 2 | Number of concurrent workers |
| request_delay | `-r, --delay` | `LT_REQUEST_DELAY` | 0.1 | Delay between requests in seconds (number) |
| request_timeout | `-t, --timeout` | `LT_REQUEST_TIMEOUT` | 30s | HTTP request timeout (Go duration) |
| user_agent | `-u, --user-agent` | `LT_USER_AGENT` | LinkTadoru/1.0 | HTTP User-Agent header |
| ignore_robots_txt | `--ignore-robots-txt` | `LT_IGNORE_ROBOTS_TXT` | false | Ignore robots.txt rules |
| follow_external_hosts | `--follow-external-hosts` | `LT_FOLLOW_EXTERNAL_HOSTS` | false | Allow crawling hosts other than the seed hosts |
| limit | `-l, --limit` | `LT_LIMIT` | 0 | Maximum pages to crawl (0=unlimited) |
| max_depth | `--max-depth` | `LT_MAX_DEPTH` | 0 | Maximum hops from the seed URLs (0=unlimited; seeds are depth 0) |
| max_response_size | `--max-response-size` | `LT_MAX_RESPONSE_SIZE` | 10485760 | Max response body size in bytes (10 MiB) |
| database_path | `-d, --database` | `LT_DATABASE_PATH` | ./linktadoru.db | SQLite database file path |
| seed_urls | - (see `--seed-file`) | - | [] | Starting URLs (config file only; overridden by URL arguments or `--seed-file`) |
| - | `--seed-file` | - | "" | Read seed URLs from a file, one per line; `-` reads stdin (CLI only, not a config key) |
| **URL Filtering** |
| include_patterns | `--include-patterns` | `LT_INCLUDE_PATTERNS` | [] | URL patterns to include (regex) |
| exclude_patterns | `--exclude-patterns` | `LT_EXCLUDE_PATTERNS` | [] | URL patterns to exclude (regex) |
| allowed_schemes | - | - | ["https://", "http://"] | Allowed URL schemes (config file only) |
| **Authentication** |
| auth.type | `--auth-type` | `LT_AUTH_TYPE` | "" | Authentication type: basic, bearer, api-key |
| auth.basic.username | `--auth-username` | `LT_AUTH_BASIC_USERNAME` | "" | Basic auth username |
| auth.basic.password | `--auth-password` | `LT_AUTH_BASIC_PASSWORD` | "" | Basic auth password |
| auth.bearer.token | `--auth-token` | `LT_AUTH_BEARER_TOKEN` | "" | Bearer token |
| auth.apikey.header | `--auth-header` | `LT_AUTH_APIKEY_HEADER` | "" | API key header name |
| auth.apikey.value | `--auth-value` | `LT_AUTH_APIKEY_VALUE` | "" | API key value |
| **HTTP Headers** |
| headers | `-H, --header` | `LT_HEADER_*` | [] | Custom HTTP headers |
| **Logging** |
| log_level | - | - | info | Log level: debug, info, warn, error (config file only) |
| log_console | - | - | true | Log to console (config file only) |
| log_file | - | - | "" | Path to log file, empty = no file logging (config file only) |
| log_max_size | - | - | 100 | Max log file size in MB before rotation (config file only) |
| log_max_backups | - | - | 5 | Number of rotated log files to keep (config file only) |
| **Other** |
| show_config | `--show-config` | - | false | Display current configuration and exit |

### Crawl Depth

`max_depth` bounds how far from the seeds the crawl travels. Seed URLs are depth
0, and `max_depth: N` crawls depth 0 through N inclusive. `0` means unlimited,
matching `limit`. With several seeds a page's depth is its distance from the
nearest one, since all seeds share a single queue.

Setting it changes how the crawl is scheduled: pages are crawled one depth at a
time, and a depth is only opened once every shallower depth is finished,
including its retries. That is what makes the depth exact — without it, a page
first reached down a long path would be recorded at that path's depth, and a
shorter route found later could not correct it, because the page has already
been crawled. The cost is that the slowest page in a depth holds up the start of
the next one.

Links past the bound are still recorded in the link graph as `discovered`; the
bound limits what is fetched, not what is known.

Two consequences worth knowing:

- `pages.depth` is filled in only by a bounded crawl. An unbounded crawl leaves
  it `NULL` rather than storing a first-discovery value that would look
  authoritative without being a shortest path.
- Because of that, `--max-depth` cannot be used on a database that still has
  unfinished pages from an unbounded run, or from a release older than depth
  tracking. That means queued URLs and also failures that still have retries
  left, since the crawler would act on both. Their distance from the seeds was
  never recorded, so the bound has no honest origin to measure from. The run
  stops with an error; start with a new database file, or finish that work
  without `--max-depth` first.

If `limit` is reached first, the crawl stops mid-depth, says so in the log, and
leaves the remaining pages queued for a later resume.

## Configuration File

Create a `linktadoru.yml` file (see [linktadoru.yml.example](../linktadoru.yml.example) for a complete annotated example):

```yaml
# Basic crawling parameters
concurrency: 2               # Number of concurrent workers
request_delay: 0.1           # Delay between requests in seconds (number)
request_timeout: "30s"       # HTTP request timeout (Go duration, e.g. "30s", "1m")
user_agent: "LinkTadoru/1.0"
ignore_robots_txt: false
follow_external_hosts: false # Stay on the seed hosts by default
limit: 0                     # Stop after N pages (0 = unlimited)
max_depth: 0                 # Stop N hops from the seeds (0 = unlimited)
max_response_size: 10485760  # Max response body size in bytes (10 MiB)

# URL filtering (regex; double the backslashes in double-quoted YAML strings)
include_patterns:
  - "^https?://[^/]*httpbin\\.org/.*"
exclude_patterns:
  - "\\.pdf$"
  - "/admin/.*"

# Custom HTTP headers
headers:
  - "Accept: text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"
  - "Accept-Language: en-us,en;q=0.5"

# Storage
database_path: "./linktadoru.db"

# Logging (config file only, see the Logging section)
log_level: "info"
```

## Environment Variables

Options that are bound to a CLI flag can be set via environment variables with the `LT_` prefix (see the table above). Options without a flag — the `log_*` keys, `allowed_schemes`, and `seed_urls` — cannot be set via environment variables; use the configuration file instead (or `--seed-file` for seed URLs).

```bash
# Basic configuration
export LT_CONCURRENCY=2
export LT_REQUEST_DELAY=0.1
export LT_REQUEST_TIMEOUT=30s
export LT_USER_AGENT="MyBot/1.0"
export LT_IGNORE_ROBOTS_TXT=false
export LT_DATABASE_PATH="./mysite.db"
export LT_LIMIT=1000

# Authentication (recommended method, see Authentication below)
export LT_AUTH_TYPE=basic
export LT_AUTH_BASIC_USERNAME="myuser"
export LT_AUTH_BASIC_PASSWORD="mypass"

# Custom HTTP headers (LT_HEADER_<NAME> pattern)
export LT_HEADER_ACCEPT="application/json"
export LT_HEADER_ACCEPT_LANGUAGE="en-US,en;q=0.9"
export LT_HEADER_X_CUSTOM="MyCustomValue"

./linktadoru https://httpbin.org
```

## Logging

Logging is configured in the configuration file only (no CLI flags or environment variables):

```yaml
log_level: "info"    # debug, info, warn, error (default: info)
log_console: true    # Log to console (default: true)
log_file: ""         # Path to log file; empty = no file logging
log_max_size: 100    # Max log file size in MB before rotation (default: 100)
log_max_backups: 5   # Number of rotated log files to keep (default: 5)
```

## Authentication

LinkTadoru supports multiple authentication methods for accessing password-protected websites. Only one method can be active at a time.

### Basic Authentication

```bash
# Environment variables (recommended)
export LT_AUTH_TYPE=basic
export LT_AUTH_BASIC_USERNAME="myuser"
export LT_AUTH_BASIC_PASSWORD="mypass"
./linktadoru https://protected.example.com

# CLI flags (not recommended: visible in process lists and shell history)
./linktadoru --auth-type basic --auth-username myuser --auth-password mypass https://protected.example.com
```

### Bearer Token Authentication

```bash
# Environment variables (recommended)
export LT_AUTH_TYPE=bearer
export LT_AUTH_BEARER_TOKEN="your-jwt-token"
./linktadoru https://api.example.com

# CLI flags
./linktadoru --auth-type bearer --auth-token "your-bearer-token" https://api.example.com
```

### API Key Authentication

```bash
# Environment variables (recommended)
export LT_AUTH_TYPE=api-key
export LT_AUTH_APIKEY_HEADER="X-API-Key"
export LT_AUTH_APIKEY_VALUE="your-api-key-here"
./linktadoru https://api.example.com

# CLI flags
./linktadoru --auth-type api-key --auth-header "X-API-Key" --auth-value "your-api-key" https://api.example.com
```

### Configuration File

Credentials can also be set in `linktadoru.yml` (not recommended for files committed to version control). Each auth block additionally supports `*_env` keys naming a custom environment variable to read the value from — see [linktadoru.yml.example](../linktadoru.yml.example).

```yaml
auth:
  type: "bearer"
  bearer:
    token: "your-token-here"
```

### Security Best Practices

⚠️ **Important Security Notes:**
- Always use environment variables for authentication credentials
- Never include credentials in CLI flags (visible in process lists and shell history)
- Never store credentials in configuration files committed to version control
- Use separate configuration files for different environments (dev/staging/prod)

## Custom HTTP Headers

LinkTadoru supports custom HTTP headers for enhanced compatibility and API access.

**Environment Variables:**
```bash
# Set custom headers using LT_HEADER_* pattern
export LT_HEADER_ACCEPT="application/json"
export LT_HEADER_ACCEPT_LANGUAGE="en-US,en;q=0.9"
export LT_HEADER_X_CUSTOM="MyCustomValue"
./linktadoru https://api.example.com
```

**CLI Flags:**
```bash
./linktadoru -H "Accept: application/json" -H "X-Custom: Value" https://api.example.com
```

**Configuration File:**
```yaml
headers:
  - "Accept: application/json"
  - "Accept-Language: en-US,en;q=0.9"
  - "X-Custom-Header: CustomValue"
```

### Header Restrictions

The following headers cannot be overridden for security and protocol compliance:
- `Host`
- `Content-Length`
- `Connection`

### Combined Authentication and Headers Example

```bash
# API crawling with bearer token and custom headers
export LT_AUTH_TYPE=bearer
export LT_AUTH_BEARER_TOKEN="your-jwt-token"
export LT_HEADER_ACCEPT="application/json"
export LT_HEADER_X_API_VERSION="v1"
./linktadoru https://api.example.com/endpoints
```

## Pattern Matching

### Include Patterns
Only URLs matching at least one include pattern will be crawled:

```yaml
include_patterns:
  - "^https?://[^/]*httpbin\\.org/.*"     # Main domain
  - "^https?://[^/]*\\.httpbin\\.org/.*"  # All subdomains
  - ".*/products/.*"                      # Specific path
```

### Exclude Patterns
URLs matching any exclude pattern will be skipped:

```yaml
exclude_patterns:
  - "\\.pdf$"         # Skip PDFs
  - "\\.jpg$"         # Skip images
  - "/admin/.*"       # Skip admin section
  - ".*\\?.*"         # Skip URLs with query strings
```

Invalid regexes are rejected at startup with a clear error message.

## Performance Tuning

`request_delay` is a number of seconds (e.g. `0.5`), not a duration string.

### Small Sites (< 1,000 pages)
```yaml
concurrency: 5
request_delay: 1
```

### Medium Sites (1,000 - 50,000 pages)
```yaml
concurrency: 10
request_delay: 0.5
```

### Large Sites (> 50,000 pages)

Start around these values and adjust; 20–50 workers with a 0.2–0.5s delay is a reasonable range if the target site tolerates it:

```yaml
concurrency: 20
request_delay: 0.2
```

### Respectful Crawling
```yaml
concurrency: 2
request_delay: 5
ignore_robots_txt: false
user_agent: "PoliteBot/1.0 (https://example.com/bot)"
```
