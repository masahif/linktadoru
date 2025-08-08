package crawler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestRobotsParser(t *testing.T) {
	robotsTxt := `
User-agent: *
Disallow: /admin/
Disallow: /private/
Allow: /private/public/
Crawl-delay: 2

User-agent: Googlebot
Disallow: /no-google/

Sitemap: https://example.com/sitemap.xml
`

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/robots.txt" {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(robotsTxt))
		} else {
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	httpClient := NewHTTPClient("Test-Crawler/1.0", 30*time.Second)
	defer httpClient.Close()

	parser := NewRobotsParser(httpClient, false)
	ctx := context.Background()

	tests := []struct {
		name     string
		url      string
		expected bool
	}{
		{"Root allowed", server.URL + "/", true},
		{"Admin disallowed", server.URL + "/admin/page", false},
		{"Private disallowed", server.URL + "/private/data", false},
		{"Private public allowed", server.URL + "/private/public/page", true},
		{"Other path allowed", server.URL + "/blog/post", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			allowed, err := parser.IsAllowed(ctx, tt.url, "Test-Crawler")
			if err != nil {
				t.Errorf("Error checking robots.txt: %v", err)
			}
			if allowed != tt.expected {
				t.Errorf("Expected %v for %s, got %v", tt.expected, tt.url, allowed)
			}
		})
	}

	// Test crawl delay
	parsedURL, _ := url.Parse(server.URL)
	delay := parser.GetCrawlDelay(parsedURL.Host)
	if delay != 2*time.Second {
		t.Errorf("Expected crawl delay of 2s, got %v", delay)
	}
}

func TestRobotsParserIgnore(t *testing.T) {
	httpClient := NewHTTPClient("Test-Crawler/1.0", 30*time.Second)
	defer httpClient.Close()

	parser := NewRobotsParser(httpClient, true) // ignoreRobotsTxt = true
	ctx := context.Background()

	// When ignoring robots.txt, everything should be allowed
	allowed, err := parser.IsAllowed(ctx, "https://example.com/admin/secret", "Test-Crawler")
	if err != nil {
		t.Errorf("Error checking robots.txt: %v", err)
	}
	if !allowed {
		t.Errorf("Expected true when ignoring robots.txt, got false")
	}
}

func TestMatchesPattern(t *testing.T) {
	tests := []struct {
		path     string
		pattern  string
		expected bool
	}{
		{"/admin/page", "/admin/", true},
		{"/admin", "/admin/", false},
		{"/blog/post", "/admin/", false},
		{"/file.pdf", "*.pdf", true},
		{"/path/file.pdf", "*.pdf", true},
		{"/file.doc", "*.pdf", false},
		{"/path/to/file", "/path/*/file", true},
		{"/path/file", "/path/*/file", false},
		{"/exact", "/exact$", true},
		{"/exact/more", "/exact$", false},
	}

	for _, tt := range tests {
		t.Run(tt.path+"_"+tt.pattern, func(t *testing.T) {
			result := matchesPattern(tt.path, tt.pattern)
			if result != tt.expected {
				t.Errorf("matchesPattern(%s, %s) = %v, expected %v",
					tt.path, tt.pattern, result, tt.expected)
			}
		})
	}
}

func TestGetCrawlDelay(t *testing.T) {
	tests := []struct {
		name          string
		robotsTxt     string
		userAgent     string
		expectedDelay time.Duration
	}{
		{
			name: "Valid crawl delay",
			robotsTxt: `User-agent: *
Crawl-delay: 5`,
			userAgent:     "TestBot/1.0",
			expectedDelay: 5 * time.Second,
		},
		{
			name: "No crawl delay specified",
			robotsTxt: `User-agent: *
Disallow: /admin/`,
			userAgent:     "TestBot/1.0",
			expectedDelay: 0,
		},
		{
			name: "Invalid crawl delay format",
			robotsTxt: `User-agent: *
Crawl-delay: invalid`,
			userAgent:     "TestBot/1.0",
			expectedDelay: 0,
		},
		{
			name: "Fractional crawl delay",
			robotsTxt: `User-agent: *
Crawl-delay: 1.5`,
			userAgent:     "TestBot/1.0",
			expectedDelay: 1500 * time.Millisecond,
		},
		{
			name: "Zero crawl delay",
			robotsTxt: `User-agent: *
Crawl-delay: 0`,
			userAgent:     "TestBot/1.0",
			expectedDelay: 0,
		},
		{
			name: "Specific user agent with delay",
			robotsTxt: `User-agent: *
Disallow: /

User-agent: TestCrawler
Crawl-delay: 3`,
			userAgent:     "TestCrawler/1.0",
			expectedDelay: 3 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/robots.txt" {
					w.Header().Set("Content-Type", "text/plain")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(tt.robotsTxt))
				} else {
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()

			// Parse server URL
			u, err := url.Parse(server.URL)
			if err != nil {
				t.Fatalf("Failed to parse server URL: %v", err)
			}

			// Create robots parser
			httpClient := NewHTTPClient("Test-Crawler/1.0", 30*time.Second)
			defer httpClient.Close()
			parser := NewRobotsParser(httpClient, false)

			// Fetch robots.txt first
			ctx := context.Background()
			_, _ = parser.IsAllowed(ctx, server.URL, tt.userAgent)

			// Test delay
			delay := parser.GetCrawlDelay(u.Host)

			if delay != tt.expectedDelay {
				t.Errorf("GetCrawlDelay() = %v, expected %v", delay, tt.expectedDelay)
			}
		})
	}
}

func TestGetCrawlDelayNetworkError(t *testing.T) {
	// Test network error case
	httpClient := NewHTTPClient("Test-Crawler/1.0", 30*time.Second)
	defer httpClient.Close()
	parser := NewRobotsParser(httpClient, false)

	// Use non-existent domain
	delay := parser.GetCrawlDelay("non-existent-domain-12345.com")

	// Should return 0 delay on network error
	if delay != 0 {
		t.Errorf("Expected 0 delay on network error, got %v", delay)
	}
}
