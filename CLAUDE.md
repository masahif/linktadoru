# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

LinkTadoru is a high-performance web crawler and link analysis tool written in Go. The project uses a modular architecture with concurrent workers, persistent SQLite storage, and supports multiple authentication methods.

**Requirements**: Go 1.23+ (uses Go 1.23.11 toolchain)

## Build Commands

```bash
# Standard build (creates dist/linktadoru)
make build

# Run tests with coverage
make test

# Run linting 
make lint

# Format code
make fmt

# Run all quality checks (fmt + lint + test)
make check

# Run CI pipeline (deps + check + build) 
make ci

# Build for all platforms
make build-all

# Build platform-specific binaries
make release-linux    # Linux (amd64 + arm64)
make release-darwin   # macOS (ARM64 only) 
make release-windows  # Windows (amd64)

# Build all release binaries (no archives)
make release

# Install/uninstall binary
make install          # Install to GOPATH/bin
make uninstall        # Remove from GOPATH/bin

# Clean build artifacts and test files
make clean

# Install dependencies
make deps

# Show all available targets
make help
```

### Testing Commands

```bash
# Run all tests
go test ./...

# Test specific package with verbose output
go test -v ./internal/crawler

# Run tests with race detection
go test -race ./...

# Run short tests only
go test -short ./...

# Test with coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Run benchmarks
make bench

# Clean test artifacts and temporary files
make test-clean

# Run tests with different coverage options
make test-coverage    # Generate and view coverage report
make test-ci          # CI-compatible test output
```

### Local CI Testing

The project supports local GitHub Actions testing with `act`:

```bash
# Test full CI workflow
act -W .github/workflows/ci.yml

# Test specific job
act -W .github/workflows/ci.yml -j test
```

## Architecture Overview

### Core Components

1. **CLI Layer** (`internal/cmd/`)
   - Uses Cobra for command-line interface
   - Viper for hierarchical configuration (CLI flags → env vars → config file → defaults)
   - Configuration priority: CLI flags > Environment variables > Config file > Defaults

2. **Configuration System** (`internal/config/`)
   - `CrawlConfig` struct centralizes all settings
   - Supports multiple authentication types: Basic, Bearer token, API key
   - Hierarchical environment variables with `LT_` prefix (e.g., `LT_AUTH_BASIC_USERNAME`)
   - Custom HTTP headers via `LT_HEADER_*` env vars

3. **Crawler Engine** (`internal/crawler/`)
   - `DefaultCrawler` implements worker pool pattern
   - Concurrent workers with configurable limits and delays
   - Rate limiting per domain with robots.txt compliance
   - URL filtering with include/exclude regex patterns

4. **Storage Layer** (`internal/storage/`)
   - SQLite-based persistent storage via `SQLiteStorage`
   - Queue management for resumable crawling
   - Stores pages, links, and error information
   - Generated columns for JSON data extraction

5. **Parser** (`internal/parser/`)
   - HTML parsing for link extraction and metadata
   - Handles base URL resolution and canonical URLs

### Key Design Patterns

- **Interface-based architecture**: Core components implement interfaces for testability
- **Worker pool**: Crawler uses configurable concurrent workers
- **Persistent queue**: SQLite queue enables resuming interrupted crawls
- **Rate limiting**: Respects robots.txt and implements per-domain delays
- **Hierarchical configuration**: Multiple configuration sources with clear precedence

## Configuration System

The configuration system follows this priority order:
1. Command-line flags (highest)
2. Environment variables (`LT_` prefix)
3. Configuration file (`linktadoru.yml`)
4. Default values (lowest)

### Environment Variable Patterns

- Basic settings: `LT_CONCURRENCY`, `LT_REQUEST_DELAY`, `LT_DATABASE_PATH`
- Authentication: `LT_AUTH_TYPE`, `LT_AUTH_BASIC_USERNAME`, `LT_AUTH_BEARER_TOKEN`
- Headers: `LT_HEADER_ACCEPT`, `LT_HEADER_X_CUSTOM` (converts underscores to hyphens)

