package crawler

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"time"
)

// maxRedirects bounds how many redirect hops a single request may follow.
const maxRedirects = 10

// DefaultMaxResponseSize bounds how many bytes of a response body are read
// when no explicit limit is configured. Without a bound, a single huge (or
// malicious) response would be read entirely into memory.
const DefaultMaxResponseSize = 10 * 1024 * 1024 // 10 MiB

// ErrResponseTooLarge is returned when a response body exceeds the configured
// size limit. Callers can detect it with errors.Is to classify the failure as
// deterministic (not worth retrying).
var ErrResponseTooLarge = errors.New("response body exceeds size limit")

// ErrRedirectNotAllowed is returned when a redirect points at a target the
// configured redirect policy rejects. Without this check a 302 could steer the
// crawler at a host it was never allowed to reach (SSRF).
var ErrRedirectNotAllowed = errors.New("redirect target not allowed")

// HTTPClient handles HTTP requests with performance metrics
type HTTPClient struct {
	client          *http.Client
	userAgent       string
	maxResponseSize int64 // Max bytes to read from a response body
	authType        string
	username        string            // Basic auth username
	password        string            // Basic auth password
	bearerToken     string            // Bearer token
	apiKeyHeader    string            // API key header name
	apiKeyValue     string            // API key header value
	customHeaders   map[string]string // Custom headers

	// redirectPolicy decides whether a redirect target may be followed.
	// Nil allows every target, bounded only by the hop limit.
	redirectPolicy func(*url.URL) bool
}

// HTTPMetrics contains performance metrics for an HTTP request
type HTTPMetrics struct {
	TTFB         time.Duration // Time to First Byte
	DownloadTime time.Duration // Total download time
	DNSLookup    time.Duration // DNS lookup time
	TCPConnect   time.Duration // TCP connection time
	TLSHandshake time.Duration // TLS handshake time
}

// HTTPResponse contains the response and metrics
type HTTPResponse struct {
	StatusCode      int
	Headers         http.Header
	Body            []byte
	ContentType     string
	ContentLength   int64
	Server          string
	LastModified    time.Time
	ContentEncoding string
	Metrics         HTTPMetrics
	FinalURL        string // After following redirects
}

// NewHTTPClient creates a new HTTP client
func NewHTTPClient(userAgent string, timeout time.Duration) *HTTPClient {
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
		DisableCompression:  false, // Enable automatic decompression
	}

	h := &HTTPClient{
		userAgent:       userAgent,
		maxResponseSize: DefaultMaxResponseSize,
		customHeaders:   make(map[string]string),
	}

	h.client = &http.Client{
		Transport:     transport,
		Timeout:       timeout,
		CheckRedirect: h.checkRedirect,
	}

	return h
}

// SetRedirectPolicy installs a predicate consulted before each redirect hop.
// Returning false aborts the request with ErrRedirectNotAllowed without
// contacting the target. Passing nil clears the policy.
func (h *HTTPClient) SetRedirectPolicy(allow func(*url.URL) bool) {
	h.redirectPolicy = allow
}

// checkRedirect gates redirect hops. It runs before the next request is sent,
// so a rejected target is never contacted. On top of the hop limit it enforces
// the redirect policy and drops credentials once the origin changes, so a
// redirect cannot leak auth headers to a host they were never meant for.
func (h *HTTPClient) checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= maxRedirects {
		return fmt.Errorf("too many redirects")
	}

	if h.redirectPolicy != nil && !h.redirectPolicy(req.URL) {
		return fmt.Errorf("%w: %s", ErrRedirectNotAllowed, req.URL.String())
	}

	// Compare against the originating request: credentials belong to the
	// origin they were configured for and must not follow the crawler off it.
	// Go's built-in redirect policy does not cover the configured API-key
	// header or arbitrary custom headers, so every crawler credential is
	// removed here at an explicit origin boundary.
	if len(via) > 0 && !sameOrigin(via[0].URL, req.URL) {
		h.stripCredentialHeaders(req.Header)
	}

	return nil
}

// sameOrigin reports whether two URLs share a scheme and host (including port).
func sameOrigin(a, b *url.URL) bool {
	return a.Scheme == b.Scheme && a.Host == b.Host
}

// stripCredentialHeaders removes every header that carries a secret.
func (h *HTTPClient) stripCredentialHeaders(header http.Header) {
	header.Del("Authorization")
	header.Del("Proxy-Authorization")
	header.Del("Cookie")

	if h.apiKeyHeader != "" {
		header.Del(h.apiKeyHeader)
	}
	for name := range h.customHeaders {
		header.Del(name)
	}
}

// SetMaxResponseSize overrides the response body size limit (bytes).
// Values <= 0 keep the current limit.
func (h *HTTPClient) SetMaxResponseSize(n int64) {
	if n > 0 {
		h.maxResponseSize = n
	}
}

