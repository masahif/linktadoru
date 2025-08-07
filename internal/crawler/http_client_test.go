package crawler

import (
	"context"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
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

	// Content length should match the actual body size (25 characters)
	expectedBody := `{"message": "success"}`
	if len(resp.Body) != len(expectedBody) {
		t.Errorf("Expected body length %d, got %d", len(expectedBody), len(resp.Body))
	}
}

func TestHTTPClientBasicAuth(t *testing.T) {
	// Create test server that requires basic auth
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			w.Header().Set("WWW-Authenticate", `Basic realm="Test"`)
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("Unauthorized"))
			return
		}

		// Verify basic auth format
		if !strings.HasPrefix(auth, "Basic ") {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("Invalid auth format"))
			return
		}

		// Decode and verify credentials
		encoded := strings.TrimPrefix(auth, "Basic ")
		decoded, err := base64.StdEncoding.DecodeString(encoded)
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("Invalid base64"))
			return
		}

		credentials := string(decoded)
		if credentials != "testuser:testpass" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("Invalid credentials"))
			return
		}

		// Success
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Authenticated!"))
	}))
	defer server.Close()

	// Test without auth - should get 401
	client := NewHTTPClient("Test-Crawler/1.0", 30*time.Second)
	defer client.Close()

	ctx := context.Background()
	resp, err := client.Get(ctx, server.URL)
	if err != nil {
		t.Fatalf("Failed to get URL: %v", err)
	}

	if resp.StatusCode != 401 {
		t.Errorf("Expected status code 401 without auth, got %d", resp.StatusCode)
	}

	// Test with auth - should get 200
	client.SetBasicAuth("testuser", "testpass")
	resp, err = client.Get(ctx, server.URL)
	if err != nil {
		t.Fatalf("Failed to get URL with auth: %v", err)
	}

	if resp.StatusCode != 200 {
		t.Errorf("Expected status code 200 with auth, got %d", resp.StatusCode)
	}

	if string(resp.Body) != "Authenticated!" {
		t.Errorf("Expected body 'Authenticated!', got '%s'", string(resp.Body))
	}
}

func TestHTTPClientSetBasicAuth(t *testing.T) {
	client := NewHTTPClient("Test-Crawler/1.0", 30*time.Second)
	defer client.Close()

	// Initially no auth
	if client.username != "" || client.password != "" {
		t.Errorf("Expected empty credentials initially, got username='%s', password='%s'", client.username, client.password)
	}

	// Set auth
	client.SetBasicAuth("user123", "pass456")
	if client.username != "user123" || client.password != "pass456" {
		t.Errorf("Expected user123/pass456, got username='%s', password='%s'", client.username, client.password)
	}

	// Clear auth by setting empty strings
	client.SetBasicAuth("", "")
	if client.username != "" || client.password != "" {
		t.Errorf("Expected empty credentials after clearing, got username='%s', password='%s'", client.username, client.password)
	}
}

func TestHTTPClientBearerAuth(t *testing.T) {
	// Create test server that requires bearer auth
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("Unauthorized"))
			return
		}

		// Verify bearer auth format
		if !strings.HasPrefix(auth, "Bearer ") {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("Invalid auth format"))
			return
		}

		// Verify token
		token := strings.TrimPrefix(auth, "Bearer ")
		if token != "test-bearer-token-123" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("Invalid token"))
			return
		}

		// Success
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Bearer Authenticated!"))
	}))
	defer server.Close()

	// Test without auth - should get 401
	client := NewHTTPClient("Test-Crawler/1.0", 30*time.Second)
	defer client.Close()

	ctx := context.Background()
	resp, err := client.Get(ctx, server.URL)
	if err != nil {
		t.Fatalf("Failed to get URL: %v", err)
	}

	if resp.StatusCode != 401 {
		t.Errorf("Expected status code 401 without auth, got %d", resp.StatusCode)
	}

	// Test with bearer auth - should get 200
	client.SetBearerAuth("test-bearer-token-123")
	resp, err = client.Get(ctx, server.URL)
	if err != nil {
		t.Fatalf("Failed to get URL with bearer auth: %v", err)
	}

	if resp.StatusCode != 200 {
		t.Errorf("Expected status code 200 with bearer auth, got %d", resp.StatusCode)
	}

	if string(resp.Body) != "Bearer Authenticated!" {
		t.Errorf("Expected body 'Bearer Authenticated!', got '%s'", string(resp.Body))
	}
}

