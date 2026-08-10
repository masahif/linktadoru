package crawler

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
)

// urlPolicy is the crawler's single URL authorization decision. Persisted
// depth-zero origins and explicit include expressions add allowed ranges;
// excludes always win.
type urlPolicy struct {
	allowedSchemes  map[string]struct{}
	implicitOrigins map[string]struct{}
	includes        []*regexp.Regexp
	excludes        []*regexp.Regexp
}

func newURLPolicy(allowedSchemes, includes, excludes []string) (*urlPolicy, error) {
	schemes := make(map[string]struct{})
	if len(allowedSchemes) == 0 {
		allowedSchemes = []string{"https://", "http://"}
	}
	for _, value := range allowedSchemes {
		scheme := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(value), "://"))
		if scheme != "" {
			schemes[scheme] = struct{}{}
		}
	}

	includePatterns := make([]*regexp.Regexp, 0, len(includes))
	for _, pattern := range includes {
		if !strings.Contains(pattern, "://") {
			return nil, fmt.Errorf("include pattern %q is relative; rewrite it as an absolute full-URL pattern", pattern)
		}
		compiled, err := regexp.Compile(`\A(?:` + pattern + `)\z`)
		if err != nil {
			return nil, fmt.Errorf("include pattern %q: %w", pattern, err)
		}
		includePatterns = append(includePatterns, compiled)
	}

	excludePatterns, err := compilePatterns(excludes)
	if err != nil {
		return nil, fmt.Errorf("exclude pattern: %w", err)
	}

	return &urlPolicy{
		allowedSchemes:  schemes,
		implicitOrigins: make(map[string]struct{}),
		includes:        includePatterns,
		excludes:        excludePatterns,
	}, nil
}

// compilePatterns compiles ordinary (substring-match) regular expressions.
func compilePatterns(patterns []string) ([]*regexp.Regexp, error) {
	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, pattern := range patterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, fmt.Errorf("pattern %q: %w", pattern, err)
		}
		compiled = append(compiled, re)
	}
	return compiled, nil
}

func (p *urlPolicy) setImplicitOrigins(urls []string) error {
	origins, err := p.origins(urls)
	if err != nil {
		return err
	}
	p.implicitOrigins = origins
	return nil
}

func (p *urlPolicy) validateExplicitURLs(urls []string) error {
	for _, raw := range urls {
		if _, _, err := p.parse(raw); err != nil {
			return fmt.Errorf("invalid seed URL %q: %w", raw, err)
		}
	}
	return nil
}

func (p *urlPolicy) allows(raw string) bool {
	_, origin, err := p.parse(raw)
	if err != nil {
		return false
	}

	_, allowed := p.implicitOrigins[origin]
	if !allowed {
		for _, re := range p.includes {
			if re.MatchString(raw) {
				allowed = true
				break
			}
		}
	}
	if !allowed {
		return false
	}

	for _, re := range p.excludes {
		if re.MatchString(raw) {
			return false
		}
	}
	return true
}

func (p *urlPolicy) origins(urls []string) (map[string]struct{}, error) {
	origins := make(map[string]struct{}, len(urls))
	for _, raw := range urls {
		_, origin, err := p.parse(raw)
		if err != nil {
			return nil, err
		}
		origins[origin] = struct{}{}
	}
	return origins, nil
}

func (p *urlPolicy) parse(raw string) (*url.URL, string, error) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, "", fmt.Errorf("malformed absolute URL: %w", err)
	}
	if !parsed.IsAbs() || parsed.Host == "" {
		return nil, "", fmt.Errorf("URL must be absolute")
	}
	if parsed.User != nil {
		return nil, "", fmt.Errorf("URL userinfo is not allowed")
	}

	scheme := strings.ToLower(parsed.Scheme)
	if _, ok := p.allowedSchemes[scheme]; !ok {
		return nil, "", fmt.Errorf("unsupported URL scheme %q", parsed.Scheme)
	}
	origin, err := originKey(parsed)
	if err != nil {
		return nil, "", err
	}
	return parsed, origin, nil
}

func originKey(parsed *url.URL) (string, error) {
	scheme := strings.ToLower(parsed.Scheme)
	hostname := strings.ToLower(parsed.Hostname())
	if hostname == "" {
		return "", fmt.Errorf("URL host is empty")
	}
	port := parsed.Port()
	if port == "" {
		switch scheme {
		case "http":
			port = "80"
		case "https":
			port = "443"
		}
	}
	host := hostname
	if port != "" {
		host = net.JoinHostPort(hostname, port)
	}
	return scheme + "://" + host, nil
}
