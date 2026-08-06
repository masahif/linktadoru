package crawler

import (
	"net/http"
	"testing"
	"time"
)

func TestIsTransientStatus(t *testing.T) {
	transient := []int{
		http.StatusRequestTimeout,      // 408
		http.StatusTooManyRequests,     // 429
		http.StatusInternalServerError, // 500
		http.StatusBadGateway,          // 502
		http.StatusServiceUnavailable,  // 503
		http.StatusGatewayTimeout,      // 504
	}
	for _, code := range transient {
		if !isTransientStatus(code) {
			t.Errorf("status %d: want transient", code)
		}
	}

	// Permanent observations. A 404 in particular must stay completed: it is the
	// deletion signal a snapshot comparison depends on, and retrying it would
	// turn it into an unavailable row.
	permanent := []int{
		http.StatusOK,
		http.StatusMovedPermanently,
		http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusNotFound,
		http.StatusGone,
		http.StatusNotImplemented,          // 501 — a real answer, not a hiccup
		http.StatusHTTPVersionNotSupported, // 505
	}
	for _, code := range permanent {
		if isTransientStatus(code) {
			t.Errorf("status %d: want permanent", code)
		}
	}
}

func TestTransientErrorTypeMatchesStorageVocabulary(t *testing.T) {
	if got := transientErrorType(503); got != "http_503" {
		t.Errorf("transientErrorType(503) = %q, want http_503", got)
	}
}

func TestRetryDelayHonoursRetryAfterSeconds(t *testing.T) {
	headers := map[string]string{"retry-after": "5"}
	if got := retryDelay(headers, 1, time.Millisecond, time.Now()); got != 5*time.Second {
		t.Errorf("delay = %v, want 5s", got)
	}
}

func TestRetryDelayHonoursRetryAfterDate(t *testing.T) {
	now := time.Now().UTC()
	headers := map[string]string{"retry-after": now.Add(4 * time.Second).Format(http.TimeFormat)}

	got := retryDelay(headers, 1, time.Millisecond, now)
	// HTTP-date has second granularity, so allow the rounding.
	if got < 3*time.Second || got > 5*time.Second {
		t.Errorf("delay = %v, want about 4s", got)
	}
}

// A hostile or mistyped Retry-After must not be able to park the crawl.
func TestRetryDelayCapsOversizedRetryAfter(t *testing.T) {
	for _, raw := range []string{"86400", "99999999"} {
		headers := map[string]string{"retry-after": raw}
		if got := retryDelay(headers, 1, time.Millisecond, time.Now()); got != maxRetryAfter {
			t.Errorf("Retry-After %q: delay = %v, want the %v cap", raw, got, maxRetryAfter)
		}
	}

	// A date far in the future is capped the same way.
	now := time.Now().UTC()
	headers := map[string]string{"retry-after": now.Add(24 * time.Hour).Format(http.TimeFormat)}
	if got := retryDelay(headers, 1, time.Millisecond, now); got != maxRetryAfter {
		t.Errorf("far-future Retry-After: delay = %v, want the %v cap", got, maxRetryAfter)
	}
}

// A date already past means "now", never a negative wait.
func TestRetryDelayClampsPastRetryAfter(t *testing.T) {
	now := time.Now().UTC()
	headers := map[string]string{"retry-after": now.Add(-time.Hour).Format(http.TimeFormat)}
	if got := retryDelay(headers, 1, time.Millisecond, now); got != 0 {
		t.Errorf("past Retry-After: delay = %v, want 0", got)
	}
}

// Without a usable header the crawler paces itself instead of spending every
// attempt at once.
func TestRetryDelayFallsBackToBackoff(t *testing.T) {
	base := 100 * time.Millisecond
	cases := []struct {
		headers map[string]string
		name    string
	}{
		{nil, "no headers"},
		{map[string]string{}, "no Retry-After"},
		{map[string]string{"retry-after": "   "}, "blank Retry-After"},
		{map[string]string{"retry-after": "soon"}, "unparseable Retry-After"},
	}
	for _, tc := range cases {
		if got := retryDelay(tc.headers, 1, base, time.Now()); got != base {
			t.Errorf("%s: delay = %v, want the %v backoff base", tc.name, got, base)
		}
	}
}

func TestBackoffDelayDoublesAndCaps(t *testing.T) {
	base := time.Second
	want := []time.Duration{time.Second, 2 * time.Second, 4 * time.Second}
	for i, w := range want {
		if got := backoffDelay(i+1, base); got != w {
			t.Errorf("backoffDelay(%d) = %v, want %v", i+1, got, w)
		}
	}

	if got := backoffDelay(20, base); got != maxRetryAfter {
		t.Errorf("backoffDelay(20) = %v, want the %v cap", got, maxRetryAfter)
	}
}

// The backoff must be a pure function of the attempt number: a jittered delay
// would make two runs over an unchanged site take different paths, which is
// exactly what the snapshot comparison cannot tolerate.
func TestBackoffDelayIsDeterministic(t *testing.T) {
	for attempt := 1; attempt <= 5; attempt++ {
		first := backoffDelay(attempt, time.Second)
		for i := 0; i < 10; i++ {
			if got := backoffDelay(attempt, time.Second); got != first {
				t.Fatalf("backoffDelay(%d) returned %v then %v", attempt, first, got)
			}
		}
	}
}
