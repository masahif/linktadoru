package storage

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/masahif/linktadoru/internal/crawler"
)

func newTransientTestStorage(t *testing.T) *SQLiteStorage {
	t.Helper()
	s, err := NewSQLiteStorage(filepath.Join(t.TempDir(), "transient.db"))
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func queueOne(t *testing.T, s *SQLiteStorage, url string) *crawler.URLItem {
	t.Helper()
	if err := s.AddToQueue([]string{url}); err != nil {
		t.Fatalf("AddToQueue: %v", err)
	}
	item, err := s.GetNextFromQueue()
	if err != nil {
		t.Fatalf("GetNextFromQueue: %v", err)
	}
	if item == nil {
		t.Fatal("GetNextFromQueue returned nothing")
	}
	return item
}

func transientPage(status int) *crawler.PageData {
	return &crawler.PageData{
		StatusCode:   status,
		TTFB:         12 * time.Millisecond,
		DownloadTime: 34 * time.Millisecond,
		ResponseSize: 56,
		HTTPHeaders:  map[string]string{"retry-after": "0", "server": "test"},
		CrawledAt:    time.Now().UTC(),
	}
}

// The claim has to carry the attempt count, otherwise a failing attempt cannot
// record which attempt it was without a second round trip.
func TestClaimReturnsRetryCount(t *testing.T) {
	s := newTransientTestStorage(t)

	item := queueOne(t, s, "https://example.com/a")
	if item.RetryCount != 0 {
		t.Errorf("first claim RetryCount = %d, want 0", item.RetryCount)
	}

	if err := s.SaveFailedAttempt(item.ID, transientPage(503), "http_503", "unavailable", time.Time{}); err != nil {
		t.Fatalf("SaveFailedAttempt: %v", err)
	}
	if _, err := s.RequeueErrorPages(3); err != nil {
		t.Fatalf("RequeueErrorPages: %v", err)
	}

	again, err := s.GetNextFromQueue()
	if err != nil || again == nil {
		t.Fatalf("second claim: item=%v err=%v", again, err)
	}
	if again.RetryCount != 1 {
		t.Errorf("second claim RetryCount = %d, want 1", again.RetryCount)
	}
}

// SaveFailedAttempt must write the observation and still leave the row
// retryable — and it must advance retry_count, which is the only thing that
// eventually stops a permanently failing URL.
func TestSaveFailedAttemptKeepsObservationAndAdvancesCount(t *testing.T) {
	s := newTransientTestStorage(t)
	item := queueOne(t, s, "https://example.com/a")

	if err := s.SaveFailedAttempt(item.ID, transientPage(429), "http_429", "rate limited", time.Time{}); err != nil {
		t.Fatalf("SaveFailedAttempt: %v", err)
	}

	var (
		status     string
		statusCode sql.NullInt64
		ttfb       sql.NullInt64
		size       sql.NullInt64
		retryCount int
		server     sql.NullString
	)
	err := s.db.QueryRow(`
		SELECT status, status_code, ttfb_ms, response_size_bytes, retry_count, server
		FROM pages WHERE id = ?`, item.ID,
	).Scan(&status, &statusCode, &ttfb, &size, &retryCount, &server)
	if err != nil {
		t.Fatalf("select row: %v", err)
	}

	if status != "error" {
		t.Errorf("status = %q, want error", status)
	}
	if !statusCode.Valid || statusCode.Int64 != 429 {
		t.Errorf("status_code = %v, want 429", statusCode)
	}
	if !ttfb.Valid || ttfb.Int64 != 12 {
		t.Errorf("ttfb_ms = %v, want 12", ttfb)
	}
	if !size.Valid || size.Int64 != 56 {
		t.Errorf("response_size_bytes = %v, want 56", size)
	}
	if retryCount != 1 {
		t.Errorf("retry_count = %d, want 1", retryCount)
	}
	if !server.Valid || server.String != "test" {
		t.Errorf("server = %v, want test (headers were not stored as JSON)", server)
	}
}

// A row waiting out its Retry-After is retryable but not yet due. The requeue
// must leave it alone, and EarliestRetryTime must report when it becomes due —
// these two disagreeing on timing is normal, not a fault.
func TestRequeueRespectsRetryAfter(t *testing.T) {
	s := newTransientTestStorage(t)
	item := queueOne(t, s, "https://example.com/a")

	future := time.Now().Add(time.Hour)
	if err := s.SaveFailedAttempt(item.ID, transientPage(503), "http_503", "unavailable", future); err != nil {
		t.Fatalf("SaveFailedAttempt: %v", err)
	}

	requeued, err := s.RequeueErrorPages(3)
	if err != nil {
		t.Fatalf("RequeueErrorPages: %v", err)
	}
	if requeued != 0 {
		t.Errorf("requeued %d rows, want 0 while the row is still waiting", requeued)
	}

	due, err := s.EarliestRetryTime(3)
	if err != nil {
		t.Fatalf("EarliestRetryTime: %v", err)
	}
	if due == nil {
		t.Fatal("EarliestRetryTime = nil, want the future due time")
	}
	if due.Before(time.Now().Add(30 * time.Minute)) {
		t.Errorf("due = %v, want about an hour out", due)
	}
}

// A NULL retry_after reads as due now, so transport failures and rows written
// by older versions retry immediately rather than sitting on a pause nobody
// recorded.
func TestEarliestRetryTimeTreatsNullAsDueNow(t *testing.T) {
	s := newTransientTestStorage(t)
	item := queueOne(t, s, "https://example.com/a")

	if err := s.SaveFailedAttempt(item.ID, nil, "network_error", "connection refused", time.Time{}); err != nil {
		t.Fatalf("SaveFailedAttempt: %v", err)
	}

	due, err := s.EarliestRetryTime(3)
	if err != nil {
		t.Fatalf("EarliestRetryTime: %v", err)
	}
	if due == nil {
		t.Fatal("EarliestRetryTime = nil, want due now")
	}
	if due.After(time.Now().Add(time.Second)) {
		t.Errorf("due = %v, want now", due)
	}

	requeued, err := s.RequeueErrorPages(3)
	if err != nil {
		t.Fatalf("RequeueErrorPages: %v", err)
	}
	if requeued != 1 {
		t.Errorf("requeued %d rows, want 1", requeued)
	}
}

// One due row must win the MIN over any number of waiting ones, or the crawler
// would sit idle while there is work it could do right now.
func TestEarliestRetryTimeFavoursDueRows(t *testing.T) {
	s := newTransientTestStorage(t)

	waiting := queueOne(t, s, "https://example.com/waiting")
	if err := s.SaveFailedAttempt(waiting.ID, transientPage(503), "http_503", "later", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("SaveFailedAttempt: %v", err)
	}

	dueRow := queueOne(t, s, "https://example.com/due")
	if err := s.SaveFailedAttempt(dueRow.ID, nil, "network_error", "refused", time.Time{}); err != nil {
		t.Fatalf("SaveFailedAttempt: %v", err)
	}

	due, err := s.EarliestRetryTime(3)
	if err != nil {
		t.Fatalf("EarliestRetryTime: %v", err)
	}
	if due == nil || due.After(time.Now().Add(time.Second)) {
		t.Errorf("due = %v, want now (a due row exists)", due)
	}
}

// Exhausted rows are not retryable, so the loop that drives off this query
// terminates instead of spinning.
func TestEarliestRetryTimeIgnoresExhaustedRows(t *testing.T) {
	s := newTransientTestStorage(t)
	item := queueOne(t, s, "https://example.com/a")

	for i := 0; i < 3; i++ {
		if err := s.SaveFailedAttempt(item.ID, transientPage(503), "http_503", "unavailable", time.Time{}); err != nil {
			t.Fatalf("SaveFailedAttempt: %v", err)
		}
	}

	due, err := s.EarliestRetryTime(3)
	if err != nil {
		t.Fatalf("EarliestRetryTime: %v", err)
	}
	if due != nil {
		t.Errorf("EarliestRetryTime = %v, want nil once the budget is spent", due)
	}
}

// The SQL list is generated from the crawler's classification, so the two
// cannot drift apart. This pins the rendered result: the dangerous direction is
// the SQL list gaining a type the crawler does not pace, which would requeue
// rows with a NULL retry_after and let the crawler hammer a struggling host.
func TestRetryableErrorTypesRenderedFromCrawler(t *testing.T) {
	const want = "('network_error', 'http_408', 'http_429', 'http_500', 'http_502', 'http_503', 'http_504')"
	if retryableErrorTypes != want {
		t.Errorf("retryableErrorTypes = %s, want %s", retryableErrorTypes, want)
	}

	// Every crawler entry must be present, and the SQL list must hold nothing
	// beyond them.
	for _, errType := range crawler.RetryableErrorTypes() {
		if !strings.Contains(retryableErrorTypes, "'"+errType+"'") {
			t.Errorf("%q is retryable in the crawler but missing from the SQL list", errType)
		}
	}
	if got, want := strings.Count(retryableErrorTypes, "'")/2, len(crawler.RetryableErrorTypes()); got != want {
		t.Errorf("SQL list holds %d types, crawler reports %d", got, want)
	}
}

// Permanent HTTP observations must never enter the retry set.
func TestPermanentHTTPErrorTypesAreNotRetryable(t *testing.T) {
	s := newTransientTestStorage(t)
	item := queueOne(t, s, "https://example.com/gone")

	if err := s.SaveFailedAttempt(item.ID, transientPage(410), "http_410", "gone", time.Time{}); err != nil {
		t.Fatalf("SaveFailedAttempt: %v", err)
	}

	due, err := s.EarliestRetryTime(3)
	if err != nil {
		t.Fatalf("EarliestRetryTime: %v", err)
	}
	if due != nil {
		t.Error("http_410 is retryable; a permanent observation must not be")
	}
}

func TestSaveErrorRecordsStatusAndAttempt(t *testing.T) {
	s := newTransientTestStorage(t)

	if err := s.SaveError(&crawler.CrawlError{
		URL:        "https://example.com/a",
		ErrorType:  "http_503",
		StatusCode: 503,
		Attempt:    2,
		OccurredAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("SaveError: %v", err)
	}
	// A transport failure has no status: it must land as NULL, not as 0.
	if err := s.SaveError(&crawler.CrawlError{
		URL:        "https://example.com/b",
		ErrorType:  "network_error",
		Attempt:    1,
		OccurredAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("SaveError: %v", err)
	}

	var code sql.NullInt64
	var attempt sql.NullInt64
	if err := s.db.QueryRow(
		"SELECT status_code, attempt FROM crawl_errors WHERE url = ?", "https://example.com/a",
	).Scan(&code, &attempt); err != nil {
		t.Fatalf("select http error: %v", err)
	}
	if !code.Valid || code.Int64 != 503 || !attempt.Valid || attempt.Int64 != 2 {
		t.Errorf("status_code=%v attempt=%v, want 503 and 2", code, attempt)
	}

	if err := s.db.QueryRow(
		"SELECT status_code FROM crawl_errors WHERE url = ?", "https://example.com/b",
	).Scan(&code); err != nil {
		t.Fatalf("select transport error: %v", err)
	}
	if code.Valid {
		t.Errorf("status_code = %v, want NULL for a request that got no response", code)
	}
}

// The summary must keep "the server answered transiently and we gave up" apart
// from "we never reached it" and from a 404, or a snapshot comparison cannot
// tell an outage from a deletion.
func TestGetRunSummarySeparatesUnavailableFromUnreachable(t *testing.T) {
	s := newTransientTestStorage(t)

	completed := queueOne(t, s, "https://example.com/ok")
	if err := s.SavePageResult(completed.ID, &crawler.PageData{StatusCode: 200, HTTPHeaders: map[string]string{}, CrawledAt: time.Now().UTC()}); err != nil {
		t.Fatalf("SavePageResult: %v", err)
	}

	notFound := queueOne(t, s, "https://example.com/gone")
	if err := s.SavePageResult(notFound.ID, &crawler.PageData{StatusCode: 404, HTTPHeaders: map[string]string{}, CrawledAt: time.Now().UTC()}); err != nil {
		t.Fatalf("SavePageResult: %v", err)
	}

	unavailable := queueOne(t, s, "https://example.com/503")
	if err := s.SaveFailedAttempt(unavailable.ID, transientPage(503), "http_503", "unavailable", time.Time{}); err != nil {
		t.Fatalf("SaveFailedAttempt: %v", err)
	}

	unreachable := queueOne(t, s, "https://example.com/dns")
	if err := s.SaveFailedAttempt(unreachable.ID, nil, "network_error", "no such host", time.Time{}); err != nil {
		t.Fatalf("SaveFailedAttempt: %v", err)
	}

	skipped := queueOne(t, s, "https://example.com/robots")
	if err := s.SavePageSkipped(skipped.ID, "robots_txt_disallow", "disallowed"); err != nil {
		t.Fatalf("SavePageSkipped: %v", err)
	}

	if err := s.AddToQueue([]string{"https://example.com/queued"}); err != nil {
		t.Fatalf("AddToQueue: %v", err)
	}

	summary, err := s.GetRunSummary()
	if err != nil {
		t.Fatalf("GetRunSummary: %v", err)
	}

	// The 404 counts as completed: it is an observation, and the deletion
	// signal a comparison relies on.
	if summary.Completed != 2 {
		t.Errorf("Completed = %d, want 2 (200 and 404)", summary.Completed)
	}
	if summary.Unavailable != 1 {
		t.Errorf("Unavailable = %d, want 1", summary.Unavailable)
	}
	if summary.Unreachable != 1 {
		t.Errorf("Unreachable = %d, want 1", summary.Unreachable)
	}
	if summary.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1", summary.Skipped)
	}
	if summary.Unfinished != 1 {
		t.Errorf("Unfinished = %d, want 1", summary.Unfinished)
	}
}
