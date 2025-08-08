package crawler

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func init() {
	// Disable slog output during testing
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	slog.SetDefault(logger)
}

func TestPageProcessor(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/test-page":
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Header().Set("Server", "test-server")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`
<!DOCTYPE html>
<html>
<head>
	<title>Test Page</title>
	<meta name="description" content="Test description">
	<meta name="robots" content="index,follow">
	<link rel="canonical" href="https://example.com/canonical">
</head>
<body>
	<a href="/internal-link">Internal Link</a>
	<a href="https://external.com/page">External Link</a>
</body>
</html>
			`))

		case "/404":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("Not Found"))

		case "/non-html":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status":"ok"}`))

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	httpClient := NewHTTPClient("Test-Crawler/1.0", 30*time.Second)
	defer httpClient.Close()

	processor := NewPageProcessor(httpClient)
	ctx := context.Background()

	t.Run("ProcessHTMLPage", func(t *testing.T) {
		result, err := processor.Process(ctx, server.URL+"/test-page")
		if err != nil {
			t.Fatalf("Failed to process page: %v", err)
		}

		// Check page data
		if result.Page.StatusCode != 200 {
			t.Errorf("Expected status code 200, got %d", result.Page.StatusCode)
		}

		if result.Page.Title != "Test Page" {
			t.Errorf("Expected title 'Test Page', got '%s'", result.Page.Title)
		}

		if result.Page.MetaDesc != "Test description" {
			t.Errorf("Expected description 'Test description', got '%s'", result.Page.MetaDesc)
		}

		if result.Page.ContentHash == "" {
			t.Error("Expected non-empty content hash")
		}

		// Check links
		if len(result.Links) != 2 {
			t.Fatalf("Expected 2 links, got %d", len(result.Links))
		}

		// Check internal link
		if result.Links[0].LinkType != "internal" {
			t.Errorf("Expected first link to be internal")
		}

		// Check external link
		if result.Links[1].LinkType != "external" {
			t.Errorf("Expected second link to be external")
		}
	})

	t.Run("Process404Page", func(t *testing.T) {
		result, err := processor.Process(ctx, server.URL+"/404")
		if err != nil {
			t.Fatalf("Failed to process 404 page: %v", err)
		}

		if result.Page.StatusCode != 404 {
			t.Errorf("Expected status code 404, got %d", result.Page.StatusCode)
		}

		// Should not parse HTML for error pages
		if len(result.Links) != 0 {
			t.Errorf("Expected no links for 404 page, got %d", len(result.Links))
		}
	})

	t.Run("ProcessNonHTMLPage", func(t *testing.T) {
		result, err := processor.Process(ctx, server.URL+"/non-html")
		if err != nil {
			t.Fatalf("Failed to process non-HTML page: %v", err)
		}

		if result.Page.HTTPHeaders["content-type"] != "application/json" {
			t.Errorf("Expected content type 'application/json', got '%s'", result.Page.HTTPHeaders["content-type"])
		}

		// Should not parse non-HTML content
		if len(result.Links) != 0 {
			t.Errorf("Expected no links for non-HTML page, got %d", len(result.Links))
		}
	})

	t.Run("ProcessNetworkError", func(t *testing.T) {
		// Try to process unreachable URL
		result, err := processor.Process(ctx, "http://localhost:99999/unreachable")
		if err != nil {
			t.Fatalf("Process should not return error, but capture it: %v", err)
		}

		if result.Error == nil {
			t.Error("Expected error to be captured")
		}

		if result.Error.ErrorType != "network_error" {
			t.Errorf("Expected error type 'network_error', got '%s'", result.Error.ErrorType)
		}
	})
}
