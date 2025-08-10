package crawler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestPageProcessorExternalLinks tests that page processor correctly handles external links
func TestPageProcessorExternalLinks(t *testing.T) {
	// Create a test HTML page with both internal and external links
	testHTML := `<!DOCTYPE html>
<html>
<head><title>Test Page</title></head>
<body>
	<h1>Test Links</h1>
	<!-- Internal links -->
	<a href="/internal-page">Internal relative</a>
	<a href="http://example.com/page1">Internal absolute</a>
	
	<!-- External links (should not be saved when follow_external_hosts=false) -->
	<a href="https://google.com/search">External: Google</a>
	<a href="https://github.com/user/repo">External: GitHub</a>
	<a href="http://httpbin.org/get">External: HTTPBin</a>
	
	<!-- Invalid schemes (should never be saved) -->
	<a href="tel:+1234567890">Phone</a>
	<a href="mailto:test@example.com">Email</a>
</body>
</html>`

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(testHTML))
	}))
	defer server.Close()

	t.Run("SaveExternalLinks=true", func(t *testing.T) {
		// Create HTTP client and page processor with external links saving enabled
		httpClient := NewHTTPClient("TestCrawler/1.0", 10*time.Second)
		processor := NewPageProcessorWithConfig(httpClient, []string{"https://", "http://"}, true) // Save external links

		// Process the test page
		ctx := context.Background()
		result, err := processor.Process(ctx, server.URL)
		if err != nil {
			t.Fatalf("Failed to process page: %v", err)
		}

		if result.Page == nil {
			t.Fatal("Expected page result, got nil")
		}

		t.Logf("Found %d links in page (save external=true):", len(result.Links))
		for i, link := range result.Links {
			t.Logf("Link %d: %s (type: %s)", i+1, link.TargetURL, link.LinkType)
		}

		// Analyze results
		var internalLinks, externalLinks []*LinkData
		for _, link := range result.Links {
			if link.LinkType == "internal" {
				internalLinks = append(internalLinks, link)
			} else if link.LinkType == "external" {
				externalLinks = append(externalLinks, link)
			}
		}

		t.Logf("Internal links: %d, External links: %d", len(internalLinks), len(externalLinks))

		// Should have both internal and external links
		expectedInternal := 1 // Only "/internal-page"
		expectedExternal := 4 // "example.com/page1", google.com, github.com, httpbin.org

		if len(internalLinks) != expectedInternal {
			t.Errorf("Expected %d internal links, got %d", expectedInternal, len(internalLinks))
		}

		if len(externalLinks) != expectedExternal {
			t.Errorf("Expected %d external links, got %d", expectedExternal, len(externalLinks))
		}
	})

	t.Run("SaveExternalLinks=false", func(t *testing.T) {
		// Create HTTP client and page processor with external links saving disabled
		httpClient := NewHTTPClient("TestCrawler/1.0", 10*time.Second)
		processor := NewPageProcessorWithConfig(httpClient, []string{"https://", "http://"}, false) // Don't save external links

		// Process the test page
		ctx := context.Background()
		result, err := processor.Process(ctx, server.URL)
		if err != nil {
			t.Fatalf("Failed to process page: %v", err)
		}

		if result.Page == nil {
			t.Fatal("Expected page result, got nil")
		}

		t.Logf("Found %d links in page (save external=false):", len(result.Links))
		for i, link := range result.Links {
			t.Logf("Link %d: %s (type: %s)", i+1, link.TargetURL, link.LinkType)
		}

		// Analyze results
		var internalLinks, externalLinks []*LinkData
		for _, link := range result.Links {
			if link.LinkType == "internal" {
				internalLinks = append(internalLinks, link)
			} else if link.LinkType == "external" {
				externalLinks = append(externalLinks, link)
			}
		}

		t.Logf("Internal links: %d, External links: %d", len(internalLinks), len(externalLinks))

		// Should ONLY have internal links - external links should be filtered out
		expectedInternal := 1 // Only "/internal-page"
		expectedExternal := 0 // NO external links should be saved

		if len(internalLinks) != expectedInternal {
			t.Errorf("Expected %d internal links, got %d", expectedInternal, len(internalLinks))
		}

		if len(externalLinks) != expectedExternal {
			t.Errorf("Expected %d external links when saveExternalLinks=false, got %d:", expectedExternal, len(externalLinks))
			for _, link := range externalLinks {
				t.Errorf("  Unexpected external link: %s", link.TargetURL)
			}
		}
	})

	// Check that invalid schemes were filtered out (should not appear in any results)
	httpClient := NewHTTPClient("TestCrawler/1.0", 10*time.Second)
	processor := NewPageProcessorWithConfig(httpClient, []string{"https://", "http://"}, true)
	ctx := context.Background()
	result, _ := processor.Process(ctx, server.URL)

	for _, link := range result.Links {
		if hasInvalidScheme(link.TargetURL) {
			t.Errorf("Invalid scheme URL was not filtered: %s", link.TargetURL)
		}
	}
}


// Helper function to check for invalid schemes
func hasInvalidScheme(url string) bool {
	invalidSchemes := []string{"tel:", "mailto:", "javascript:", "chrome-extension:"}
	for _, scheme := range invalidSchemes {
		if len(url) >= len(scheme) && url[:len(scheme)] == scheme {
			return true
		}
	}
	return false
}