package crawler

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/masahif/linktadoru/internal/parser"
)

// DefaultPageProcessor implements the PageProcessor interface
type DefaultPageProcessor struct {
	httpClient *HTTPClient
}

// NewPageProcessor creates a new page processor
func NewPageProcessor(httpClient *HTTPClient) PageProcessor {
	return &DefaultPageProcessor{
		httpClient: httpClient,
	}
}

// Process processes a single page
func (p *DefaultPageProcessor) Process(ctx context.Context, url string) (*PageResult, error) {
	// Fetch the page
	resp, err := p.httpClient.Get(ctx, url)
	if err != nil {
		return &PageResult{
			Error: &CrawlError{
				URL:          url,
				ErrorType:    "network_error",
				ErrorMessage: err.Error(),
				OccurredAt:   time.Now().UTC(),
			},
		}, nil
	}

	// Check if content is HTML
	isHTML := false
	if ct := resp.ContentType; ct != "" {
		isHTML = strings.HasPrefix(ct, "text/html") ||
			strings.HasPrefix(ct, "application/xhtml+xml")
	}

	// Create page data
	pageData := &PageData{
		URL:             url,
		StatusCode:      resp.StatusCode,
		TTFB:            resp.Metrics.TTFB,
		DownloadTime:    resp.Metrics.DownloadTime,
		ResponseSize:    int64(len(resp.Body)),
		ContentType:     resp.ContentType,
		ContentLength:   resp.ContentLength,
		LastModified:    resp.LastModified,
		Server:          resp.Server,
		ContentEncoding: resp.ContentEncoding,
		CrawledAt:       time.Now().UTC(),
	}

	result := &PageResult{
		Page:  pageData,
		Links: []*LinkData{},
	}

	// Only parse HTML content
	if !isHTML || resp.StatusCode >= 400 {
		log.Printf("Skipping HTML parsing for %s: isHTML=%v, statusCode=%d", url, isHTML, resp.StatusCode)
		return result, nil
	}

	// Parse HTML
	htmlParser, err := parser.NewHTMLParser(resp.FinalURL)
	if err != nil {
		return result, nil
	}

	parseResult, err := htmlParser.Parse(resp.Body)
	if err != nil {
		return result, nil
	}

	// Update page data with parsed metadata
	pageData.Title = parseResult.Title
	pageData.MetaDesc = parseResult.MetaDesc
	pageData.MetaRobots = parseResult.MetaRobots
	pageData.CanonicalURL = parseResult.CanonicalURL
	pageData.ContentHash = parseResult.ContentHash

	// Convert parsed links to LinkData
	log.Printf("Found %d links in %s", len(parseResult.Links), url)
	for _, link := range parseResult.Links {
		linkType := "internal"
		if link.IsExternal {
			linkType = "external"
		}

		linkData := &LinkData{
			SourceURL:    resp.FinalURL, // Use final URL after redirects
			TargetURL:    link.URL,
			AnchorText:   link.AnchorText,
			LinkType:     linkType,
			RelAttribute: link.RelAttribute,
			CrawledAt:    time.Now().UTC(),
		}

		result.Links = append(result.Links, linkData)
		log.Printf("Added link: %s -> %s (%s)", resp.FinalURL, link.URL, linkType)
	}

	return result, nil
}
