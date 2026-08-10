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
- **Safe by Default**: Stays on the seed origins unless an absolute URL range is added with `include_patterns`, and caps response bodies at 10 MiB (`max_response_size`)
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

# Generated seed list: each seed plus its direct links
./linktadoru --seed-file urls.txt --max-depth 1 --limit 0

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
ignore_robots_txt: false
database_path: "./linktadoru.db"
limit: 0                     # 0 = unlimited
max_depth: 0                 # 0 = unlimited; positive N includes discovery depth N

# URL filtering
include_patterns: []
exclude_patterns:
  - "\\.pdf$"
  - "/admin/.*"

# Custom HTTP headers
headers:
  - "Accept: application/json"
  - "X-Custom-Header: value"
```

See [linktadoru.yml.example](linktadoru.yml.example) for a complete annotated example and the [Configuration Reference](docs/configuration.md) for every option.

### Environment Variables

Options that have a CLI flag can also be set via environment variables with the `LT_` prefix. Logging options (`log_*`) and `allowed_schemes` have no flags and can only be set in the configuration file.

```bash
# Basic settings
export LT_CONCURRENCY=2
export LT_REQUEST_DELAY=0.5
export LT_IGNORE_ROBOTS_TXT=true

# HTTP headers (LT_HEADER_* pattern)
export LT_HEADER_ACCEPT="application/json"

./linktadoru https://httpbin.org
```

## Authentication and Custom Headers

LinkTadoru supports Basic Auth, Bearer tokens, and API keys, plus custom HTTP headers for all requests. Example:

```bash
# Environment variables (recommended for credentials)
export LT_AUTH_TYPE=basic
export LT_AUTH_BASIC_USERNAME=myuser
export LT_AUTH_BASIC_PASSWORD=mypass
./linktadoru -H "Accept: application/json" https://protected.example.com
```

See [Configuration Reference — Authentication](docs/configuration.md#authentication) for all methods (CLI flags, environment variables, config file) and header options.

**Security Note**: For security reasons, it's recommended to use environment variables rather than storing credentials in configuration files.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

Apache License 2.0 - see [LICENSE](LICENSE) file.
