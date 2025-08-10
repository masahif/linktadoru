package parser

import (
	"testing"
)

func TestHTMLParser(t *testing.T) {
	htmlContent := `
<!DOCTYPE html>
<html>
<head>
	<title>Test Page Title</title>
	<meta name="description" content="This is a test description">
	<meta name="robots" content="index,follow">
	<link rel="canonical" href="https://example.com/canonical-page">
</head>
<body>
	<h1>Test Page</h1>
	<p>Some content</p>
	<a href="/relative-link">Relative Link</a>
	<a href="https://example.com/absolute-link">Absolute Link</a>
	<a href="https://external.com/page" rel="nofollow">External Link</a>
	<a href="#anchor">Anchor Link</a>
	<a href="javascript:void(0)">JavaScript Link</a>
	<a href="/page-with-text">Link with <span>nested</span> text</a>
</body>
</html>
`

	parser, err := NewHTMLParser("https://example.com/test-page")
	if err != nil {
		t.Fatalf("Failed to create parser: %v", err)
	}

	result, err := parser.Parse([]byte(htmlContent))
	if err != nil {
		t.Fatalf("Failed to parse HTML: %v", err)
	}

	// Test metadata extraction
	if result.Title != "Test Page Title" {
		t.Errorf("Expected title 'Test Page Title', got '%s'", result.Title)
	}

	if result.MetaDesc != "This is a test description" {
		t.Errorf("Expected description 'This is a test description', got '%s'", result.MetaDesc)
	}

	if result.MetaRobots != "index,follow" {
		t.Errorf("Expected robots 'index,follow', got '%s'", result.MetaRobots)
	}

	if result.CanonicalURL != "https://example.com/canonical-page" {
		t.Errorf("Expected canonical URL 'https://example.com/canonical-page', got '%s'", result.CanonicalURL)
	}

	// Test content hash
	if result.ContentHash == "" {
		t.Error("Expected non-empty content hash")
	}

	// Test link extraction
	expectedLinks := []struct {
		url        string
		anchorText string
		isExternal bool
		rel        string
	}{
		{"https://example.com/relative-link", "Relative Link", false, ""},
		{"https://example.com/absolute-link", "Absolute Link", false, ""},
		{"https://external.com/page", "External Link", true, "nofollow"},
		{"https://example.com/page-with-text", "Link with nested text", false, ""},
	}

	if len(result.Links) != len(expectedLinks) {
		t.Fatalf("Expected %d links, got %d", len(expectedLinks), len(result.Links))
	}

	for i, expected := range expectedLinks {
		link := result.Links[i]
		if link.URL != expected.url {
			t.Errorf("Link %d: expected URL '%s', got '%s'", i, expected.url, link.URL)
		}
		if link.AnchorText != expected.anchorText {
			t.Errorf("Link %d: expected anchor text '%s', got '%s'", i, expected.anchorText, link.AnchorText)
		}
		if link.IsExternal != expected.isExternal {
			t.Errorf("Link %d: expected IsExternal %v, got %v", i, expected.isExternal, link.IsExternal)
		}
		if link.RelAttribute != expected.rel {
			t.Errorf("Link %d: expected rel '%s', got '%s'", i, expected.rel, link.RelAttribute)
		}
	}
}

func TestHTMLParserRelativeCanonical(t *testing.T) {
	htmlContent := `
<!DOCTYPE html>
<html>
<head>
	<link rel="canonical" href="/canonical-page">
</head>
</html>
`

	parser, err := NewHTMLParser("https://example.com/test-page")
	if err != nil {
		t.Fatalf("Failed to create parser: %v", err)
	}

	result, err := parser.Parse([]byte(htmlContent))
	if err != nil {
		t.Fatalf("Failed to parse HTML: %v", err)
	}

	if result.CanonicalURL != "https://example.com/canonical-page" {
		t.Errorf("Expected canonical URL 'https://example.com/canonical-page', got '%s'", result.CanonicalURL)
	}
}

func TestHTMLParserEmptyContent(t *testing.T) {
	parser, err := NewHTMLParser("https://example.com/")
	if err != nil {
		t.Fatalf("Failed to create parser: %v", err)
	}

	result, err := parser.Parse([]byte(""))
	if err != nil {
		t.Fatalf("Failed to parse empty HTML: %v", err)
	}

	if result.Title != "" || result.MetaDesc != "" || len(result.Links) != 0 {
		t.Error("Expected empty results for empty HTML")
	}
}

