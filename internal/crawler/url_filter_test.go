package crawler

import (
	"testing"

	"linktadoru/internal/config"
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
					IncludePatterns: tt.includePatterns,
					ExcludePatterns: tt.excludePatterns,
				},
			}

			result := crawler.shouldCrawlURL(tt.url)
			if result != tt.expected {
				t.Errorf("shouldCrawlURL() = %v, expected %v for URL %s", result, tt.expected, tt.url)
			}
		})
	}
}
