# LinkTadoru

[![Build Status](https://github.com/fukuda-deltax/linktadoru/actions/workflows/ci.yml/badge.svg)](https://github.com/fukuda-deltax/linktadoru/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/fukuda-deltax/linktadoru)](https://golang.org/doc/devel/release.html)
[![License](https://img.shields.io/github/license/fukuda-deltax/linktadoru)](LICENSE)
[![Latest Release](https://img.shields.io/github/v/release/fukuda-deltax/linktadoru)](https://github.com/fukuda-deltax/linktadoru/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/fukuda-deltax/linktadoru)](https://goreportcard.com/report/github.com/fukuda-deltax/linktadoru)

A high-performance web crawler and link analysis tool built in Go. LinkTadoru discovers and analyzes website structures, extracts metadata, and maps link relationships to understand site architecture and content connections.

## Features

- **Fast Concurrent Crawling**: Configurable worker pool for parallel processing
- **Comprehensive Data Collection**: Extracts page titles, metadata, canonical URLs, and structured data
- **Link Analysis**: Maps internal and external link relationships with anchor text
- **Performance Metrics**: Tracks TTFB, download times, and response sizes
- **Robots.txt Compliance**: Respects robots.txt rules and crawl delays
- **Rate Limiting**: Built-in rate limiter to prevent server overload
- **SQLite Storage**: All data stored in a queryable SQLite database
- **Persistent Queue**: Resumable persistent crawl queue for interrupted sessions
- **Exclusive Control**: Safe concurrent processing in multi-process environments
- **Duplicate Detection**: Content hash-based duplicate page detection
- **Flexible Configuration**: CLI flags, environment variables, or config file

## Installation

### From Source

```bash
# Clone the repository
git clone https://github.com/fukuda-deltax/linktadoru.git
cd linktadoru

# Build using Make (recommended)
make build

# Or build directly with Go
go build -o dist/linktadoru ./cmd/crawler

# Install globally
make install
```

### Cross-Platform Builds

```bash
# Build for all platforms
make build-all

# Build for specific platforms
make build-linux    # Linux (amd64, arm64)
make build-darwin   # macOS (Intel, Apple Silicon)  
make build-windows  # Windows (amd64)

# Create release archives
make release
```


### Requirements

- Go 1.23 or higher
- SQLite3 (included with Go SQLite driver)

## Usage

### Basic Usage

```bash
# Crawl a single website
./dist/linktadoru https://example.com

# Crawl multiple seed URLs
./dist/linktadoru https://example.com https://blog.example.com

# With custom settings
./dist/linktadoru --limit 5000 --concurrency 5 --delay 2s https://example.com
```

### Configuration Options

The crawler can be configured via:
1. Command-line flags (highest priority)
2. Environment variables (prefix: `LT_`)
3. Configuration file (`config.yaml`)
4. Default values

#### Command-Line Flags

```bash
./linktadoru --help

Flags:
  --include-patterns strings   Regex patterns for URLs to include
  -l, --limit int             Stop after N pages (0=unlimited)
  -c, --concurrency int       Number of concurrent workers (default 10)
  --config string             Config file path (default "./config.yaml")
  -d, --database string       SQLite database path (default "./crawl.db")
  -r, --delay duration        Delay between requests (default 1s)
  --exclude-patterns strings  Regex patterns for URLs to exclude
  --ignore-robots             Ignore robots.txt rules
  -t, --timeout duration      HTTP request timeout (default 30s)
  -u, --user-agent string     HTTP User-Agent (default "LinkTadoru/1.0")
```

#### Configuration File

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
  - "\\.pdf$"
  - "/admin/.*"
  - ".*#.*"

# Storage configuration
database_path: "./crawl.db"
```

#### Environment Variables

All configuration options can be set via environment variables:

```bash
export LT_LIMIT=5000
export LT_CONCURRENCY=5
export LT_REQUEST_DELAY=2s
./linktadoru https://example.com
```

## Database Schema

The crawler uses a unified SQLite database design with the following structure:

### Unified Pages Table (Queue + Results)
- **Queue Management**: URLs with status tracking (`queued` → `processing` → `completed`/`error`)
- **Crawl Results**: Title, meta description, performance metrics, HTTP headers
- **Duplicate Prevention**: URL uniqueness enforced at database level
- **NULL Handling**: Result fields remain NULL until crawled

### Links Table
- Source and target URL relationships
- Anchor text and link type (internal/external)
- rel attributes and discovery timestamps

### Crawl Errors Table
- Detailed error tracking with types and messages
- Separate from page-level error status

### Analysis Views
- **completed_pages**: Clean view of successfully crawled pages
- **queue_status**: Real-time queue statistics by status

**Key Benefits:**
- No duplicate URLs in queue
- Atomic operations for thread safety
- Persistent state for resumable crawls
- Efficient analysis through specialized views

## Development

### Building from Source

```bash
# Run tests
make test

# Run linter (requires golangci-lint)
make lint

# Format code
make fmt

# Run all checks before committing
make check

# Clean build artifacts
make clean
```

### Project Structure

```
.
├── cmd/crawler/          # Main application entry point
├── internal/
│   ├── cmd/             # CLI command handling
│   ├── config/          # Configuration management
│   ├── crawler/         # Core crawling logic
│   ├── interfaces/      # Interface definitions
│   ├── parser/          # HTML parsing
│   └── storage/         # Database operations
├── config.yaml.example  # Example configuration
└── go.mod              # Go module definition
```

### Running Tests

```bash
# Run all tests
go test ./...

# With coverage
go test -cover ./...

# Specific package
go test -v ./internal/crawler
```

### Building

```bash
# Build for current platform
go build -o linktadoru ./cmd/crawler

# Cross-compile for Linux
GOOS=linux GOARCH=amd64 go build -o crawler-linux ./cmd/crawler

# Cross-compile for Windows
GOOS=windows GOARCH=amd64 go build -o crawler.exe ./cmd/crawler
```

## Development

### Local Testing of GitHub Actions

Use [act](https://github.com/nektos/act) to test GitHub Actions locally:

```bash
# Install act
brew install act  # macOS
# or follow installation guide for your platform

# Test CI workflow
act push

# Test specific job
act -j test

# Test with environment variables
act --env-file .env
```

See [docs/github-actions-local-testing.md](docs/github-actions-local-testing.md) for detailed setup.

### Versioning and Releases

This project follows [Semantic Versioning](https://semver.org/):

- **Major**: Breaking changes (`1.0.0` → `2.0.0`)
- **Minor**: New features (`1.0.0` → `1.1.0`)
- **Patch**: Bug fixes (`1.0.0` → `1.0.1`)
- **Pre-release**: Beta/RC (`1.1.0-beta.1`)

#### Creating a Release

```bash
# Create and push a version tag
git tag v1.0.0
git push origin v1.0.0
```

This automatically triggers the release workflow, which:
- Runs tests
- Builds binaries for multiple platforms
- Creates GitHub release with assets
- Builds and pushes Docker images

See [docs/versioning-and-releases.md](docs/versioning-and-releases.md) for complete guide.

### Available Binaries

Each release includes binaries for:
- Linux (amd64, arm64)
- macOS (amd64, arm64) 
- Windows (amd64)

File naming: `linktadoru-v1.0.0-linux-amd64`

## Performance Considerations

- **Concurrency**: Adjust `--concurrency` based on target server capacity
- **Rate Limiting**: Use `--delay` to prevent overwhelming the server
- **Memory Usage**: Large sites may require increased memory allocation
- **Database Performance**: SQLite performs well up to millions of pages

## Examples

### Crawl with URL Pattern Matching

```bash
./linktadoru --include-patterns "^https?://[^/]*example\.com/.*" https://example.com
```

### Exclude Specific Patterns

```bash
./linktadoru --exclude-patterns "\.pdf$" --exclude-patterns "/search\?" https://example.com
```

### High-Performance Crawling

```bash
./linktadoru --concurrency 20 --delay 500ms --max-pages 50000 https://example.com
```

### Ignore Robots.txt

```bash
./linktadoru --ignore-robots https://example.com
```

## Troubleshooting

### Common Issues

1. **"Too many open files" error**
   - Increase system file descriptor limit: `ulimit -n 4096`

2. **High memory usage**
   - Reduce concurrency or implement batch processing
   - Use `--batch-limit` to process in chunks

3. **Slow crawling**
   - Increase concurrency (if server allows)
   - Reduce request delay
   - Check network connectivity

### Debug Mode

Enable verbose logging by setting the log level:

```bash
LOG_LEVEL=debug ./linktadoru https://example.com
```

## Documentation and Examples

For detailed usage examples and advanced configuration:

- 📖 **[Basic Usage Examples](docs/basic-usage.md)** - Command-line usage, configuration files, Docker setup
- 🔧 **[Configuration Reference](config.yaml.example)** - Complete configuration options
- 🏗️ **[Technical Specification](docs/technical-specification.md)** - Architecture and implementation details

## Docker Support

### Quick Start with Docker

```bash
# Build image
docker build -t linktadoru .

# Run with persistent data
docker run -v $(pwd)/data:/app/data linktadoru --database /app/data/crawl.db --limit 10 https://example.com
```

### Development with DevContainer

This project includes VS Code DevContainer support for consistent development environments:

```bash
# Open in VS Code with DevContainer extension
code .
# VS Code will prompt to reopen in container
```

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

## License

LinkTadoru is licensed under the Apache License, Version 2.0 - see the LICENSE file for details.

## Acknowledgments

- Built with [Cobra](https://github.com/spf13/cobra) for CLI
- Uses [Viper](https://github.com/spf13/viper) for configuration
- SQLite storage via [go-sqlite3](https://github.com/mattn/go-sqlite3)
