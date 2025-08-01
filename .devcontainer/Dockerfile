FROM mcr.microsoft.com/devcontainers/go:1-1.23-bullseye

# Install additional system dependencies
RUN apt-get update && export DEBIAN_FRONTEND=noninteractive \
    && apt-get -y install --no-install-recommends \
        sqlite3 \
        libsqlite3-dev \
        build-essential \
        curl \
        git \
        vim \
    && apt-get clean \
    && rm -rf /var/lib/apt/lists/*

# Install Go tools
RUN go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest \
    && go install golang.org/x/tools/gopls@latest \
    && go install github.com/go-delve/delve/cmd/dlv@latest

# Set up workspace
WORKDIR /workspace

# Create data directory for development
RUN mkdir -p /workspace/data && chown -R vscode:vscode /workspace/data