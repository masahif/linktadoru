package crawler

import (
	"context"
	"net/url"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateLimiter manages rate limiting per domain
type RateLimiter struct {
	limiters map[string]*rate.Limiter
	delays   map[string]time.Duration // effective delay per domain, to make SetDomainDelay idempotent
	mu       sync.RWMutex
	delay    time.Duration
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(defaultDelay time.Duration) *RateLimiter {
	return &RateLimiter{
		limiters: make(map[string]*rate.Limiter),
		delays:   make(map[string]time.Duration),
		delay:    defaultDelay,
	}
}

// Wait waits for permission to proceed with a request to the given URL
func (r *RateLimiter) Wait(ctx context.Context, urlStr string) error {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return err
	}

	domain := parsedURL.Host
	limiter := r.getLimiter(domain)

	return limiter.Wait(ctx)
}

// SetDomainDelay sets a custom delay for a specific domain and reports whether
// the delay changed. Calling it again with the same delay is a no-op —
// replacing the limiter would reset its token bucket and effectively disable
// rate limiting when called on every request.
func (r *RateLimiter) SetDomainDelay(domain string, delay time.Duration) bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if delay <= 0 {
		delay = r.delay
	}

	if current, ok := r.delays[domain]; ok && current == delay {
		return false
	}

	r.delays[domain] = delay
	r.limiters[domain] = rate.NewLimiter(rate.Every(delay), 1)
	return true
}

// getLimiter gets or creates a rate limiter for a domain
func (r *RateLimiter) getLimiter(domain string) *rate.Limiter {
	r.mu.RLock()
	limiter, exists := r.limiters[domain]
	r.mu.RUnlock()

	if exists {
		return limiter
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Check again in case another goroutine created it
	if limiter, exists := r.limiters[domain]; exists {
		return limiter
	}

	// Create new limiter with default delay
	limit := rate.Every(r.delay)
	limiter = rate.NewLimiter(limit, 1)
	r.limiters[domain] = limiter
	r.delays[domain] = r.delay

	return limiter
}
