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
	mu       sync.RWMutex
	delay    time.Duration
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(defaultDelay time.Duration) *RateLimiter {
	return &RateLimiter{
		limiters: make(map[string]*rate.Limiter),
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

// SetDomainDelay sets a custom delay for a specific domain
func (r *RateLimiter) SetDomainDelay(domain string, delay time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if delay <= 0 {
		delay = r.delay
	}

	limit := rate.Every(delay)
	r.limiters[domain] = rate.NewLimiter(limit, 1)
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

	return limiter
}
