# LinkTadoru

[![Build Status](https://github.com/masahif/linktadoru/actions/workflows/ci.yml/badge.svg)](https://github.com/masahif/linktadoru/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/masahif/linktadoru)](https://golang.org/doc/devel/release.html)
[![License](https://img.shields.io/github/license/masahif/linktadoru)](LICENSE)
[![Latest Release](https://img.shields.io/github/v/release/masahif/linktadoru)](https://github.com/masahif/linktadoru/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/masahif/linktadoru)](https://goreportcard.com/report/github.com/masahif/linktadoru)

A high-performance web crawler and link analysis tool built in Go.

## Features

- **Fast Concurrent Crawling**: Configurable worker pool for parallel processing
- **Link Analysis**: Maps internal and external link relationships
- **Multiple Authentication Methods**: Support for Basic Auth, Bearer tokens, and API keys
- **Custom HTTP Headers**: Set custom headers for requests
- **Robots.txt Compliance**: Respects robots.txt rules and crawl delays
- **SQLite Storage**: All data stored in a queryable SQLite database
- **Resumable**: Persistent queue for interrupted sessions
- **Flexible Configuration**: CLI flags, environment variables, or config file with hierarchical support

## Installation

### Download Binary

Download pre-built binaries from the [releases page](https://github.com/masahif/linktadoru/releases).

### Build from Source

```bash
git clone https://github.com/masahif/linktadoru.git
cd linktadoru
make build
```

Requirements: Go 1.23+

## Quick Start

```bash
# Crawl a website
./linktadoru https://httpbin.org

# With options
./linktadoru --limit 100 --concurrency 5 https://httpbin.org

# Using config file
./linktadoru --config linktadoru.yml https://httpbin.org

# View current configuration
./linktadoru --show-config

# With custom headers
./linktadoru -H "Accept: application/json" -H "X-Custom: value" https://api.example.com
```

## Documentation

- 📖 **[Basic Usage](docs/basic-usage.md)** - Command-line usage and examples
- 🔧 **[Configuration](docs/configuration.md)** - All configuration options
- 🏗️ **[Technical Details](docs/technical-specification.md)** - Architecture and internals
- 🚀 **[Development](docs/development.md)** - Building and contributing

## Configuration

LinkTadoru follows a hierarchical configuration priority:
1. Command-line arguments (highest priority)
2. Environment variables
3. Configuration file
4. Default values (lowest priority)

### Configuration File

```yaml
# linktadoru.yml
concurrency: 2
request_delay: 0.1           # seconds
user_agent: "LinkTadoru/1.0"
ignore_robots: false
database_path: "./linktadoru.db"
limit: 0                    # 0 = unlimited

# URL filtering
include_patterns: []
exclude_patterns:
  - "\.pdf$"
  - "/admin/.*"

# Authentication (choose one method)
auth:
  type: "basic"             # "basic", "bearer", or "api-key"
  basic:
    username: "user"
    password: "pass"

# Custom HTTP headers
headers:
  - "Accept: application/json"
  - "X-Custom-Header: value"
```

### Environment Variables

All configuration can be set via environment variables with `LT_` prefix:

```bash
# Basic settings
export LT_CONCURRENCY=2
export LT_REQUEST_DELAY=0.5
export LT_IGNORE_ROBOTS=true

# Hierarchical settings (using underscores)
export LT_AUTH_TYPE=basic
export LT_AUTH_BASIC_USERNAME=myuser
export LT_AUTH_BASIC_PASSWORD=mypass

# HTTP headers
export LT_HEADER_ACCEPT="application/json"
export LT_HEADER_X_CUSTOM="value"

./linktadoru https://httpbin.org
```

## Authentication

LinkTadoru supports multiple authentication methods for accessing protected resources.

### Basic Authentication

```bash
# CLI flags
./linktadoru --auth-type basic --auth-username user --auth-password pass https://protected.httpbin.org

# Environment variables (recommended)
export LT_AUTH_TYPE=basic
export LT_AUTH_BASIC_USERNAME=myuser
export LT_AUTH_BASIC_PASSWORD=mypass
./linktadoru https://protected.httpbin.org
```

### Bearer Token Authentication

```bash
# CLI flags
./linktadoru --auth-type bearer --auth-token "your-bearer-token" https://api.example.com

# Environment variables (recommended)
export LT_AUTH_TYPE=bearer
export LT_AUTH_BEARER_TOKEN=your-bearer-token-here
./linktadoru https://api.example.com
```

### API Key Authentication

```bash
# CLI flags
./linktadoru --auth-type api-key --auth-header "X-API-Key" --auth-value "your-key" https://api.example.com

# Environment variables (recommended)
export LT_AUTH_TYPE=api-key
export LT_AUTH_APIKEY_HEADER=X-API-Key
export LT_AUTH_APIKEY_VALUE=your-api-key-here
./linktadoru https://api.example.com
```

### Configuration File

```yaml
# linktadoru.yml
auth:
  type: "bearer"
  bearer:
    token: "your-token-here"
    # Or use environment variable:
    # token_env: "MY_BEARER_TOKEN"
```

## Custom HTTP Headers

Set custom HTTP headers for all requests:

```bash
# CLI flags
./linktadoru -H "Accept: application/json" -H "X-Custom: value" https://api.example.com

# Environment variables
export LT_HEADER_ACCEPT="application/json"
export LT_HEADER_X_API_VERSION="v1"
./linktadoru https://api.example.com
```

**Security Note**: For security reasons, it's recommended to use environment variables rather than storing credentials in configuration files.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

Apache License 2.0 - see [LICENSE](LICENSE) file.