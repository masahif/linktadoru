package crawler

import (
	"net/url"
	"testing"

	"github.com/masahif/linktadoru/internal/config"
)

func TestShouldCrawlURL(t *testing.T) {
	tests := []struct {
		name            string
		includePatterns []string
		excludePatterns []string
		url             string
		expected        bool
	}{
		{
			name:            "No patterns - should allow all",
			includePatterns: []string{},
			excludePatterns: []string{},
			url:             "https://example.com/page",
			expected:        true,
		},
		{
			name:            "Include pattern match",
			includePatterns: []string{"^https?://[^/]*example\\.com/.*"},
			excludePatterns: []string{},
			url:             "https://example.com/page",
			expected:        true,
		},
		{
			name:            "Include pattern no match",
			includePatterns: []string{"^https?://[^/]*example\\.com/.*"},
			excludePatterns: []string{},
			url:             "https://other.com/page",
			expected:        false,
		},
		{
			name:            "Multiple include patterns - first matches",
			includePatterns: []string{"^https?://[^/]*example\\.com/.*", "^https?://[^/]*blog\\.com/.*"},
			excludePatterns: []string{},
			url:             "https://example.com/page",
			expected:        true,
		},
		{
			name:            "Multiple include patterns - second matches",
			includePatterns: []string{"^https?://[^/]*example\\.com/.*", "^https?://[^/]*blog\\.com/.*"},
			excludePatterns: []string{},
			url:             "https://blog.com/post",
			expected:        true,
		},
		{
			name:            "Multiple include patterns - none match",
			includePatterns: []string{"^https?://[^/]*example\\.com/.*", "^https?://[^/]*blog\\.com/.*"},
			excludePatterns: []string{},
			url:             "https://other.com/page",
			expected:        false,
		},
		{
			name:            "Exclude pattern match",
			includePatterns: []string{},
			excludePatterns: []string{"\\.pdf$"},
			url:             "https://example.com/file.pdf",
			expected:        false,
		},
		{
			name:            "Exclude pattern no match",
			includePatterns: []string{},
			excludePatterns: []string{"\\.pdf$"},
			url:             "https://example.com/page.html",
			expected:        true,
		},
		{
			name:            "Include match but exclude match - exclude wins",
			includePatterns: []string{"^https?://[^/]*example\\.com/.*"},
			excludePatterns: []string{"\\.pdf$"},
			url:             "https://example.com/file.pdf",
			expected:        false,
		},
		{
			name:            "Include match and exclude no match - include wins",
			includePatterns: []string{"^https?://[^/]*example\\.com/.*"},
			excludePatterns: []string{"\\.pdf$"},
			url:             "https://example.com/page.html",
			expected:        true,
		},
		{
			name:            "Complex include pattern with path",
			includePatterns: []string{"^https?://[^/]*example\\.com/blog/.*"},
			excludePatterns: []string{},
			url:             "https://example.com/blog/post1",
			expected:        true,
		},
		{
			name:            "Complex include pattern with path - no match",
			includePatterns: []string{"^https?://[^/]*example\\.com/blog/.*"},
			excludePatterns: []string{},
			url:             "https://example.com/news/post1",
			expected:        false,
		},
		{
			name:            "Multiple exclude patterns",
			includePatterns: []string{},
			excludePatterns: []string{"\\.pdf$", "/admin/.*", ".*\\?print=1"},
			url:             "https://example.com/admin/panel",
			expected:        false,
		},
		{
			name:            "Admin exclude pattern",
			includePatterns: []string{},
			excludePatterns: []string{"/admin/.*"},
			url:             "https://example.com/admin/panel",
			expected:        false,
		},
		{
			name:            "Print parameter exclude",
			includePatterns: []string{},
			excludePatterns: []string{".*\\?print=1"},
			url:             "https://example.com/page?print=1",
			expected:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			crawler := &DefaultCrawler{
				config: &config.CrawlConfig{
					IncludePatterns:     tt.includePatterns,
					ExcludePatterns:     tt.excludePatterns,
					FollowExternalHosts: true, // Allow all hosts for legacy tests
				},
				allowedHosts: []string{}, // Empty list but external hosts allowed
			}

			result := crawler.shouldCrawlURL(tt.url)
			if result != tt.expected {
				t.Errorf("shouldCrawlURL() = %v, expected %v for URL %s", result, tt.expected, tt.url)
			}
		})
	}
}

