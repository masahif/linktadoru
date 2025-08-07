# Configuration Reference

LinkTadoru can be configured through multiple methods with the following priority:

1. Command-line flags (highest priority)
2. Environment variables (prefix: `LT_`)
3. Configuration file (`linktadoru.yml`)
4. Default values (lowest priority)

## Command-Line Flags

```bash
./linktadoru --help

Flags:
      --auth-header string         API key header name (e.g., X-API-Key)
      --auth-password string       Password for basic authentication
      --auth-token string          Bearer token for authorization header
      --auth-type string           Authentication type: 'basic', 'bearer', or 'api-key'
      --auth-username string       Username for basic authentication
      --auth-value string          API key header value
  -c, --concurrency int            Number of concurrent workers (default 2)
      --config string              config file (default is ./linktadoru.yml)
  -d, --database string            Path to SQLite database file (default "./linktadoru.db")
  -r, --delay float                Delay between requests in seconds (default 0.1)
      --exclude-patterns strings   Regex patterns for URLs to exclude
  -H, --header strings             Custom HTTP headers in 'Name: Value' format (use multiple times for multiple headers)
  -h, --help                       help for linktadoru
      --ignore-robots              Ignore robots.txt rules
      --include-patterns strings   Regex patterns for URLs to include
  -l, --limit int                  Stop after N pages (0=unlimited)
      --show-config                Display current configuration in YAML format and exit
  -t, --timeout duration           HTTP request timeout (default 30s)
  -u, --user-agent string          HTTP User-Agent header (default "LinkTadoru/1.0")
  -v, --version                    version for linktadoru
```

## Configuration File

Create a `linktadoru.yml` file:

```yaml
# Basic crawling parameters (updated defaults)
concurrency: 2              # Number of concurrent workers (default: 2, was 10)
request_delay: 0.1           # Delay between requests in seconds (default: 0.1, was 1.0)
request_timeout: 30.0        # HTTP request timeout in seconds
user_agent: "LinkTadoru/1.0" # User-Agent header
ignore_robots: false        # Whether to ignore robots.txt rules
limit: 0                    # Stop after N pages (0 = unlimited)

# Authentication configuration
auth:
  type: ""                  # Authentication type: "", "basic", "bearer", or "api-key"
  basic:
    username: "user"        # Basic auth username
    password: "pass"        # Basic auth password
  bearer:
    token: "your_token_here"  # Bearer token
  apikey:
    header: "X-API-Key"     # Header name for API key
    value: "your_key_here"  # API key value

# Custom HTTP headers
headers:
  - "Accept: text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8"
  - "Accept-Language: en-us,en;q=0.5"
  # Add custom headers as needed

# URL filtering
include_patterns:
  - "^https?://[^/]*httpbin\.org/.*"
  - "^https?://[^/]*subdomain\.httpbin\.org/.*"

exclude_patterns:
  - "\.pdf$"
  - "/admin/.*"
  - ".*#.*"

# Storage configuration
database_path: "./linktadoru.db"
```

## Environment Variables

All configuration options can be set via environment variables with the `LT_` prefix:

```bash
# Basic configuration
export LT_CONCURRENCY=2
export LT_REQUEST_DELAY=0.1
export LT_REQUEST_TIMEOUT=30.0
export LT_USER_AGENT="MyBot/1.0"
export LT_IGNORE_ROBOTS=false
export LT_DATABASE_PATH="./mysite.db"
export LT_LIMIT=1000

# Authentication (recommended method)
export LT_AUTH_TYPE=basic
export LT_AUTH_BASIC_USERNAME="myuser"
export LT_AUTH_BASIC_PASSWORD="mypass"

# Bearer token authentication
export LT_AUTH_TYPE=bearer
export LT_AUTH_BEARER_TOKEN="your-jwt-token"

# API key authentication
export LT_AUTH_TYPE=api-key
export LT_AUTH_APIKEY_HEADER="X-API-Key"
export LT_AUTH_APIKEY_VALUE="your-api-key"

# Custom HTTP headers (hierarchical)
export LT_HEADER_ACCEPT="application/json"
export LT_HEADER_ACCEPT_LANGUAGE="en-US,en;q=0.9"
export LT_HEADER_X_CUSTOM="MyCustomValue"

./linktadoru https://httpbin.org
```

## Configuration Options

