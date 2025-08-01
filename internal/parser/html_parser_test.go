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