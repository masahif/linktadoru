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

	tests := []struct {
		name           string
		path           string
		expectedStatus int
		expectedTitle  string
		expectedDesc   string
		expectedLinks  int
		expectError    bool
		errorType      string
		contentType    string
	}{
		{
			name:           "ProcessHTMLPage",
			path:           "/test-page",
			expectedStatus: 200,
			expectedTitle:  "Test Page",
			expectedDesc:   "Test description",
			expectedLinks:  2,
			expectError:    false,
		},
		{
			name:           "Process404Page",
			path:           "/404",
			expectedStatus: 404,
			expectedLinks:  0,
			expectError:    false,
		},
		{
			name:           "ProcessNonHTMLPage",
			path:           "/non-html",
			expectedStatus: 200,
			expectedLinks:  0,
			expectError:    false,
			contentType:    "application/json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := processor.Process(ctx, server.URL+tt.path)
			if err != nil {
				t.Fatalf("Failed to process page: %v", err)
			}

			// Check status code
			if result.Page.StatusCode != tt.expectedStatus {
				t.Errorf("Expected status code %d, got %d", tt.expectedStatus, result.Page.StatusCode)
			}

			// Check title for HTML pages
			if tt.expectedTitle != "" && result.Page.Title != tt.expectedTitle {
				t.Errorf("Expected title '%s', got '%s'", tt.expectedTitle, result.Page.Title)
			}

			// Check description
			if tt.expectedDesc != "" && result.Page.MetaDesc != tt.expectedDesc {
				t.Errorf("Expected description '%s', got '%s'", tt.expectedDesc, result.Page.MetaDesc)
			}

			// Check links count
			if len(result.Links) != tt.expectedLinks {
				t.Errorf("Expected %d links, got %d", tt.expectedLinks, len(result.Links))
			}

			// Check content type if specified
			if tt.contentType != "" && result.Page.HTTPHeaders["content-type"] != tt.contentType {
				t.Errorf("Expected content type '%s', got '%s'", tt.contentType, result.Page.HTTPHeaders["content-type"])
			}

			// Additional validation for HTML page
			if tt.name == "ProcessHTMLPage" {
				validateHTMLPageLinks(t, result)
			}
		})
	}

	// Test network error separately due to different URL
	t.Run("ProcessNetworkError", func(t *testing.T) {
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

func validateHTMLPageLinks(t *testing.T, result *PageResult) {
	if result.Page.ContentHash == "" {
		t.Error("Expected non-empty content hash")
	}

	if len(result.Links) >= 1 && result.Links[0].LinkType != "internal" {
		t.Errorf("Expected first link to be internal")
	}

	if len(result.Links) >= 2 && result.Links[1].LinkType != "external" {
		t.Errorf("Expected second link to be external")
	}
}
