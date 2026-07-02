package crawler

// Regression tests for the project-audit fixes: response body size limit,
// startup validation of URL filter patterns, rate-limiter idempotence, and
// robots.txt crawl-delay wiring.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/masahif/linktadoru/internal/config"
)

func TestHTTPClientEnforcesResponseSizeLimit(t *testing.T) {
	big := strings.Repeat("x", 64*1024)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(big))
	}))
	defer server.Close()

	client := NewHTTPClient("LinkTadoru-Test/1.0", 5*time.Second)
	client.SetMaxResponseSize(1024)

	_, err := client.Get(context.Background(), server.URL)
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("Get on oversized body: err = %v, want ErrResponseTooLarge", err)
	}

	// A body exactly at the limit must succeed.
	client.SetMaxResponseSize(int64(len(big)))
	resp, err := client.Get(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("Get at exactly the limit failed: %v", err)
	}
	if len(resp.Body) != len(big) {
		t.Errorf("body truncated: got %d bytes, want %d", len(resp.Body), len(big))
	}
}

// An oversized page must be classified as the deterministic
// 'response_too_large' (excluded from retry), not the transient
// 'network_error' (retried).
func TestPageProcessorClassifiesOversizedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(strings.Repeat("x", 8192)))
	}))
	defer server.Close()

	client := NewHTTPClient("LinkTadoru-Test/1.0", 5*time.Second)
	client.SetMaxResponseSize(1024)
	processor := NewPageProcessor(client)

	result, err := processor.Process(context.Background(), server.URL)
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if result.Error == nil {
		t.Fatal("expected a CrawlError for the oversized response")
	}
	if result.Error.ErrorType != "response_too_large" {
		t.Errorf("ErrorType = %q, want response_too_large", result.Error.ErrorType)
	}
}

// An invalid pattern must fail crawler construction loudly. Before this fix
// regexp.MatchString errors were discarded, so a bad pattern silently never
// matched — the same "configured but ineffective" failure mode as issue #46.
func TestNewCrawlerRejectsInvalidPatterns(t *testing.T) {
	base := func() *config.CrawlConfig {
		return &config.CrawlConfig{
			Concurrency:    1,
			RequestDelay:   0.1,
			RequestTimeout: time.Second,
			UserAgent:      "LinkTadoru-Test/1.0",
		}
	}

	cfg := base()
	cfg.IncludePatterns = []string{"["}
	if _, err := NewCrawler(cfg, &MockStorage{}); err == nil || !strings.Contains(err.Error(), "include_patterns") {
		t.Errorf("invalid include pattern: err = %v, want include_patterns compile error", err)
	}

	cfg = base()
	cfg.ExcludePatterns = []string{"(unclosed"}
	if _, err := NewCrawler(cfg, &MockStorage{}); err == nil || !strings.Contains(err.Error(), "exclude_patterns") {
		t.Errorf("invalid exclude pattern: err = %v, want exclude_patterns compile error", err)
	}
}

// Re-applying the same delay must not replace the limiter: a fresh limiter has
// a full token bucket, so replacing it on every request would effectively
// disable rate limiting.
func TestSetDomainDelayIdempotent(t *testing.T) {
	rl := NewRateLimiter(10 * time.Millisecond)

	rl.SetDomainDelay("example.com", 50*time.Millisecond)
	first := rl.getLimiter("example.com")

	rl.SetDomainDelay("example.com", 50*time.Millisecond)
	if rl.getLimiter("example.com") != first {
		t.Error("same delay replaced the limiter (token bucket reset)")
	}

	rl.SetDomainDelay("example.com", 100*time.Millisecond)
	if rl.getLimiter("example.com") == first {
		t.Error("changed delay did not replace the limiter")
	}
}

// The robots.txt Crawl-delay directive must reach the rate limiter when it is
// slower than the configured default.
func TestCrawlDelayFromRobotsIsApplied(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			_, _ = w.Write([]byte("User-agent: *\nCrawl-delay: 2\n"))
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body>ok</body></html>"))
	}))
	defer server.Close()

	cfg := &config.CrawlConfig{
		SeedURLs:       []string{server.URL},
		Concurrency:    1,
		RequestDelay:   0.1,
		RequestTimeout: 5 * time.Second,
		UserAgent:      "LinkTadoru-Test/1.0",
	}
	c, err := NewCrawler(cfg, &MockStorage{})
	if err != nil {
		t.Fatalf("NewCrawler: %v", err)
	}
	c.ctx = context.Background() // normally set by Start

	host := strings.TrimPrefix(server.URL, "http://")
	if !c.shouldProcessURL(0, &URLItem{ID: 1, URL: server.URL + "/page"}) {
		t.Fatal("URL unexpectedly disallowed")
	}

	c.rateLimiter.mu.RLock()
	applied := c.rateLimiter.delays[host]
	c.rateLimiter.mu.RUnlock()
	if applied != 2*time.Second {
		t.Errorf("crawl-delay applied to rate limiter = %v, want 2s", applied)
	}
}
