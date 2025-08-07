package crawler

import (
	"bufio"
	"context"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"
)

// RobotsParser handles robots.txt parsing and rule checking
type RobotsParser struct {
	httpClient   *HTTPClient
	rules        map[string]*RobotRules
	mu           sync.RWMutex
	ignoreRobots bool
}

// RobotRules contains the parsed rules for a domain
type RobotRules struct {
	Disallowed []string
	Allowed    []string
	CrawlDelay time.Duration
	Sitemap    []string
}

// NewRobotsParser creates a new robots.txt parser
func NewRobotsParser(httpClient *HTTPClient, ignoreRobots bool) *RobotsParser {
	return &RobotsParser{
		httpClient:   httpClient,
		rules:        make(map[string]*RobotRules),
		ignoreRobots: ignoreRobots,
	}
}

// IsAllowed checks if a URL is allowed by robots.txt
func (r *RobotsParser) IsAllowed(ctx context.Context, urlStr string, userAgent string) (bool, error) {
	if r.ignoreRobots {
		return true, nil
	}

	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return false, fmt.Errorf("invalid URL: %w", err)
	}

	domain := parsedURL.Host
	rules, err := r.getRules(ctx, domain, parsedURL.Scheme)
	if err != nil {
		// If we can't fetch robots.txt, assume allowed
		return true, nil
	}

	path := parsedURL.Path
	if path == "" {
		path = "/"
	}

	// Check disallow rules first
	for _, pattern := range rules.Disallowed {
		if matchesPattern(path, pattern) {
			// Check if there's a more specific allow rule
			for _, allowPattern := range rules.Allowed {
				if matchesPattern(path, allowPattern) && len(allowPattern) > len(pattern) {
					return true, nil
				}
			}
			return false, nil
		}
	}

	return true, nil
}

// GetCrawlDelay returns the crawl delay for a domain
func (r *RobotsParser) GetCrawlDelay(domain string) time.Duration {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if rules, ok := r.rules[domain]; ok {
		return rules.CrawlDelay
	}

	return 0
}

// getRules fetches and parses robots.txt for a domain
func (r *RobotsParser) getRules(ctx context.Context, domain, scheme string) (*RobotRules, error) {
	r.mu.RLock()
	rules, exists := r.rules[domain]
	r.mu.RUnlock()

	if exists {
		return rules, nil
	}

	// Fetch robots.txt
	robotsURL := fmt.Sprintf("%s://%s/robots.txt", scheme, domain)
	resp, err := r.httpClient.Get(ctx, robotsURL)
	if err != nil {
		return nil, err
	}

	switch resp.StatusCode {
	case 404:
		// No robots.txt means everything is allowed
		rules = &RobotRules{
			Disallowed: []string{},
			Allowed:    []string{},
			CrawlDelay: 0,
		}
	case 200:
		rules = r.parseRobotsTxt(string(resp.Body))
	default:
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	r.mu.Lock()
	r.rules[domain] = rules
	r.mu.Unlock()

	return rules, nil
}

// parseRobotsTxt parses robots.txt content
func (r *RobotsParser) parseRobotsTxt(content string) *RobotRules {
	rules := &RobotRules{
		Disallowed: []string{},
		Allowed:    []string{},
		Sitemap:    []string{},
	}

	scanner := bufio.NewScanner(strings.NewReader(content))
	inUserAgent := false
	currentUserAgent := ""

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse directive
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		directive := strings.ToLower(strings.TrimSpace(parts[0]))
		value := strings.TrimSpace(parts[1])

		switch directive {
		case "user-agent":
			currentUserAgent = strings.ToLower(value)
			inUserAgent = (currentUserAgent == "*" || strings.Contains(currentUserAgent, "crawler"))

		case "disallow":
			if inUserAgent && value != "" {
				rules.Disallowed = append(rules.Disallowed, value)
			}

		case "allow":
			if inUserAgent && value != "" {
				rules.Allowed = append(rules.Allowed, value)
			}

		case "crawl-delay":
			if inUserAgent {
				if delay, err := time.ParseDuration(value + "s"); err == nil {
					rules.CrawlDelay = delay
				}
			}

		case "sitemap":
			rules.Sitemap = append(rules.Sitemap, value)
		}
	}

	return rules
}

// matchesPattern checks if a path matches a robots.txt pattern
func matchesPattern(path, pattern string) bool {
	// Handle wildcard
	if strings.Contains(pattern, "*") {
		// Convert pattern to regex-like matching
		pattern = strings.ReplaceAll(pattern, "*", ".*")
		// For simplicity, we'll do prefix matching with wildcards
		parts := strings.Split(pattern, ".*")
		if len(parts) == 1 {
			return strings.HasPrefix(path, parts[0])
		}

		// Check if path starts with first part
		if !strings.HasPrefix(path, parts[0]) {
			return false
		}

		// Check if path contains subsequent parts in order
		remaining := path[len(parts[0]):]
		for i := 1; i < len(parts); i++ {
			if parts[i] == "" {
				continue
			}
			idx := strings.Index(remaining, parts[i])
			if idx == -1 {
				return false
			}
			remaining = remaining[idx+len(parts[i]):]
		}

		return true
	}

	// Handle $ (end of URL)
	if strings.HasSuffix(pattern, "$") {
		pattern = strings.TrimSuffix(pattern, "$")
		return path == pattern
	}

	// Default: prefix matching
	return strings.HasPrefix(path, pattern)
}
