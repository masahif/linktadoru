# LinkTadoru Makefile
# Build automation for cross-platform compilation

# Binary name
BINARY_NAME=linktadoru
VERSION=$(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME=$(shell date -u '+%Y-%m-%d_%H:%M:%S')

# Go build flags
LDFLAGS=-ldflags "-X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME} -s -w"
GOFLAGS=-trimpath

# Directories
DIST_DIR=dist
CMD_DIR=cmd/crawler

# Default target
.DEFAULT_GOAL := help

## help: Show this help message
.PHONY: help
help:
	@echo 'Usage:'
	@echo '  make <target>'
	@echo ''
	@echo 'Targets:'
	@awk 'BEGIN {FS = ": "} /^## / {sub(/^## /, "", $$1); printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

## build: Build binary for current platform
.PHONY: build
build:
	@echo "Building ${BINARY_NAME} for current platform..."
	@mkdir -p ${DIST_DIR}
	go build ${GOFLAGS} ${LDFLAGS} -o ${DIST_DIR}/${BINARY_NAME} ./${CMD_DIR}
	@echo "Binary built: ${DIST_DIR}/${BINARY_NAME}"

## test: Run tests with coverage (CI-compatible)
.PHONY: test
test:
	@echo "Running tests..."
	go test -v -race -timeout 10m -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -html=coverage.out -o coverage.html

## test-coverage: Generate and view test coverage report
.PHONY: test-coverage
test-coverage:
	@echo "Generating test coverage report..."
	go test -v -race -timeout 10m -coverprofile=coverage.out -covermode=atomic ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report generated: coverage.html"

## test-short: Run short tests
.PHONY: test-short
test-short:
	@echo "Running short tests..."
	go test -short -v ./...

## bench: Run benchmarks
.PHONY: bench
bench:
	@echo "Running benchmarks..."
	go test -bench=. -benchmem ./...

## test-clean: Clean test artifacts and temporary files
.PHONY: test-clean
test-clean:
	@echo "Cleaning test artifacts..."
	rm -f test_*.db*
	rm -f *.test
	rm -f coverage.out coverage.html coverage.txt
	rm -f complexity-report.txt

## lint: Run golangci-lint
.PHONY: lint
lint:
	@echo "Running linter..."
	golangci-lint run ./...

## fmt: Format code
.PHONY: fmt
fmt:
	@echo "Formatting code..."
	go fmt ./...
	gofmt -s -w .

## vet: Run go vet
.PHONY: vet
vet:
	@echo "Running go vet..."
	go vet ./...

## cyclo: Check cyclomatic complexity (warning only)
.PHONY: cyclo
cyclo:
	@echo "Checking cyclomatic complexity..."
	@if command -v gocyclo >/dev/null 2>&1; then \
		if gocyclo -over 15 . >/dev/null 2>&1; then \
			echo "✓ All functions have acceptable complexity (≤15)"; \
		else \
			echo "⚠ Warning: Some functions exceed complexity threshold of 15:"; \
			gocyclo -over 15 .; \
			echo "Consider refactoring complex functions for better maintainability."; \
		fi \
	else \
		echo "gocyclo not installed. Install with: go install github.com/fzipp/gocyclo/cmd/gocyclo@latest"; \
		exit 1; \
	fi

## cyclo-strict: Check cyclomatic complexity (fail on violations)
.PHONY: cyclo-strict
cyclo-strict:
	@echo "Checking cyclomatic complexity (strict mode)..."
	@if command -v gocyclo >/dev/null 2>&1; then \
		gocyclo -over 15 . && echo "✓ All functions have acceptable complexity (≤15)" || (echo "❌ Some functions exceed complexity threshold of 15" && exit 1); \
	else \
		echo "gocyclo not installed. Install with: go install github.com/fzipp/gocyclo/cmd/gocyclo@latest"; \
		exit 1; \
	fi

## cyclo-gate: Intelligent complexity gate (fail only on severe violations)
.PHONY: cyclo-gate
cyclo-gate:
	@echo "Running intelligent complexity gate..."
	@if command -v gocyclo >/dev/null 2>&1; then \
		HIGH_COMPLEXITY=$$(gocyclo -over 25 . | wc -l); \
		VERY_HIGH_COMPLEXITY=$$(gocyclo -over 40 . | wc -l); \
		TOTAL_OVER_15=$$(gocyclo -over 15 . | wc -l); \
		echo "📊 Complexity Analysis:"; \
		echo "  Functions > 15: $$TOTAL_OVER_15"; \
		echo "  Functions > 25: $$HIGH_COMPLEXITY"; \
		echo "  Functions > 40: $$VERY_HIGH_COMPLEXITY"; \
		if [ $$VERY_HIGH_COMPLEXITY -gt 0 ]; then \
			echo "❌ CRITICAL: Functions with complexity > 40 found:"; \
			gocyclo -over 40 .; \
			echo "These must be refactored before release."; \
			exit 1; \
		elif [ $$HIGH_COMPLEXITY -gt 5 ]; then \
			echo "❌ WARNING: More than 5 functions with complexity > 25:"; \
			gocyclo -over 25 .; \
			echo "Consider refactoring some of these functions."; \
			exit 1; \
		elif [ $$TOTAL_OVER_15 -gt 15 ]; then \
			echo "❌ INFO: More than 15 functions with complexity > 15:"; \
			echo "Consider general code cleanup."; \
			exit 1; \
		else \
			echo "✅ Complexity gate passed"; \
			echo "  - No functions > 40 (critical)"; \
			echo "  - ≤ 5 functions > 25 (high)"; \
			echo "  - ≤ 15 functions > 15 (moderate)"; \
		fi \
	else \
		echo "gocyclo not installed. Install with: go install github.com/fzipp/gocyclo/cmd/gocyclo@latest"; \
		exit 1; \
	fi

## cyclo-report: Generate detailed complexity report
.PHONY: cyclo-report
cyclo-report:
	@echo "Generating cyclomatic complexity report..."
	@if command -v gocyclo >/dev/null 2>&1; then \
		echo "=== Cyclomatic Complexity Report ===" > complexity-report.txt; \
		echo "Generated: $$(date)" >> complexity-report.txt; \
		echo "" >> complexity-report.txt; \
		echo "Functions with complexity > 15:" >> complexity-report.txt; \
		gocyclo -over 15 . >> complexity-report.txt || echo "No functions exceed complexity of 15" >> complexity-report.txt; \
		echo "" >> complexity-report.txt; \
		echo "Overall complexity statistics:" >> complexity-report.txt; \
		gocyclo -avg . | tail -1 >> complexity-report.txt; \
		echo "Report saved to: complexity-report.txt"; \
		cat complexity-report.txt; \
	else \
		echo "gocyclo not installed. Install with: go install github.com/fzipp/gocyclo/cmd/gocyclo@latest"; \
		exit 1; \
	fi

## clean: Clean build artifacts and test files
.PHONY: clean
clean: test-clean
	@echo "Cleaning build artifacts..."
	go clean
	rm -rf ${DIST_DIR}
	rm -f ${BINARY_NAME}

## deps: Download dependencies
.PHONY: deps
deps:
	@echo "Downloading dependencies..."
	go mod download
	go mod tidy

## build-all: Build for all platforms
.PHONY: build-all
build-all: clean
	@echo "Building for all platforms..."
	@mkdir -p ${DIST_DIR}
	@$(MAKE) build-linux
	@$(MAKE) build-darwin
	@$(MAKE) build-windows
	@echo "Build complete. Binaries in ${DIST_DIR}/"

## build-linux: Build for Linux (amd64 and arm64)
.PHONY: build-linux
build-linux:
	@echo "Building for Linux..."
	@mkdir -p ${DIST_DIR}
	GOOS=linux GOARCH=amd64 go build ${GOFLAGS} ${LDFLAGS} -o ${DIST_DIR}/${BINARY_NAME}-linux-amd64 ./${CMD_DIR}
	GOOS=linux GOARCH=arm64 go build ${GOFLAGS} ${LDFLAGS} -o ${DIST_DIR}/${BINARY_NAME}-linux-arm64 ./${CMD_DIR}

## build-darwin: Build for macOS (Apple Silicon only)
.PHONY: build-darwin
build-darwin:
	@echo "Building for macOS..."
	@mkdir -p ${DIST_DIR}
	GOOS=darwin GOARCH=arm64 go build ${GOFLAGS} ${LDFLAGS} -o ${DIST_DIR}/${BINARY_NAME}-darwin-arm64 ./${CMD_DIR}

## build-windows: Build for Windows (amd64)
.PHONY: build-windows
build-windows:
	@echo "Building for Windows..."
	@mkdir -p ${DIST_DIR}
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ${GOFLAGS} ${LDFLAGS} -o ${DIST_DIR}/${BINARY_NAME}-windows-amd64.exe ./${CMD_DIR}

## install: Install binary to GOPATH/bin
.PHONY: install
install:
	@echo "Installing ${BINARY_NAME}..."
	go build ${GOFLAGS} ${LDFLAGS} -o $$(go env GOPATH)/bin/${BINARY_NAME} ./${CMD_DIR}

## uninstall: Remove binary from GOPATH/bin
.PHONY: uninstall
uninstall:
	@echo "Uninstalling ${BINARY_NAME}..."
	rm -f $$(go env GOPATH)/bin/${BINARY_NAME}


## run: Run the application with help
.PHONY: run
run: build
	@echo "Running ${BINARY_NAME}..."
	./${DIST_DIR}/${BINARY_NAME} --help

## release-linux: Build Linux binaries for release
.PHONY: release-linux
release-linux: build-linux
	@echo "Linux release binaries ready in ${DIST_DIR}/"
	@ls -la ${DIST_DIR}/ | grep linux

## release-darwin: Build macOS binaries for release
.PHONY: release-darwin
release-darwin: build-darwin
	@echo "macOS release binaries ready in ${DIST_DIR}/"
	@ls -la ${DIST_DIR}/ | grep darwin

## release-windows: Build Windows binaries for release
.PHONY: release-windows
release-windows: build-windows
	@echo "Windows release binaries ready in ${DIST_DIR}/"
	@ls -la ${DIST_DIR}/ | grep windows

## release: Build binaries for all platforms (no archives)
.PHONY: release
release: release-linux release-darwin release-windows
	@echo "All release binaries ready in ${DIST_DIR}/"
	@ls -la ${DIST_DIR}/

## check: Run all checks (fmt, vet, lint, cyclo, test)
.PHONY: check
check: fmt vet lint cyclo test

## ci: Run CI pipeline (deps, check, build)
.PHONY: ci
ci: deps check build

## test-ci: Run tests with CI-compatible output (no HTML report)
.PHONY: test-ci
test-ci:
	@echo "Running tests for CI..."
	go test -v -race -timeout 10m -coverprofile=coverage.out -covermode=atomic ./...

## act: Run GitHub Actions locally with act
.PHONY: act
act:
	@echo "Running GitHub Actions locally..."
	act -W .github/workflows/ci.yml

## act-list: List available GitHub Actions workflows
.PHONY: act-list
act-list:
	@echo "Available GitHub Actions workflows:"
	act -l

## act-test: Run only test job locally  
.PHONY: act-test
act-test:
	@echo "Running test job locally..."
	act -W .github/workflows/ci.yml -j test

.PHONY: all
all: clean deps check build