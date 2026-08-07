package storage

import (
	"strings"
	"testing"
	"time"

	"github.com/masahif/linktadoru/internal/crawler"
)

// The retryable set must match the error types the crawler actually writes.
// Before this test existed, the SQL filtered on types nothing ever wrote
// ('network_timeout', ...), so the retry mechanism silently never fired.
func TestRetryEligibilityMatchesWrittenErrorTypes(t *testing.T) {
	store := newTempStorage(t)

	// One errored page per error type the crawler can write (see
	// page_processor.go and crawler.go SavePageError call sites).
	errorTypes := map[string]string{
		"https://example.com/net":  "network_error",
		"https://example.com/http": "http_transient",
		"https://example.com/proc": "processing_error",
		"https://example.com/rate": "rate_limit_error",
		"https://example.com/big":  "response_too_large",
	}
	for u, et := range errorTypes {
		if err := store.AddToQueue([]string{u}, 0); err != nil {
			t.Fatalf("AddToQueue(%s): %v", u, err)
		}
		item, err := store.GetNextFromQueue()
		if err != nil || item == nil {
			t.Fatalf("GetNextFromQueue(%s): item=%v err=%v", u, item, err)
		}
		if err := store.SavePageError(item.ID, et, "boom"); err != nil {
			t.Fatalf("SavePageError(%s): %v", u, err)
		}
	}

	// Transport and selected HTTP failures are eligible.
	items, err := store.GetRetryablePages(3)
	if err != nil {
		t.Fatalf("GetRetryablePages: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("retryable = %+v, want network and transient HTTP pages", items)
	}

	requeued, err := store.RequeueErrorPages(3)
	if err != nil {
		t.Fatalf("RequeueErrorPages: %v", err)
	}
	if requeued != 2 {
		t.Errorf("requeued = %d, want 2", requeued)
	}
	for _, u := range []string{"https://example.com/net", "https://example.com/http"} {
		if got := mustStatus(t, store, u); got != "pending" {
			t.Errorf("%s status = %q, want pending (requeued)", u, got)
		}
	}
	for _, u := range []string{"https://example.com/proc", "https://example.com/rate", "https://example.com/big"} {
		if got := mustStatus(t, store, u); got != "error" {
			t.Errorf("%s status = %q, want error (deterministic failures must not requeue)", u, got)
		}
	}
}

func TestSavePageResponseErrorIsAtomicAndInspectable(t *testing.T) {
	store := newTempStorage(t)
	const pageURL = "https://example.com/unavailable"
	if err := store.AddToQueue([]string{pageURL}, 0); err != nil {
		t.Fatal(err)
	}
	item, err := store.GetNextFromQueue()
	if err != nil || item == nil {
		t.Fatalf("GetNextFromQueue: item=%v err=%v", item, err)
	}
	page := &crawler.PageData{
		URL:          pageURL,
		StatusCode:   503,
		TTFB:         12 * time.Millisecond,
		DownloadTime: 34 * time.Millisecond,
		ResponseSize: 9,
		HTTPHeaders:  map[string]string{"x-test": "preserved"},
		CrawledAt:    time.Now().UTC(),
	}
	if err := store.SavePageResponseError(item.ID, page, "http_transient", "HTTP 503"); err != nil {
		t.Fatal(err)
	}

	var status, headers, errorType string
	var statusCode, retryCount, ttfbMS, downloadMS, responseSize int
	if err := store.db.QueryRow(`
		SELECT status, status_code, response_http_headers, retry_count,
		       last_error_type, ttfb_ms, download_time_ms, response_size_bytes
		FROM pages WHERE id = ?
	`, item.ID).Scan(
		&status, &statusCode, &headers, &retryCount, &errorType,
		&ttfbMS, &downloadMS, &responseSize,
	); err != nil {
		t.Fatal(err)
	}
	if status != "error" || statusCode != 503 || retryCount != 1 || errorType != "http_transient" {
		t.Fatalf("stored state = %q %d %d %q", status, statusCode, retryCount, errorType)
	}
	if !strings.Contains(headers, "preserved") {
		t.Fatalf("headers = %q", headers)
	}
	if ttfbMS != 12 || downloadMS != 34 || responseSize != 9 {
		t.Fatalf("timing/size = %d/%d/%d, want 12/34/9", ttfbMS, downloadMS, responseSize)
	}
}

// Rows written by older releases carry Go's local-timezone time.String() form.
// A local date can be AHEAD of the current UTC date (e.g. JST just after local
// midnight), making the legacy string compare greater than a UTC cutoff — so a
// plain `< cutoff` would skip it and the resume would hang on the stuck
// 'processing' row. Any non-current-format timestamp must be treated as stale.
func TestCleanupStaleProcessingResetsLegacyFormatRows(t *testing.T) {
	store := newTempStorage(t)

	// Far-future local-format value: maximally adversarial for `< cutoff`.
	legacy := "2030-01-01 09:00:00.5 +0900 JST"
	if _, err := store.db.Exec(
		"INSERT INTO pages (url, status, processing_started_at) VALUES ('https://example.com/legacy-ts', 'processing', ?)",
		legacy,
	); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}

	if err := store.CleanupStaleProcessing(0); err != nil {
		t.Fatalf("CleanupStaleProcessing: %v", err)
	}
	if got := mustStatus(t, store, "https://example.com/legacy-ts"); got != "pending" {
		t.Errorf("legacy-format processing row status = %q, want pending (reset)", got)
	}
}

// Timestamps must be stored in the fixed-width UTC layout so string comparison
// in SQL matches chronological order regardless of local timezone/DST.
func TestTimestampsStoredInFixedWidthUTC(t *testing.T) {
	store := newTempStorage(t)
	if err := store.AddToQueue([]string{"https://example.com/ts"}, 0); err != nil {
		t.Fatalf("AddToQueue: %v", err)
	}

	// CAST(...) makes the expression lose the column's DATETIME declared type,
	// so the driver returns the raw stored TEXT instead of parsing it into a
	// time.Time and re-formatting (which would trim trailing zeros).
	var added string
	if err := store.db.QueryRow(
		"SELECT CAST(added_at AS TEXT) FROM pages WHERE url = 'https://example.com/ts'",
	).Scan(&added); err != nil {
		t.Fatalf("select added_at: %v", err)
	}

	parsed, err := time.Parse(sqlTimeFormat, added)
	if err != nil {
		t.Fatalf("added_at %q does not match sqlTimeFormat: %v", added, err)
	}
	if d := time.Since(parsed); d < 0 || d > time.Minute {
		t.Errorf("added_at %q not near now (delta %v)", added, d)
	}
	// Fixed width is what keeps lexicographic order == chronological order.
	if len(added) != len(sqlTimeFormat) {
		t.Errorf("added_at %q is not fixed-width (len %d, want %d)", added, len(added), len(sqlTimeFormat))
	}
}
