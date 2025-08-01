package crawler

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"net/http/httptrace"
	"time"
)

// HTTPClient handles HTTP requests with performance metrics
type HTTPClient struct {
	client    *http.Client
	userAgent string
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

	client := &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	return &HTTPClient{
		client:    client,
		userAgent: userAgent,
	}
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
	defer resp.Body.Close()

	// Calculate TTFB if we got the first byte time
	if !firstByteTime.IsZero() {
		metrics.TTFB = firstByteTime.Sub(startTime)
	}

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
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