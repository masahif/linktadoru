# Build image for Anthropic OSS Scanner (https://github.com/anthropics/oss-scanner).
# The scanner builds this with the repository as the build context, then removes
# network access, so everything the tests need is fetched here.
FROM golang:1.26-bookworm

# -race in the test suite needs cgo.
ENV DEBIAN_FRONTEND=noninteractive \
    CGO_ENABLED=1 \
    GOTOOLCHAIN=local

RUN apt-get update && apt-get install -y --no-install-recommends \
      build-essential ca-certificates git sqlite3 \
    && rm -rf /var/lib/apt/lists/*

# scanner contract: the checkout lives inside the image, at /src
COPY . /src
WORKDIR /src

RUN go mod download \
    && go build -o /usr/local/bin/linktadoru ./cmd/crawler

# The tests use httptest servers only, so they run offline. Keep the image even
# if a baseline test fails.
RUN go test -race -timeout 10m ./... \
    || echo 'WARNING: baseline tests failed'

# Reproducers and patches must also build with the network disabled.
ENV GOPROXY=off \
    GOSUMDB=off

CMD ["bash"]
