# Development Guide

## Prerequisites

- Go 1.23 or higher
- Make (optional but recommended)
- golangci-lint (for linting)

## Project Structure

```
.
├── cmd/crawler/          # Main application entry point
├── internal/
│   ├── cmd/             # CLI command handling
│   ├── config/          # Configuration management
│   ├── crawler/         # Core crawling logic
│   ├── parser/          # HTML parsing
│   └── storage/         # Database operations
├── docs/                # Documentation
├── .github/workflows/   # GitHub Actions CI/CD
└── config.yaml.example  # Example configuration
```

## Building

### Using Make

```bash
# Build for current platform
make build

# Build for all platforms
make build-all

# Specific platforms
make build-linux
make build-darwin
make build-windows

# Install locally
make install
```

### Manual Build

```bash
# Build binary
go build -o linktadoru ./cmd/crawler

# With version information
go build -ldflags "-X main.Version=v1.0.0 -X main.BuildTime=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
  -o linktadoru ./cmd/crawler

# Cross-compilation
GOOS=linux GOARCH=amd64 go build -o linktadoru-linux-amd64 ./cmd/crawler
GOOS=darwin GOARCH=arm64 go build -o linktadoru-darwin-arm64 ./cmd/crawler
GOOS=windows GOARCH=amd64 go build -o linktadoru.exe ./cmd/crawler
```

## Testing

### Run Tests

```bash
# All tests
make test

# With coverage
make test-coverage

# Specific package
go test -v ./internal/crawler

# With race detection
go test -race ./...
```

### Test Coverage

```bash
# Generate coverage report
make test-coverage

# View HTML report
go tool cover -html=coverage.out
```

## Code Quality

### Linting

```bash
# Run linter
make lint

# Auto-fix issues
golangci-lint run --fix
```

### Formatting

```bash
# Format code
make fmt

# Check formatting
gofmt -l .
```

## Development Workflow

### 1. Create Feature Branch

```bash
git checkout -b feature/your-feature
```

### 2. Make Changes

Follow the coding standards:
- Use meaningful variable names
- Add comments for exported functions
- Write tests for new functionality
- Keep functions small and focused

### 3. Run Checks

#### Local Testing (Recommended)
```bash
# Install act (if not already installed)
brew install act  # macOS
# or follow docs/github-actions-local-testing.md

# Run CI locally with act
act -W .github/workflows/ci.yml

# Test specific job
act -W .github/workflows/ci.yml -j test
```

#### Traditional Testing
```bash
# Run all checks
make check

# Individual checks
make fmt
make lint
make test
```

### 4. Commit Changes

```bash
git add .
git commit -m "feat: add new feature"
```

Follow conventional commits:
- `feat:` New feature
- `fix:` Bug fix
- `docs:` Documentation
- `style:` Formatting
- `refactor:` Code restructuring
- `test:` Tests
- `chore:` Maintenance

### 5. Push and Create PR

```bash
git push origin feature/your-feature

# Create PR with GitHub CLI
gh pr create --title "feat: your feature" --body "Description of changes"
```

**CI Strategy**: 
- ✅ **PR → main**: 自動でCI実行
- ❌ **push → main**: CI実行されません  
- 🔧 **手動実行**: `gh workflow run CI --ref main`

詳細は [github-actions-local-testing.md](github-actions-local-testing.md) を参照。

Create a pull request on GitHub. CI will run automatically.

## Local GitHub Actions Testing

Use [act](https://github.com/nektos/act) to test workflows locally:

```bash
# Install act
brew install act  # macOS
# or see https://github.com/nektos/act#installation

# Test push event
act push

# Test pull request
act pull_request

# Test specific job
act -j test
```

**Note**: You must be in a git repository for act to work.


## Database Management

### Schema Updates

When modifying the database schema:

1. Update schema in `internal/storage/schema.go`
2. Add migration logic if needed
3. Update tests
4. Document changes

### Debugging Database

```bash
# Open database
sqlite3 crawl.db

# Common queries
.tables
.schema pages
SELECT COUNT(*) FROM pages;
SELECT status, COUNT(*) FROM pages GROUP BY status;
```

## Performance Profiling

### CPU Profiling

```go
import _ "net/http/pprof"

// In main()
go func() {
    log.Println(http.ListenAndServe("localhost:6060", nil))
}()
```

```bash
# Generate profile
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30

# Analyze
go tool pprof -http=:8080 profile.out
```

### Memory Profiling

```bash
# Heap profile
go tool pprof http://localhost:6060/debug/pprof/heap

# Goroutine profile
go tool pprof http://localhost:6060/debug/pprof/goroutine
```

## Release Process

### 1. Update Version

Update version in relevant files if needed.

### 2. Create Tag

```bash
git tag v1.0.0
git push origin v1.0.0
```

### 3. GitHub Actions

The release workflow automatically:
- Runs tests
- Builds binaries for all platforms
- Creates GitHub release
- Uploads artifacts

## Troubleshooting

### Common Issues

1. **Module errors**: Run `go mod tidy`
2. **Lint failures**: Run `golangci-lint run --fix`
3. **Test failures**: Check recent changes, run with `-v` flag
4. **Build errors**: Ensure Go 1.23+ is installed

### Debug Mode

Enable debug logging:

```bash
LOG_LEVEL=debug ./linktadoru https://httpbin.org
```

## Contributing

See [CONTRIBUTING.md](../CONTRIBUTING.md) for contribution guidelines.