### Configuration Examples

**Basic crawling with environment variables:**
```bash
export LT_CONCURRENCY=5
export LT_REQUEST_DELAY=2s
export LT_DATABASE_PATH=./my-crawl.db
./linktadoru https://example.com
```

**Basic Authentication:**
```bash
export LT_AUTH_TYPE=basic
export LT_AUTH_BASIC_USERNAME=testuser
export LT_AUTH_BASIC_PASSWORD=testpass
export LT_CONCURRENCY=5
./linktadoru https://example.com
```

**Bearer Token Authentication:**
```bash
export LT_AUTH_TYPE=bearer
export LT_AUTH_BEARER_TOKEN=mytoken123
export LT_HEADER_0="Accept: application/json"
./linktadoru https://api.example.com
```

**API Key Authentication:**
```bash
export LT_AUTH_TYPE=api-key
export LT_AUTH_APIKEY_HEADER=X-API-Key
export LT_AUTH_APIKEY_VALUE=key123
export LT_HEADER_0="Accept: application/json"
export LT_HEADER_1="X-Custom: Value"
./linktadoru https://api.example.com
```

## Database Schema

The SQLite database contains these main tables:
- `pages`: Crawled page data with metadata
- `links`: Link relationships between pages  
- `errors`: Error tracking and reporting
- `queue`: Work queue for resumable crawling
- `meta`: Key-value metadata storage

## Testing Strategy

- **Unit tests**: Each package has comprehensive test coverage
- **Integration tests**: End-to-end crawler functionality tests
- **Mock interfaces**: Use interfaces for dependency injection and testing
- **Test databases**: Use temporary SQLite files (`test_*.db`) for isolation

## Key Dependencies

- **CLI**: `github.com/spf13/cobra` + `github.com/spf13/viper`
- **Database**: `modernc.org/sqlite` (pure Go SQLite)
- **HTTP**: `golang.org/x/net` for HTML parsing
- **Rate limiting**: `golang.org/x/time/rate`

## Development Workflow

1. Create feature branch from `main`
2. Make changes with tests
3. Run `make check` or use `act` for local CI testing
4. Use conventional commit messages (`feat:`, `fix:`, `docs:`, etc.)
5. Create PR - CI runs automatically on PR creation

### Debugging

- Use `--show-config` flag to inspect effective configuration
- Enable debug logging with `LOG_LEVEL=debug`
- Query SQLite database directly: `sqlite3 crawl.db`
- Use pprof endpoints for performance profiling

### Development Tools

**Quick testing and development:**
```bash
make run              # Build and run with --help
make help             # Show all available make targets
make check            # Run all quality checks (fmt + vet + lint + test)
make vet              # Run go vet only
```

**Configuration debugging:**
```bash
./linktadoru --show-config                    # Show effective configuration
LT_CONCURRENCY=5 ./linktadoru --show-config   # Test env var override
```

**Binary usage examples:**
```bash
# Basic usage
./linktadoru https://httpbin.org

# With options  
./linktadoru --limit 100 --concurrency 5 https://httpbin.org

# Using config file
./linktadoru --config linktadoru.yml https://httpbin.org

# Resume interrupted crawl
./linktadoru --database existing-crawl.db https://httpbin.org

# Limit crawl scope with patterns
./linktadoru --include-pattern ".*\\.html$" --exclude-pattern "/admin/.*" https://example.com
```

## Important Files

- `cmd/crawler/main.go`: Application entry point
- `internal/cmd/root.go`: CLI command definition and configuration binding
- `internal/config/config.go`: Configuration struct and validation
- `internal/crawler/crawler.go`: Main crawler implementation
- `internal/storage/sqlite.go`: Database operations
- `linktadoru.yml.example`: Configuration template
- `Makefile`: Build and development commands