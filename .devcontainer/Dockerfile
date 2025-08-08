FROM golang:1.23-bullseye

# Install system dependencies
RUN apt-get update && apt-get install -y \
    build-essential \
    curl \
    wget \
    git \
    sqlite3 \
    && rm -rf /var/lib/apt/lists/*

# Install development tools
RUN curl https://raw.githubusercontent.com/nektos/act/master/install.sh | bash
RUN curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin latest
RUN curl -sfL https://raw.githubusercontent.com/aquasecurity/trivy/main/contrib/install.sh | sh -s -- -b /usr/local/bin

# Install code quality tools
RUN go install github.com/fzipp/gocyclo/cmd/gocyclo@latest

WORKDIR /workspace