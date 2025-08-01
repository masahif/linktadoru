package crawler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPClient(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check User-Agent
		if ua := r.Header.Get("User-Agent"); ua != "Test-Crawler/1.0" {
			t.Errorf("Expected User-Agent 'Test-Crawler/1.0', got '%s'", ua)
		}

		// Set response headers
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Server", "test-server")
		w.Header().Set("Last-Modified", "Wed, 21 Oct 2015 07:28:00 GMT")

		// Add delay to test TTFB
		time.Sleep(50 * time.Millisecond)

		// Write response
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html><body>Test Page</body></html>"))
	}))
	defer server.Close()

	client := NewHTTPClient("Test-Crawler/1.0", 30*time.Second)
	defer client.Close()

	ctx := context.Background()
	resp, err := client.Get(ctx, server.URL)
	if err != nil {
		t.Fatalf("Failed to get URL: %v", err)
	}

	// Check response
	if resp.StatusCode != 200 {
		t.Errorf("Expected status code 200, got %d", resp.StatusCode)
	}

	if resp.ContentType != "text/html; charset=utf-8" {
		t.Errorf("Expected content type 'text/html; charset=utf-8', got '%s'", resp.ContentType)
	}

	if resp.Server != "test-server" {
		t.Errorf("Expected server 'test-server', got '%s'", resp.Server)
	}

	// Check metrics
	if resp.Metrics.TTFB < 50*time.Millisecond {
		t.Errorf("TTFB should be at least 50ms, got %v", resp.Metrics.TTFB)
	}

	if resp.Metrics.DownloadTime < resp.Metrics.TTFB {
		t.Errorf("Download time should be greater than TTFB")
	}

	// Check body
	expectedBody := "<html><body>Test Page</body></html>"
	if string(resp.Body) != expectedBody {
		t.Errorf("Expected body '%s', got '%s'", expectedBody, string(resp.Body))
	}
}

func TestHTTPClientRedirect(t *testing.T) {
	// Create test server with redirect
	redirectCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if redirectCount < 2 {
			redirectCount++
			http.Redirect(w, r, "/final", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Final page"))
	}))
	defer server.Close()

	client := NewHTTPClient("Test-Crawler/1.0", 30*time.Second)
	defer client.Close()

	ctx := context.Background()
	resp, err := client.Get(ctx, server.URL)
	if err != nil {
		t.Fatalf("Failed to get URL: %v", err)
	}

	if resp.StatusCode != 200 {
		t.Errorf("Expected status code 200, got %d", resp.StatusCode)
	}

	if resp.FinalURL != server.URL+"/final" {
		t.Errorf("Expected final URL '%s', got '%s'", server.URL+"/final", resp.FinalURL)
	}
}

func TestHTTPClientTimeout(t *testing.T) {
	// Create slow server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewHTTPClient("Test-Crawler/1.0", 1*time.Second)
	defer client.Close()

	ctx := context.Background()
	_, err := client.Get(ctx, server.URL)
	if err == nil {
		t.Errorf("Expected timeout error, got nil")
	}
}

func TestHTTPClientErrorCases(t *testing.T) {
	client := NewHTTPClient("Test-Crawler/1.0", 30*time.Second)
	defer client.Close()

	ctx := context.Background()

	// Test invalid URL
	_, err := client.Get(ctx, "invalid-url")
	if err == nil {
		t.Errorf("Expected error for invalid URL, got nil")
	}

	// Test non-existent domain
	_, err = client.Get(ctx, "http://non-existent-domain-12345.com")
	if err == nil {
		t.Errorf("Expected error for non-existent domain, got nil")
	}

	// Test server error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("Internal Server Error"))
	}))
	defer server.Close()

	resp, err := client.Get(ctx, server.URL)
	if err != nil {
		t.Errorf("Unexpected error for server error response: %v", err)
	}
	if resp.StatusCode != 500 {
		t.Errorf("Expected status code 500, got %d", resp.StatusCode)
	}

	// Test cancelled context
	cancelledCtx, cancel := context.WithCancel(ctx)
	cancel() // Cancel immediately

	_, err = client.Get(cancelledCtx, server.URL)
	if err == nil && cancelledCtx.Err() != nil {
		t.Errorf("Expected error for cancelled context, got nil")
	}
}

func TestHTTPClientHeaders(t *testing.T) {
	// Test that additional headers are handled properly
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set various response headers
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Content-Length", "25")
		w.Header().Set("Cache-Control", "no-cache")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message": "success"}`))
	}))
	defer server.Close()

	client := NewHTTPClient("Test-Crawler/1.0", 30*time.Second)
	defer client.Close()

	ctx := context.Background()
	resp, err := client.Get(ctx, server.URL)
	if err != nil {
		t.Fatalf("Failed to get URL: %v", err)
	}

	if resp.ContentType != "application/json" {
		t.Errorf("Expected content type 'application/json', got '%s'", resp.ContentType)
	}

	if resp.ContentEncoding != "gzip" {
		t.Errorf("Expected content encoding 'gzip', got '%s'", resp.ContentEncoding)
	}

	if resp.ContentLength != 25 {
		t.Errorf("Expected content length 25, got %d", resp.ContentLength)
	}
}