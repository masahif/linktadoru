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

## test: Run tests with coverage
.PHONY: test
test:
	@echo "Running tests..."
	go test -v -race -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html

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
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint not installed. Install with: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
		exit 1; \
	fi

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

## build-darwin: Build for macOS (Intel and Apple Silicon)
.PHONY: build-darwin
build-darwin:
	@echo "Building for macOS..."
	@mkdir -p ${DIST_DIR}
	GOOS=darwin GOARCH=amd64 go build ${GOFLAGS} ${LDFLAGS} -o ${DIST_DIR}/${BINARY_NAME}-darwin-amd64 ./${CMD_DIR}
	GOOS=darwin GOARCH=arm64 go build ${GOFLAGS} ${LDFLAGS} -o ${DIST_DIR}/${BINARY_NAME}-darwin-arm64 ./${CMD_DIR}

## build-windows: Build for Windows (amd64)
.PHONY: build-windows
build-windows:
	@echo "Building for Windows..."
	@mkdir -p ${DIST_DIR}
	GOOS=windows GOARCH=amd64 go build ${GOFLAGS} ${LDFLAGS} -o ${DIST_DIR}/${BINARY_NAME}-windows-amd64.exe ./${CMD_DIR}

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

## release: Create release archives
.PHONY: release
release: build-all
	@echo "Creating release archives..."
	@mkdir -p ${DIST_DIR}/release
	@cd ${DIST_DIR} && \
		tar czf release/${BINARY_NAME}-${VERSION}-linux-amd64.tar.gz ${BINARY_NAME}-linux-amd64 && \
		tar czf release/${BINARY_NAME}-${VERSION}-linux-arm64.tar.gz ${BINARY_NAME}-linux-arm64 && \
		tar czf release/${BINARY_NAME}-${VERSION}-darwin-amd64.tar.gz ${BINARY_NAME}-darwin-amd64 && \
		tar czf release/${BINARY_NAME}-${VERSION}-darwin-arm64.tar.gz ${BINARY_NAME}-darwin-arm64 && \
		zip release/${BINARY_NAME}-${VERSION}-windows-amd64.zip ${BINARY_NAME}-windows-amd64.exe
	@echo "Release archives created in ${DIST_DIR}/release/"

## check: Run all checks (fmt, vet, lint, test)
.PHONY: check
check: fmt vet lint test

.PHONY: all
all: clean deps check build