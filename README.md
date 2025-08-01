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
- **Robots.txt Compliance**: Respects robots.txt rules and crawl delays
- **SQLite Storage**: All data stored in a queryable SQLite database
- **Resumable**: Persistent queue for interrupted sessions
- **Flexible Configuration**: CLI flags, environment variables, or config file

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
./linktadoru --config mysite.yaml https://httpbin.org
```

## Documentation

- 📖 **[Basic Usage](docs/basic-usage.md)** - Command-line usage and examples
- 🔧 **[Configuration](docs/configuration.md)** - All configuration options
- 🏗️ **[Technical Details](docs/technical-specification.md)** - Architecture and internals
- 🚀 **[Development](docs/development.md)** - Building and contributing

## Configuration

```yaml
# config.yaml
concurrency: 10
request_delay: 1s
user_agent: "MyBot/1.0"
respect_robots: true
database_path: "./crawl.db"
```

Or use environment variables:
```bash
export LT_CONCURRENCY=5
export LT_REQUEST_DELAY=2s
./linktadoru https://httpbin.org
```

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

## License

Apache License 2.0 - see [LICENSE](LICENSE) file.