| Option | CLI Flag | Environment Variable | Default | Description |
|--------|----------|---------------------|---------|-------------|
| **Authentication** |
| auth_type | `--auth-type` | `LT_AUTH_TYPE` | "" | Authentication type: basic, bearer, api-key |
| auth_username | `--auth-username` | `LT_AUTH_BASIC_USERNAME` | "" | Basic auth username |
| auth_password | `--auth-password` | `LT_AUTH_BASIC_PASSWORD` | "" | Basic auth password |
| auth_token | `--auth-token` | `LT_AUTH_BEARER_TOKEN` | "" | Bearer token |
| auth_header | `--auth-header` | `LT_AUTH_APIKEY_HEADER` | "" | API key header name |
| auth_value | `--auth-value` | `LT_AUTH_APIKEY_VALUE` | "" | API key value |
| **HTTP Headers** |
| headers | `-H, --header` | `LT_HEADER_*` | [] | Custom HTTP headers |
| **Basic Settings** |
| concurrency | `-c, --concurrency` | `LT_CONCURRENCY` | 2 | Number of concurrent workers |
| request_delay | `-r, --delay` | `LT_REQUEST_DELAY` | 0.1 | Delay between requests in seconds |
| request_timeout | `-t, --timeout` | `LT_REQUEST_TIMEOUT` | 30s | HTTP request timeout |
| user_agent | `-u, --user-agent` | `LT_USER_AGENT` | LinkTadoru/1.0 | HTTP User-Agent header |
| ignore_robots | `--ignore-robots` | `LT_IGNORE_ROBOTS` | false | Ignore robots.txt rules |
| limit | `-l, --limit` | `LT_LIMIT` | 0 | Maximum pages to crawl (0=unlimited) |
| database_path | `-d, --database` | `LT_DATABASE_PATH` | ./linktadoru.db | SQLite database file path |
| **URL Filtering** |
| include_patterns | `--include-patterns` | `LT_INCLUDE_PATTERNS` | [] | URL patterns to include (regex) |
| exclude_patterns | `--exclude-patterns` | `LT_EXCLUDE_PATTERNS` | [] | URL patterns to exclude (regex) |
| **Other** |
| show_config | `--show-config` | - | false | Display current configuration and exit |

## Authentication

LinkTadoru supports multiple authentication methods for accessing password-protected websites:

- **Basic Authentication**: Standard HTTP Basic Auth with username/password
- **Bearer Token**: Authorization header with bearer token (OAuth, JWT, etc.)
- **API Key**: Custom header with API key

### Authentication Types

#### Basic Authentication

**Environment Variables (Recommended):**
```bash
# Using default environment variables
export LT_AUTH_TYPE=basic
export LT_AUTH_BASIC_USERNAME="myuser" 
export LT_AUTH_BASIC_PASSWORD="mypass"
./linktadoru https://protected.example.com

# Alternative: use CLI flags to specify custom env vars
export MY_USER="myuser"
export MY_PASS="mypass"
./linktadoru --auth-type basic --auth-username=$MY_USER --auth-password=$MY_PASS https://protected.example.com
```

**CLI Flags (Not Recommended for Production):**
```bash
./linktadoru --auth-type basic --auth-username myuser --auth-password mypass https://protected.example.com
```

**Configuration File (Not Recommended):**
```yaml
# linktadoru.yml - NOT recommended for security reasons
auth:
  type: basic
  basic:
    username: "myuser"
    password: "mypass"
```

#### Bearer Token Authentication

**Environment Variables (Recommended):**
```bash
export LT_AUTH_TYPE=bearer
export LT_AUTH_BEARER_TOKEN="eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."
./linktadoru https://api.example.com
```

**CLI Flags:**
```bash
./linktadoru --auth-type bearer --auth-token "your-bearer-token" https://api.example.com
```

#### API Key Authentication

**Environment Variables (Recommended):**
```bash
export LT_AUTH_TYPE=api-key
export LT_AUTH_APIKEY_HEADER="X-API-Key"
export LT_AUTH_APIKEY_VALUE="your-api-key-here"
./linktadoru https://api.example.com
```

**CLI Flags:**
```bash
./linktadoru --auth-type api-key --auth-header "X-API-Key" --auth-value "your-api-key" https://api.example.com
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
- `Transfer-Encoding`

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
ignore_robots: false
user_agent: "PoliteBot/1.0 (https://httpbin.org/bot)"
```