func TestHTMLParserSchemeFiltering(t *testing.T) {
	tests := []struct {
		name           string
		baseURL        string
		allowedSchemes []string
		htmlContent    string
		expectedLinks  int
		expectedURLs   []string
	}{
		{
			name:           "Filter tel and mailto links",
			baseURL:        "https://example.com",
			allowedSchemes: []string{"https://", "http://"},
			htmlContent: `<html><body>
				<a href="tel:1234567890">Call</a>
				<a href="mailto:test@example.com">Email</a>
				<a href="https://example.com/page">Valid Link</a>
				<a href="http://example.com/page2">HTTP Link</a>
			</body></html>`,
			expectedLinks: 2,
			expectedURLs:  []string{"https://example.com/page", "http://example.com/page2"},
		},
		{
			name:           "Filter chrome-extension links",
			baseURL:        "https://example.com",
			allowedSchemes: []string{"https://", "http://"},
			htmlContent: `<html><body>
				<a href="chrome-extension://abc123/popup.html">Extension</a>
				<a href="https://example.com/page">Valid Link</a>
				<a href="ftp://ftp.example.com/file.txt">FTP Link</a>
			</body></html>`,
			expectedLinks: 1,
			expectedURLs:  []string{"https://example.com/page"},
		},
		{
			name:           "Allow custom schemes when configured",
			baseURL:        "https://example.com",
			allowedSchemes: []string{"https://", "http://", "ftp://"},
			htmlContent: `<html><body>
				<a href="ftp://ftp.example.com/file.txt">FTP Link</a>
				<a href="https://example.com/page">HTTPS Link</a>
				<a href="tel:1234567890">Tel Link</a>
			</body></html>`,
			expectedLinks: 2,
			expectedURLs:  []string{"ftp://ftp.example.com/file.txt", "https://example.com/page"},
		},
		{
			name:           "Relative links are always allowed",
			baseURL:        "https://example.com",
			allowedSchemes: []string{"https://"},
			htmlContent: `<html><body>
				<a href="/relative/path">Relative Link</a>
				<a href="http://other.com/page">HTTP Link</a>
				<a href="mailto:test@example.com">Email</a>
			</body></html>`,
			expectedLinks: 1,
			expectedURLs:  []string{"https://example.com/relative/path"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser, err := NewHTMLParserWithSchemes(tt.baseURL, tt.allowedSchemes)
			if err != nil {
				t.Fatalf("Failed to create parser: %v", err)
			}

			result, err := parser.Parse([]byte(tt.htmlContent))
			if err != nil {
				t.Fatalf("Failed to parse HTML: %v", err)
			}

			if len(result.Links) != tt.expectedLinks {
				t.Errorf("Expected %d links, got %d", tt.expectedLinks, len(result.Links))
			}

			// Verify expected URLs are present
			actualURLs := make([]string, len(result.Links))
			for i, link := range result.Links {
				actualURLs[i] = link.URL
			}

			for _, expectedURL := range tt.expectedURLs {
				found := false
				for _, actualURL := range actualURLs {
					if actualURL == expectedURL {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected URL %s not found in results: %v", expectedURL, actualURLs)
				}
			}
		})
	}
}

func TestIsAllowedScheme(t *testing.T) {
	tests := []struct {
		name           string
		allowedSchemes []string
		href           string
		expected       bool
	}{
		{"HTTPS allowed", []string{"https://", "http://"}, "https://example.com", true},
		{"HTTP allowed", []string{"https://", "http://"}, "http://example.com", true},
		{"Tel blocked", []string{"https://", "http://"}, "tel:1234567890", false},
		{"Mailto blocked", []string{"https://", "http://"}, "mailto:test@example.com", false},
		{"Chrome extension blocked", []string{"https://", "http://"}, "chrome-extension://abc123/popup.html", false},
		{"FTP blocked by default", []string{"https://", "http://"}, "ftp://ftp.example.com", false},
		{"FTP allowed when configured", []string{"https://", "http://", "ftp://"}, "ftp://ftp.example.com", true},
		{"Relative URL allowed", []string{"https://", "http://"}, "/relative/path", true},
		{"Relative URL with query allowed", []string{"https://", "http://"}, "/path?query=1", true},
		{"JavaScript blocked", []string{"https://", "http://"}, "javascript:alert('hi')", false},
	}

	parser := &HTMLParser{allowedSchemes: []string{"https://", "http://"}}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Update parser schemes for test
			parser.allowedSchemes = tt.allowedSchemes
			result := parser.isAllowedScheme(tt.href)
			if result != tt.expected {
				t.Errorf("isAllowedScheme(%q) = %v, expected %v", tt.href, result, tt.expected)
			}
		})
	}
}
