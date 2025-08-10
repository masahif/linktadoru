package crawler

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"github.com/masahif/linktadoru/internal/parser"
)

// DefaultPageProcessor implements the PageProcessor interface
type DefaultPageProcessor struct {
	httpClient     *HTTPClient
	allowedSchemes []string
}

// NewPageProcessor creates a new page processor with default schemes
func NewPageProcessor(httpClient *HTTPClient) PageProcessor {
	return NewPageProcessorWithSchemes(httpClient, []string{"https://", "http://"})
}

// NewPageProcessorWithSchemes creates a new page processor with custom allowed schemes
func NewPageProcessorWithSchemes(httpClient *HTTPClient, allowedSchemes []string) PageProcessor {
	return &DefaultPageProcessor{
		httpClient:     httpClient,
		allowedSchemes: allowedSchemes,
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

	// Convert HTTP headers to map[string]string
	headerMap := make(map[string]string)
	for name, values := range resp.Headers {
		if len(values) > 0 {
			// Use the first value for simplicity, could concatenate multiple values
			headerMap[strings.ToLower(name)] = values[0]
		}
	}

	// Create page data
	pageData := &PageData{
		URL:          url,
		StatusCode:   resp.StatusCode,
		TTFB:         resp.Metrics.TTFB,
		DownloadTime: resp.Metrics.DownloadTime,
		ResponseSize: int64(len(resp.Body)),
		HTTPHeaders:  headerMap,
		CrawledAt:    time.Now().UTC(),
	}

	result := &PageResult{
		Page:  pageData,
		Links: []*LinkData{},
	}

	// Only parse HTML content
	if !isHTML || resp.StatusCode >= 400 {
		slog.Debug("Skipping HTML parsing", "url", url, "is_html", isHTML, "status_code", resp.StatusCode)
		return result, nil
	}

	// Parse HTML with configured allowed schemes
	htmlParser, err := parser.NewHTMLParserWithSchemes(resp.FinalURL, p.allowedSchemes)
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
	slog.Debug("Found links", "url", url, "links_count", len(parseResult.Links))
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
		slog.Debug("Added link", "source", resp.FinalURL, "target", link.URL, "type", linkType)
	}

	return result, nil
}
