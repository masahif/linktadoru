# Development Guide

## Prerequisites

- Go 1.23 or higher
- Make (optional but recommended)
- golangci-lint (for linting)
- Docker (for containerized development)
- act (optional, for local GitHub Actions testing)

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

# Generate coverage report  
make test-coverage

# Specific package
go test -v ./internal/crawler

# With race detection
go test -race ./...

# View HTML coverage report
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

## Development Environments

### Local Development (Traditional)

Standard Go development with local tools:
```bash
# Install prerequisites
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Run development commands
make test
make lint
make check
```

### VS Code DevContainer (Recommended)

For the best integrated development experience:

**Prerequisites:**
- VS Code
- Docker Desktop  
- Dev Containers extension

**Usage:**
1. Open project in VS Code
2. Command Palette (`Ctrl+Shift+P` / `Cmd+Shift+P`)
3. "Dev Containers: Reopen in Container"
4. Wait for container build (first time only)
5. All development tools ready to use

**Benefits:**
- Complete toolchain pre-installed
- VS Code integration (debugging, IntelliSense)
- Consistent environment across team members

### Docker-based Development (Command Line)

For consistent environment without VS Code:

```bash
# Build development image
docker build -t linktadoru-dev .devcontainer/

# Run tests
docker run --rm -v $(pwd):/workspace linktadoru-dev make test

# Run quality checks
docker run --rm -v $(pwd):/workspace linktadoru-dev make check

# Interactive shell
docker run -it --rm -v $(pwd):/workspace linktadoru-dev bash

# Local GitHub Actions testing
docker run --rm -v $(pwd):/workspace -v /var/run/docker.sock:/var/run/docker.sock linktadoru-dev act
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

#### Traditional Testing
```bash
# Run all checks
make check

# Individual checks
make fmt
make lint
make test
```

#### Local CI Testing (Advanced)
```bash
# Install act (if not already installed)
brew install act  # macOS
# or follow docs/github-actions-local-testing.md

# Run CI locally with act
make act

# Test specific job
make act-test

# List available workflows
make act-list
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
- ✅ **PR → main**: CI runs automatically
- ❌ **push → main**: CI does not run automatically  
- 🔧 **Manual trigger**: `gh workflow run CI --ref main`

See [github-actions-local-testing.md](github-actions-local-testing.md) for details.

Create a pull request on GitHub. CI will run automatically.

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

Enable pprof by importing `_ "net/http/pprof"` and starting HTTP server in main().

```bash
# CPU profile
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30

# Memory profile  
go tool pprof http://localhost:6060/debug/pprof/heap

# View in browser
go tool pprof -http=:8080 profile.out
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