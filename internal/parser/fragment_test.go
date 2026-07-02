package parser

import (
	"strings"
	"testing"
)

// Fragments identify positions within one page, not distinct resources.
// resolveURL must strip them so "…/page#a" and "…/page#b" dedupe to the same
// pages row instead of being crawled twice.
func TestParseStripsURLFragments(t *testing.T) {
	p, err := NewHTMLParser("https://example.com/base")
	if err != nil {
		t.Fatalf("NewHTMLParser: %v", err)
	}

	html := `<html><body>
		<a href="/page#section-a">a</a>
		<a href="/page#section-b">b</a>
		<a href="https://example.com/other#top">c</a>
	</body></html>`

	result, err := p.Parse([]byte(html))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	seen := map[string]int{}
	for _, link := range result.Links {
		if strings.Contains(link.URL, "#") {
			t.Errorf("fragment not stripped: %s", link.URL)
		}
		seen[link.URL]++
	}
	if seen["https://example.com/page"] != 2 {
		t.Errorf("expected both fragment variants to resolve to the same URL, got %v", seen)
	}
	if seen["https://example.com/other"] != 1 {
		t.Errorf("expected absolute link without fragment, got %v", seen)
	}
}
