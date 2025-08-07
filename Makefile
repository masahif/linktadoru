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
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

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
	go install ${GOFLAGS} ${LDFLAGS} ./${CMD_DIR}

## uninstall: Remove binary from GOPATH/bin
.PHONY: uninstall
uninstall:
	@echo "Uninstalling ${BINARY_NAME}..."
	rm -f $$(go env GOPATH)/bin/${BINARY_NAME}


## run: Run the application with help
.PHONY: run
run: build
	@echo "Running ${BINARY_NAME}..."
	./${BINARY_NAME} --help

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

## check: Run all checks (fmt, vet, lint, test)
.PHONY: check
check: fmt vet lint test

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