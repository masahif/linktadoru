package crawler

import (
	"context"
	"testing"
	"time"
)

func TestRateLimiter(t *testing.T) {
	limiter := NewRateLimiter(100 * time.Millisecond)
	ctx := context.Background()

	// Test rate limiting for same domain
	start := time.Now()
	
	// First request should be immediate
	err := limiter.Wait(ctx, "https://example.com/page1")
	if err != nil {
		t.Errorf("First request failed: %v", err)
	}
	
	// Second request should wait
	err = limiter.Wait(ctx, "https://example.com/page2")
	if err != nil {
		t.Errorf("Second request failed: %v", err)
	}
	
	elapsed := time.Since(start)
	if elapsed < 100*time.Millisecond {
		t.Errorf("Rate limiting not working, elapsed time: %v", elapsed)
	}

	// Different domain should not be rate limited
	start2 := time.Now()
	err = limiter.Wait(ctx, "https://other.com/page1")
	if err != nil {
		t.Errorf("Different domain request failed: %v", err)
	}
	elapsed2 := time.Since(start2)
	if elapsed2 > 10*time.Millisecond {
		t.Errorf("Different domain was rate limited, elapsed time: %v", elapsed2)
	}
}

func TestRateLimiterCustomDelay(t *testing.T) {
	limiter := NewRateLimiter(100 * time.Millisecond)
	ctx := context.Background()

	// Set custom delay for specific domain
	limiter.SetDomainDelay("example.com", 200*time.Millisecond)

	start := time.Now()
	
	// First request
	err := limiter.Wait(ctx, "https://example.com/page1")
	if err != nil {
		t.Errorf("First request failed: %v", err)
	}
	
	// Second request should wait 200ms
	err = limiter.Wait(ctx, "https://example.com/page2")
	if err != nil {
		t.Errorf("Second request failed: %v", err)
	}
	
	elapsed := time.Since(start)
	if elapsed < 200*time.Millisecond {
		t.Errorf("Custom delay not working, elapsed time: %v", elapsed)
	}
}