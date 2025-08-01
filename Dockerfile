# Build stage
FROM golang:1.23-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git gcc musl-dev sqlite-dev

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o linktadoru ./cmd/crawler

# Final stage
FROM alpine:latest

# Install runtime dependencies
RUN apk --no-cache add ca-certificates sqlite

# Create non-root user
RUN addgroup -g 1001 -S linktadoru && \
    adduser -u 1001 -D -S -G linktadoru linktadoru

# Set working directory
WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /app/linktadoru .

# Copy example config
COPY --from=builder /app/config.yaml.example ./config.yaml.example

# Create data directory
RUN mkdir -p /app/data && chown -R linktadoru:linktadoru /app

# Switch to non-root user
USER linktadoru

# Expose volume for data persistence
VOLUME ["/app/data"]

# Set default database path to persistent volume
ENV LT_DATABASE_PATH=/app/data/crawl.db

# Default command
ENTRYPOINT ["./linktadoru"]
CMD ["--help"]