func TestIsAllowedHost(t *testing.T) {
	tests := []struct {
		name             string
		seedURLs         []string
		followExternal   bool
		targetURL        string
		expected         bool
	}{
		{
			name:           "Same host allowed - exact match",
			seedURLs:       []string{"https://example.com"},
			followExternal: false,
			targetURL:      "https://example.com/page",
			expected:       true,
		},
		{
			name:           "Different host blocked",
			seedURLs:       []string{"https://example.com"},
			followExternal: false,
			targetURL:      "https://other.com/page",
			expected:       false,
		},
		{
			name:           "Different scheme blocked",
			seedURLs:       []string{"https://example.com"},
			followExternal: false,
			targetURL:      "http://example.com/page",
			expected:       false,
		},
		{
			name:           "Different port blocked",
			seedURLs:       []string{"https://example.com"},
			followExternal: false,
			targetURL:      "https://example.com:8080/page",
			expected:       false,
		},
		{
			name:           "Subdomain blocked",
			seedURLs:       []string{"https://example.com"},
			followExternal: false,
			targetURL:      "https://www.example.com/page",
			expected:       false,
		},
		{
			name:           "Multiple seed URLs - first host matches",
			seedURLs:       []string{"https://example.com", "https://blog.com"},
			followExternal: false,
			targetURL:      "https://example.com/page",
			expected:       true,
		},
		{
			name:           "Multiple seed URLs - second host matches",
			seedURLs:       []string{"https://example.com", "https://blog.com"},
			followExternal: false,
			targetURL:      "https://blog.com/post",
			expected:       true,
		},
		{
			name:           "Multiple seed URLs - no match",
			seedURLs:       []string{"https://example.com", "https://blog.com"},
			followExternal: false,
			targetURL:      "https://other.com/page",
			expected:       false,
		},
		{
			name:           "External hosts allowed - different host",
			seedURLs:       []string{"https://example.com"},
			followExternal: true,
			targetURL:      "https://other.com/page",
			expected:       true,
		},
		{
			name:           "External hosts allowed - same host",
			seedURLs:       []string{"https://example.com"},
			followExternal: true,
			targetURL:      "https://example.com/page",
			expected:       true,
		},
		{
			name:           "Invalid URL",
			seedURLs:       []string{"https://example.com"},
			followExternal: false,
			targetURL:      "invalid-url",
			expected:       false,
		},
		{
			name:           "Empty seed URLs - external disabled",
			seedURLs:       []string{},
			followExternal: false,
			targetURL:      "https://example.com/page",
			expected:       false,
		},
		{
			name:           "Empty seed URLs - external enabled",
			seedURLs:       []string{},
			followExternal: true,
			targetURL:      "https://example.com/page",
			expected:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a crawler with the test configuration
			config := &config.CrawlConfig{
				SeedURLs:            tt.seedURLs,
				FollowExternalHosts: tt.followExternal,
			}

			// Extract allowed hosts like NewCrawler does
			allowedHosts := make([]string, 0, len(config.SeedURLs))
			for _, seedURL := range config.SeedURLs {
				if parsedURL, err := url.Parse(seedURL); err == nil {
					host := parsedURL.Scheme + "://" + parsedURL.Host
					// Avoid duplicates
					found := false
					for _, existing := range allowedHosts {
						if existing == host {
							found = true
							break
						}
					}
					if !found {
						allowedHosts = append(allowedHosts, host)
					}
				}
			}

			crawler := &DefaultCrawler{
				config:       config,
				allowedHosts: allowedHosts,
			}

			result := crawler.isAllowedHost(tt.targetURL)
			if result != tt.expected {
				t.Errorf("isAllowedHost() = %v, expected %v for URL %s with seeds %v", result, tt.expected, tt.targetURL, tt.seedURLs)
			}
		})
	}
}

func TestShouldCrawlURLWithHostFiltering(t *testing.T) {
	tests := []struct {
		name             string
		seedURLs         []string
		followExternal   bool
		includePatterns  []string
		excludePatterns  []string
		targetURL        string
		expected         bool
	}{
		{
			name:            "Same host - no patterns",
			seedURLs:        []string{"https://example.com"},
			followExternal:  false,
			includePatterns: []string{},
			excludePatterns: []string{},
			targetURL:       "https://example.com/page",
			expected:        true,
		},
		{
			name:            "External host blocked - no patterns",
			seedURLs:        []string{"https://example.com"},
			followExternal:  false,
			includePatterns: []string{},
			excludePatterns: []string{},
			targetURL:       "https://other.com/page",
			expected:        false,
		},
		{
			name:            "External host allowed - no patterns",
			seedURLs:        []string{"https://example.com"},
			followExternal:  true,
			includePatterns: []string{},
			excludePatterns: []string{},
			targetURL:       "https://other.com/page",
			expected:        true,
		},
		{
			name:            "Same host - matches include pattern",
			seedURLs:        []string{"https://example.com"},
			followExternal:  false,
			includePatterns: []string{".*example\\.com.*"},
			excludePatterns: []string{},
			targetURL:       "https://example.com/page",
			expected:        true,
		},
		{
			name:            "External host - matches include pattern but blocked by host filter",
			seedURLs:        []string{"https://example.com"},
			followExternal:  false,
			includePatterns: []string{".*other\\.com.*"},
			excludePatterns: []string{},
			targetURL:       "https://other.com/page",
			expected:        false, // Host filter takes precedence
		},
		{
			name:            "Same host - blocked by exclude pattern",
			seedURLs:        []string{"https://example.com"},
			followExternal:  false,
			includePatterns: []string{},
			excludePatterns: []string{"\\.pdf$"},
			targetURL:       "https://example.com/file.pdf",
			expected:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a crawler with the test configuration
			config := &config.CrawlConfig{
				SeedURLs:            tt.seedURLs,
				FollowExternalHosts: tt.followExternal,
				IncludePatterns:     tt.includePatterns,
				ExcludePatterns:     tt.excludePatterns,
			}

			// Extract allowed hosts like NewCrawler does
			allowedHosts := make([]string, 0, len(config.SeedURLs))
			for _, seedURL := range config.SeedURLs {
				if parsedURL, err := url.Parse(seedURL); err == nil {
					host := parsedURL.Scheme + "://" + parsedURL.Host
					// Avoid duplicates
					found := false
					for _, existing := range allowedHosts {
						if existing == host {
							found = true
							break
						}
					}
					if !found {
						allowedHosts = append(allowedHosts, host)
					}
				}
			}

			crawler := &DefaultCrawler{
				config:       config,
				allowedHosts: allowedHosts,
			}

			result := crawler.shouldCrawlURL(tt.targetURL)
			if result != tt.expected {
				t.Errorf("shouldCrawlURL() = %v, expected %v for URL %s with config %+v", result, tt.expected, tt.targetURL, config)
			}
		})
	}
}
