// Package parser provides HTML parsing and content extraction capabilities.
// It extracts metadata, links, and other relevant information from HTML documents.
package parser

import (
	"crypto/sha256"
	"fmt"
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// HTMLParser extracts metadata and links from HTML
type HTMLParser struct {
	baseURL        *url.URL
	allowedSchemes []string
}

// ParseResult contains the parsed HTML data
type ParseResult struct {
	Title        string
	MetaDesc     string
	MetaRobots   string
	CanonicalURL string
	ContentHash  string
	Links        []Link
}

// Link represents a parsed link
type Link struct {
	URL          string
	AnchorText   string
	RelAttribute string
	IsExternal   bool
}

// NewHTMLParser creates a new HTML parser with default allowed schemes
func NewHTMLParser(baseURL string) (*HTMLParser, error) {
	return NewHTMLParserWithSchemes(baseURL, []string{"https://", "http://"})
}

// NewHTMLParserWithSchemes creates a new HTML parser with custom allowed schemes
func NewHTMLParserWithSchemes(baseURL string, allowedSchemes []string) (*HTMLParser, error) {
	parsedURL, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	if len(allowedSchemes) == 0 {
		allowedSchemes = []string{"https://", "http://"}
	}

	return &HTMLParser{
		baseURL:        parsedURL,
		allowedSchemes: allowedSchemes,
	}, nil
}

// Parse parses HTML content and extracts metadata and links.
// It extracts title, meta description, meta robots, canonical URL,
// and all links from the HTML document. The content hash is computed
// for duplicate detection purposes.
func (p *HTMLParser) Parse(htmlContent []byte) (*ParseResult, error) {
	doc, err := html.Parse(strings.NewReader(string(htmlContent)))
	if err != nil {
		return nil, fmt.Errorf("failed to parse HTML: %w", err)
	}

	result := &ParseResult{
		Links: []Link{},
	}

	// Extract metadata and links
	p.traverse(doc, result)

	// Generate content hash
	hash := sha256.Sum256(htmlContent)
	result.ContentHash = fmt.Sprintf("%x", hash)

	return result, nil
}

// traverse recursively walks the HTML tree
func (p *HTMLParser) traverse(n *html.Node, result *ParseResult) {
	if n.Type == html.ElementNode {
		switch n.Data {
		case "title":
			if n.FirstChild != nil && n.FirstChild.Type == html.TextNode {
				result.Title = strings.TrimSpace(n.FirstChild.Data)
			}

		case "meta":
			p.parseMeta(n, result)

		case "link":
			p.parseLink(n, result)

		case "a":
			p.parseAnchor(n, result)
		}
	}

	// Traverse children
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		p.traverse(c, result)
	}
}

// parseMeta extracts metadata from meta tags
func (p *HTMLParser) parseMeta(n *html.Node, result *ParseResult) {
	var name, content string

	for _, attr := range n.Attr {
		switch attr.Key {
		case "name":
			name = strings.ToLower(attr.Val)
		case "content":
			content = attr.Val
		}
	}

	switch name {
	case "description":
		result.MetaDesc = content
	case "robots":
		result.MetaRobots = content
	}
}

// parseLink extracts canonical URL from link tags
func (p *HTMLParser) parseLink(n *html.Node, result *ParseResult) {
	var rel, href string

	for _, attr := range n.Attr {
		switch attr.Key {
		case "rel":
			rel = strings.ToLower(attr.Val)
		case "href":
			href = attr.Val
		}
	}

	if rel == "canonical" && href != "" {
		// Make canonical URL absolute
		if absURL, err := p.resolveURL(href); err == nil {
			result.CanonicalURL = absURL
		}
	}
}

// parseAnchor extracts links from anchor tags
func (p *HTMLParser) parseAnchor(n *html.Node, result *ParseResult) {
	var href, rel string

	for _, attr := range n.Attr {
		switch attr.Key {
		case "href":
			href = attr.Val
		case "rel":
			rel = attr.Val
		}
	}

	if href == "" || strings.HasPrefix(href, "#") || strings.HasPrefix(href, "javascript:") {
		return
	}

	// Early scheme validation before URL resolution
	if !p.isAllowedScheme(href) {
		return
	}

	// Extract anchor text
	anchorText := p.extractText(n)

	// Resolve relative URL
	absURL, err := p.resolveURL(href)
	if err != nil {
		return
	}

	// Validate resolved URL scheme
	if !p.isAllowedScheme(absURL) {
		return
	}

	// Check if external
	parsedURL, err := url.Parse(absURL)
	if err != nil {
		return
	}

	isExternal := parsedURL.Host != p.baseURL.Host

	link := Link{
		URL:          absURL,
		AnchorText:   strings.TrimSpace(anchorText),
		RelAttribute: rel,
		IsExternal:   isExternal,
	}

	result.Links = append(result.Links, link)
}

// resolveURL converts relative URLs to absolute URLs
func (p *HTMLParser) resolveURL(href string) (string, error) {
	u, err := url.Parse(href)
	if err != nil {
		return "", err
	}

	// Resolve relative to base URL
	resolved := p.baseURL.ResolveReference(u)
	return resolved.String(), nil
}

// extractText recursively extracts text content from a node
func (p *HTMLParser) extractText(n *html.Node) string {
	if n.Type == html.TextNode {
		return strings.TrimSpace(n.Data)
	}

	var parts []string
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		text := p.extractText(c)
		if text != "" {
			parts = append(parts, text)
		}
	}

	return strings.Join(parts, " ")
}

// isAllowedScheme checks if the URL has an allowed scheme
func (p *HTMLParser) isAllowedScheme(href string) bool {
	// Check for absolute URLs with schemes
	if strings.Contains(href, "://") {
		// Check against allowed schemes
		for _, scheme := range p.allowedSchemes {
			if strings.HasPrefix(href, scheme) {
				return true
			}
		}
		return false
	}

	// Check for other protocol schemes like tel:, mailto:, javascript: without ://
	if strings.Contains(href, ":") && !strings.HasPrefix(href, "/") && !strings.HasPrefix(href, "?") && !strings.HasPrefix(href, "#") {
		// This is likely a scheme-based URL like tel:, mailto:, javascript:
		// Only allow if it matches our allowed schemes (unlikely for these cases)
		for _, scheme := range p.allowedSchemes {
			if strings.HasPrefix(href, strings.TrimSuffix(scheme, "://")) {
				return true
			}
		}
		return false
	}

	// For relative URLs (no scheme), they're allowed (will be resolved to base URL's scheme)
	return true
}