func TestHTTPClientAPIKeyAuth(t *testing.T) {
	// Create test server that requires API key auth
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiKey := r.Header.Get("X-API-Key")
		if apiKey == "" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("API key required"))
			return
		}

		// Verify API key
		if apiKey != "test-api-key-456" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("Invalid API key"))
			return
		}

		// Success
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("API Key Authenticated!"))
	}))
	defer server.Close()

	// Test without auth - should get 401
	client := NewHTTPClient("Test-Crawler/1.0", 30*time.Second)
	defer client.Close()

	ctx := context.Background()
	resp, err := client.Get(ctx, server.URL)
	if err != nil {
		t.Fatalf("Failed to get URL: %v", err)
	}

	if resp.StatusCode != 401 {
		t.Errorf("Expected status code 401 without auth, got %d", resp.StatusCode)
	}

	// Test with API key auth - should get 200
	client.SetAPIKeyAuth("X-API-Key", "test-api-key-456")
	resp, err = client.Get(ctx, server.URL)
	if err != nil {
		t.Fatalf("Failed to get URL with API key auth: %v", err)
	}

	if resp.StatusCode != 200 {
		t.Errorf("Expected status code 200 with API key auth, got %d", resp.StatusCode)
	}

	if string(resp.Body) != "API Key Authenticated!" {
		t.Errorf("Expected body 'API Key Authenticated!', got '%s'", string(resp.Body))
	}
}

func TestHTTPClientCustomHeaders(t *testing.T) {
	// Create test server that checks custom headers
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check for custom headers
		customHeader := r.Header.Get("X-Custom-Header")
		acceptHeader := r.Header.Get("Accept")

		// Verify headers are present
		if customHeader != "custom-value" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("Missing or invalid X-Custom-Header"))
			return
		}

		if acceptHeader != "application/json" {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte("Missing or invalid Accept header"))
			return
		}

		// Success
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Headers validated!"))
	}))
	defer server.Close()

	// Test with custom headers
	client := NewHTTPClient("Test-Crawler/1.0", 30*time.Second)
	defer client.Close()

	// Set custom headers
	client.SetCustomHeaders(map[string]string{
		"X-Custom-Header": "custom-value",
		"Accept":          "application/json",
	})

	ctx := context.Background()
	resp, err := client.Get(ctx, server.URL)
	if err != nil {
		t.Fatalf("Failed to get URL with custom headers: %v", err)
	}

	if resp.StatusCode != 200 {
		t.Errorf("Expected status code 200 with custom headers, got %d", resp.StatusCode)
	}

	if string(resp.Body) != "Headers validated!" {
		t.Errorf("Expected body 'Headers validated!', got '%s'", string(resp.Body))
	}
}

func TestHTTPClientAddCustomHeader(t *testing.T) {
	client := NewHTTPClient("Test-Crawler/1.0", 30*time.Second)
	defer client.Close()

	// Initially no custom headers
	if len(client.customHeaders) != 0 {
		t.Errorf("Expected empty custom headers initially, got %d", len(client.customHeaders))
	}

	// Add custom headers
	client.AddCustomHeader("X-Test-Header", "test-value")
	client.AddCustomHeader("X-Another-Header", "another-value")

	if len(client.customHeaders) != 2 {
		t.Errorf("Expected 2 custom headers, got %d", len(client.customHeaders))
	}

	if client.customHeaders["X-Test-Header"] != "test-value" {
		t.Errorf("Expected 'test-value' for X-Test-Header, got '%s'", client.customHeaders["X-Test-Header"])
	}

	if client.customHeaders["X-Another-Header"] != "another-value" {
		t.Errorf("Expected 'another-value' for X-Another-Header, got '%s'", client.customHeaders["X-Another-Header"])
	}
}