// SetBasicAuth configures basic authentication for HTTP requests
func (h *HTTPClient) SetBasicAuth(username, password string) {
	h.authType = "basic"
	h.username = username
	h.password = password
}

// SetBearerAuth configures bearer token authentication for HTTP requests
func (h *HTTPClient) SetBearerAuth(token string) {
	h.authType = "bearer"
	h.bearerToken = token
}

// SetAPIKeyAuth configures API key authentication for HTTP requests
func (h *HTTPClient) SetAPIKeyAuth(header, value string) {
	h.authType = "apikey"
	h.apiKeyHeader = header
	h.apiKeyValue = value
}

// SetCustomHeaders sets custom HTTP headers
func (h *HTTPClient) SetCustomHeaders(headers map[string]string) {
	if h.customHeaders == nil {
		h.customHeaders = make(map[string]string)
	}
	for k, v := range headers {
		h.customHeaders[k] = v
	}
}

// AddCustomHeader adds a single custom HTTP header
func (h *HTTPClient) AddCustomHeader(name, value string) {
	if h.customHeaders == nil {
		h.customHeaders = make(map[string]string)
	}
	h.customHeaders[name] = value
}

// Get performs an HTTP GET request with comprehensive performance tracking.
// It measures DNS lookup time, TCP connection time, TLS handshake time,
// time to first byte (TTFB), and total download time. The response includes
// both the content and detailed performance metrics.
func (h *HTTPClient) Get(ctx context.Context, url string) (*HTTPResponse, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set User-Agent
	req.Header.Set("User-Agent", h.userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	// Don't set Accept-Encoding manually - let Go handle compression automatically

	// Set basic authentication if configured
	switch h.authType {
	case "basic":
		if h.username != "" && h.password != "" {
			req.SetBasicAuth(h.username, h.password)
		}
	case "bearer":
		if h.bearerToken != "" {
			req.Header.Set("Authorization", "Bearer "+h.bearerToken)
		}
	case "apikey":
		if h.apiKeyHeader != "" && h.apiKeyValue != "" {
			req.Header.Set(h.apiKeyHeader, h.apiKeyValue)
		}
	}

	// Set custom headers
	for name, value := range h.customHeaders {
		req.Header.Set(name, value)
	}

	// Setup performance tracking
	var metrics HTTPMetrics
	var dnsStart, dnsDone, connectStart, connectDone, tlsStart, tlsDone time.Time
	var firstByteTime time.Time

	trace := &httptrace.ClientTrace{
		DNSStart: func(info httptrace.DNSStartInfo) {
			dnsStart = time.Now()
		},
		DNSDone: func(info httptrace.DNSDoneInfo) {
			dnsDone = time.Now()
			metrics.DNSLookup = dnsDone.Sub(dnsStart)
		},
		ConnectStart: func(network, addr string) {
			connectStart = time.Now()
		},
		ConnectDone: func(network, addr string, err error) {
			connectDone = time.Now()
			metrics.TCPConnect = connectDone.Sub(connectStart)
		},
		TLSHandshakeStart: func() {
			tlsStart = time.Now()
		},
		TLSHandshakeDone: func(state tls.ConnectionState, err error) {
			tlsDone = time.Now()
			metrics.TLSHandshake = tlsDone.Sub(tlsStart)
		},
		GotFirstResponseByte: func() {
			firstByteTime = time.Now()
		},
	}

	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))

	// Perform request
	startTime := time.Now()
	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			// Log error but don't fail the request
			_ = err
		}
	}()

	// Calculate TTFB if we got the first byte time
	if !firstByteTime.IsZero() {
		metrics.TTFB = firstByteTime.Sub(startTime)
	}

	// Read response body, bounded so a huge response cannot exhaust memory.
	// Read one extra byte to distinguish "exactly at the limit" from "over it".
	body, err := io.ReadAll(io.LimitReader(resp.Body, h.maxResponseSize+1))
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}
	if int64(len(body)) > h.maxResponseSize {
		return nil, fmt.Errorf("%w (%d bytes limit): %s", ErrResponseTooLarge, h.maxResponseSize, url)
	}

	// Calculate total download time
	metrics.DownloadTime = time.Since(startTime)

	// Parse Last-Modified header
	var lastModified time.Time
	if lm := resp.Header.Get("Last-Modified"); lm != "" {
		if t, err := http.ParseTime(lm); err == nil {
			lastModified = t
		}
	}

	return &HTTPResponse{
		StatusCode:      resp.StatusCode,
		Headers:         resp.Header,
		Body:            body,
		ContentType:     resp.Header.Get("Content-Type"),
		ContentLength:   resp.ContentLength,
		Server:          resp.Header.Get("Server"),
		LastModified:    lastModified,
		ContentEncoding: resp.Header.Get("Content-Encoding"),
		Metrics:         metrics,
		FinalURL:        resp.Request.URL.String(),
	}, nil
}

// Close closes the HTTP client
func (h *HTTPClient) Close() {
	h.client.CloseIdleConnections()
}
