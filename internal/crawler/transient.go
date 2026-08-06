package crawler

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// A received HTTP response is an observation, not necessarily a finished crawl.
// This file draws the line between the two: which status codes mean "ask
// again later", and how long to wait before doing so.

// transientStatuses are the responses worth asking again for. Each one is the
// server saying it could not answer *right now* — overloaded, rate limiting,
// a gateway hiccup, a request that took too long — rather than answering a
// question about the resource.
//
// Everything else, 403 and 404 and 410 included, is a real answer about the
// resource and stays a completed observation. Retrying those would reproduce
// the same result and would turn a legitimate 404 into an unavailable row,
// destroying exactly the deletion signal a snapshot comparison relies on.
var transientStatuses = map[int]bool{
	http.StatusRequestTimeout:      true, // 408
	http.StatusTooManyRequests:     true, // 429
	http.StatusInternalServerError: true, // 500
	http.StatusBadGateway:          true, // 502
	http.StatusServiceUnavailable:  true, // 503
	http.StatusGatewayTimeout:      true, // 504
}

// isTransientStatus reports whether a status code should be retried rather
// than recorded as the final word on a URL.
func isTransientStatus(statusCode int) bool {
	return transientStatuses[statusCode]
}

// transientErrorType names a transient response for storage. The prefix is
// what storage.retryableErrorTypes matches on, and keeping the code in the
// name means a run summary can tell a rate limit apart from a bad gateway
// without a second column.
func transientErrorType(statusCode int) string {
	return fmt.Sprintf("http_%d", statusCode)
}

// networkErrorType is the transport failure that produced no response at all —
// DNS, timeout, connection reset. Like a transient HTTP status it is worth
// asking again about, and for the same reason it must be paced: retrying a
// timing-out host immediately just spends the budget in milliseconds.
const networkErrorType = "network_error"

// RetryableErrorTypes lists every last_error_type a retry should pick up, in a
// stable order.
//
// It is exported because the storage layer selects rows by these same strings
// and must not keep its own copy: a hand-written SQL list that gained a status
// this function does not know about would be requeued without ever being
// paced, and the crawler would hammer a struggling host as fast as it could
// claim the row. Deriving the list from one place makes that drift impossible
// rather than merely detectable. (MaxRetries is exported for the same reason.)
func RetryableErrorTypes() []string {
	types := make([]string, 0, len(transientStatuses)+1)
	types = append(types, networkErrorType)
	for _, status := range sortedTransientStatuses() {
		types = append(types, transientErrorType(status))
	}
	return types
}

// sortedTransientStatuses returns the transient codes in ascending order, so
// callers that build strings from them get a stable result rather than Go's
// randomised map order.
func sortedTransientStatuses() []int {
	codes := make([]int, 0, len(transientStatuses))
	for code := range transientStatuses {
		codes = append(codes, code)
	}
	sort.Ints(codes)
	return codes
}

// isRetryableErrorType reports whether a failure should be attempted again.
//
// This is the write side of the decision RetryableErrorTypes drives on the read
// side: one decides what to record on a failing row, the other selects rows to
// retry. Both come from transientStatuses, so they cannot disagree.
func isRetryableErrorType(errorType string) bool {
	if errorType == networkErrorType {
		return true
	}
	code, ok := strings.CutPrefix(errorType, "http_")
	if !ok {
		return false
	}
	status, err := strconv.Atoi(code)
	return err == nil && isTransientStatus(status)
}

// Retry pacing.
//
// Two rules, in order: honour what the server asked for, and when it asked for
// nothing, back off on our own terms. Both are capped, and neither uses
// randomness — a jittered delay would make two runs over an unchanged site
// take different paths, which is the one thing the snapshot workflow cannot
// tolerate.
const (
	// maxRetryAfter bounds how long a server may park us. A Retry-After of
	// 86400, whether hostile or mistyped, would otherwise stall the crawl for a
	// day. Matches the existing robots.txt crawl-delay cap for the same reason.
	maxRetryAfter = 60 * time.Second

	// defaultRetryBackoff is the wait after a transient failure that carried no
	// Retry-After. It doubles per attempt, capped at maxRetryAfter.
	defaultRetryBackoff = 1 * time.Second
)

// retryDelay decides how long to wait before attempting a URL again.
//
// attempt is the number of attempts already made, so the first failure asks
// for retryDelay(1). headers may be nil, and its keys are lowercased by the
// page processor.
func retryDelay(headers map[string]string, attempt int, base time.Duration, now time.Time) time.Duration {
	if d, ok := parseRetryAfter(headers, now); ok {
		return d
	}
	return backoffDelay(attempt, base)
}

// backoffDelay is the fallback pacing: base, then double per attempt, capped.
// A pure function of the attempt number, deliberately.
func backoffDelay(attempt int, base time.Duration) time.Duration {
	if base <= 0 {
		base = defaultRetryBackoff
	}
	if attempt < 1 {
		attempt = 1
	}

	d := base
	for i := 1; i < attempt; i++ {
		d *= 2
		if d >= maxRetryAfter {
			return maxRetryAfter
		}
	}
	if d > maxRetryAfter {
		return maxRetryAfter
	}
	return d
}

// parseRetryAfter reads the Retry-After header in both forms RFC 9110 allows:
// a delay in seconds, or an HTTP date. The result is clamped to
// [0, maxRetryAfter]; a date already in the past means "now", not a negative
// wait.
func parseRetryAfter(headers map[string]string, now time.Time) (time.Duration, bool) {
	if headers == nil {
		return 0, false
	}
	raw := strings.TrimSpace(headers["retry-after"])
	if raw == "" {
		return 0, false
	}

	if secs, err := strconv.Atoi(raw); err == nil {
		return clampRetryAfter(time.Duration(secs) * time.Second), true
	}

	if t, err := http.ParseTime(raw); err == nil {
		return clampRetryAfter(t.Sub(now)), true
	}

	// Unparseable. Fall back to our own backoff rather than treating a
	// malformed header as "retry immediately".
	return 0, false
}

func clampRetryAfter(d time.Duration) time.Duration {
	if d < 0 {
		return 0
	}
	if d > maxRetryAfter {
		return maxRetryAfter
	}
	return d
}
