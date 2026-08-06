package crawler_test

// End-to-end coverage for issue #71: a transient HTTP response must be kept as
// an observation, retried, and — when it never recovers — reported as unknown
// rather than as an absent page.

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/masahif/linktadoru/internal/config"
	"github.com/masahif/linktadoru/internal/crawler"
	"github.com/masahif/linktadoru/internal/storage"

	_ "modernc.org/sqlite"
)

// transientCfg is the shared configuration for these tests. Concurrency 1 keeps
// the request sequence readable; the crawl is unbounded unless a test sets
// MaxDepth.
func transientCfg() *config.CrawlConfig {
	return &config.CrawlConfig{
		Concurrency:     1,
		RequestDelay:    0.001,
		RequestTimeout:  5 * time.Second,
		UserAgent:       "LinkTadoru-Test/1.0",
		IgnoreRobotsTxt: true,
		AllowedSchemes:  []string{"http://", "https://"},
	}
}

// runTransientCrawl runs a crawl to completion against handler and returns a
// connection to the resulting database.
func runTransientCrawl(t *testing.T, cfg *config.CrawlConfig, handler http.Handler, seedPath string) (*sql.DB, string) {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	seed := server.URL + seedPath
	cfg.SeedURLs = []string{seed}

	dbPath := filepath.Join(t.TempDir(), "transient.db")
	store, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}

	c, err := crawler.NewCrawler(cfg, store)
	if err != nil {
		_ = store.Close()
		t.Fatalf("NewCrawler: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- c.Start(context.Background(), cfg.SeedURLs) }()
	select {
	case err := <-done:
		if err != nil {
			_ = store.Close()
			t.Fatalf("Start: %v", err)
		}
	case <-time.After(60 * time.Second):
		_ = c.Stop()
		_ = store.Close()
		t.Fatal("crawler did not terminate")
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close storage: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open assertion connection: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	return db, server.URL
}

type pageRow struct {
	status     string
	statusCode sql.NullInt64
	retryCount int
	headers    sql.NullString
}

func readPage(t *testing.T, db *sql.DB, url string) pageRow {
	t.Helper()
	var row pageRow
	err := db.QueryRow(
		"SELECT status, status_code, retry_count, response_http_headers FROM pages WHERE url = ?", url,
	).Scan(&row.status, &row.statusCode, &row.retryCount, &row.headers)
	if err != nil {
		t.Fatalf("select %s: %v", url, err)
	}
	return row
}

// A transient response that later succeeds must be parsed, and the links it
// contains must be queued. Without the retry, the page's whole subtree is
// missing from the crawl — the failure mode issue #71 exists to fix.
func TestTransientSeedRecoversAndQueuesItsLinks(t *testing.T) {
	var seedHits atomic.Int32

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if seedHits.Add(1) == 1 {
			// Retry-After: 0 keeps the test quick and exercises the header path.
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<html><body><a href="/child">child</a></body></html>`)
	})
	mux.HandleFunc("/child", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<html><body>child</body></html>`)
	})

	db, base := runTransientCrawl(t, transientCfg(), mux, "/")

	seed := readPage(t, db, base+"/")
	if seed.status != "completed" {
		t.Errorf("seed status = %q, want completed after recovery", seed.status)
	}
	if !seed.statusCode.Valid || seed.statusCode.Int64 != http.StatusOK {
		t.Errorf("seed status_code = %v, want 200", seed.statusCode)
	}

	child := readPage(t, db, base+"/child")
	if child.status != "completed" {
		t.Errorf("child status = %q, want completed — the subtree behind a transient response must not be lost", child.status)
	}
}

// The observation has to survive the decision to retry: status code, headers
// and timing are written even while the row stays retryable. Otherwise the
// evidence for retrying (the status, the Retry-After) is destroyed by the act
// of retrying.
func TestTransientResponsePreservesObservation(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "0")
		w.Header().Set("Server", "test-server")
		w.WriteHeader(http.StatusTooManyRequests)
	})

	db, base := runTransientCrawl(t, transientCfg(), handler, "/")

	row := readPage(t, db, base+"/")
	if row.status != "error" {
		t.Errorf("status = %q, want error", row.status)
	}
	if !row.statusCode.Valid || row.statusCode.Int64 != http.StatusTooManyRequests {
		t.Errorf("status_code = %v, want 429 recorded alongside the error", row.statusCode)
	}
	if !row.headers.Valid || row.headers.String == "" {
		t.Error("response_http_headers is empty; the observed headers were discarded")
	}

	// The generated column proves the headers were stored as usable JSON.
	var server sql.NullString
	if err := db.QueryRow("SELECT server FROM pages WHERE url = ?", base+"/").Scan(&server); err != nil {
		t.Fatalf("select generated server column: %v", err)
	}
	if !server.Valid || server.String != "test-server" {
		t.Errorf("server = %v, want test-server", server)
	}
}

// A URL that never recovers must stop after the full budget rather than looping
// forever, and must leave a row that reads as unknown, not as absent.
func TestPermanentlyTransientURLTerminatesAfterBudget(t *testing.T) {
	var hits atomic.Int32

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusServiceUnavailable)
	})

	db, base := runTransientCrawl(t, transientCfg(), handler, "/")

	if got := hits.Load(); got != int32(crawler.MaxRetries) {
		t.Errorf("server saw %d requests, want %d (the whole per-URL budget, each attempt made once)", got, crawler.MaxRetries)
	}

	row := readPage(t, db, base+"/")
	if row.status != "error" {
		t.Errorf("status = %q, want error", row.status)
	}
	if row.retryCount != crawler.MaxRetries {
		t.Errorf("retry_count = %d, want %d", row.retryCount, crawler.MaxRetries)
	}
	// error + a status code is what "the server answered, transiently, and we
	// gave up" looks like, as opposed to error + NULL for never reaching it.
	if !row.statusCode.Valid {
		t.Error("status_code is NULL; an exhausted transient response must stay distinguishable from an unreachable one")
	}

	// One crawl_errors row per attempt, numbered.
	var attempts int
	if err := db.QueryRow(
		"SELECT COUNT(*) FROM crawl_errors WHERE url = ? AND status_code = 503", base+"/",
	).Scan(&attempts); err != nil {
		t.Fatalf("count crawl_errors: %v", err)
	}
	if attempts != crawler.MaxRetries {
		t.Errorf("crawl_errors rows = %d, want %d (one per attempt)", attempts, crawler.MaxRetries)
	}

	var maxAttempt sql.NullInt64
	if err := db.QueryRow(
		"SELECT MAX(attempt) FROM crawl_errors WHERE url = ?", base+"/",
	).Scan(&maxAttempt); err != nil {
		t.Fatalf("select max attempt: %v", err)
	}
	if !maxAttempt.Valid || maxAttempt.Int64 != int64(crawler.MaxRetries) {
		t.Errorf("max attempt = %v, want %d", maxAttempt, crawler.MaxRetries)
	}
}

// 403, 404 and 410 are answers about the resource, not hiccups. They stay
// completed and inspectable, and they are never retried — a 404 turned into an
// unavailable row would destroy the deletion signal a snapshot comparison
// depends on.
func TestPermanentStatusesAreNotRetried(t *testing.T) {
	for _, code := range []int{http.StatusForbidden, http.StatusNotFound, http.StatusGone} {
		t.Run(http.StatusText(code), func(t *testing.T) {
			var hits atomic.Int32
			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits.Add(1)
				w.WriteHeader(code)
			})

			db, base := runTransientCrawl(t, transientCfg(), handler, "/")

			if got := hits.Load(); got != 1 {
				t.Errorf("server saw %d requests, want 1 — %d must not be retried", got, code)
			}

			row := readPage(t, db, base+"/")
			if row.status != "completed" {
				t.Errorf("status = %q, want completed", row.status)
			}
			if !row.statusCode.Valid || row.statusCode.Int64 != int64(code) {
				t.Errorf("status_code = %v, want %d and inspectable", row.statusCode, code)
			}
			if row.retryCount != 0 {
				t.Errorf("retry_count = %d, want 0", row.retryCount)
			}
		})
	}
}

// A transient response with no Retry-After must pace itself rather than
// spending every attempt at once. The elapsed time proves the backoff ran.
func TestTransientWithoutRetryAfterBacksOff(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway) // no Retry-After
	})

	start := time.Now()
	db, base := runTransientCrawl(t, transientCfg(), handler, "/")
	elapsed := time.Since(start)

	// Two waits happen between three attempts: base, then double.
	const minExpected = 2500 * time.Millisecond
	if elapsed < minExpected {
		t.Errorf("crawl took %v, want at least %v — the attempts were not spaced", elapsed, minExpected)
	}

	row := readPage(t, db, base+"/")
	if row.retryCount != crawler.MaxRetries {
		t.Errorf("retry_count = %d, want %d", row.retryCount, crawler.MaxRetries)
	}
}

// A retry is a fresh trip through the worker path, so the host allow-list has
// to be re-applied to its redirects. A redirect to another host must be
// refused on the retry exactly as it is on the first attempt.
func TestRetryReappliesRedirectHostPolicy(t *testing.T) {
	var foreignHits atomic.Int32
	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		foreignHits.Add(1)
		_, _ = fmt.Fprint(w, "should never be fetched")
	}))
	defer foreign.Close()

	var seedHits atomic.Int32
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if seedHits.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		// Every later attempt tries to walk us onto a host the crawl was never
		// allowed to reach.
		http.Redirect(w, r, foreign.URL+"/", http.StatusFound)
	})

	db, base := runTransientCrawl(t, transientCfg(), handler, "/")

	if got := foreignHits.Load(); got != 0 {
		t.Errorf("foreign host received %d requests on retry, want 0", got)
	}
	if seedHits.Load() < 2 {
		t.Fatal("the retry never happened, so the policy was not exercised")
	}

	row := readPage(t, db, base+"/")
	if row.status == "completed" {
		t.Error("a blocked cross-host redirect must not land as a completed page")
	}
}

// Bounded crawls keep retrying inside the layer: a page that only succeeds on
// its second attempt still has children, and those children belong to the very
// next layer.
func TestTransientRetryRunsInsideTheLayer(t *testing.T) {
	var seedHits atomic.Int32
	var childSeen atomic.Bool

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if seedHits.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<html><body><a href="/child">child</a></body></html>`)
	})
	mux.HandleFunc("/child", func(w http.ResponseWriter, r *http.Request) {
		childSeen.Store(true)
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<html><body><a href="/grandchild">gc</a></body></html>`)
	})
	mux.HandleFunc("/grandchild", func(w http.ResponseWriter, r *http.Request) {
		t.Error("grandchild fetched: max_depth=1 must not reach depth 2")
	})

	cfg := transientCfg()
	cfg.MaxDepth = 1

	db, base := runTransientCrawl(t, cfg, mux, "/")

	if !childSeen.Load() {
		t.Error("the recovered seed's child was never crawled; the layer moved on without it")
	}

	child := readPage(t, db, base+"/child")
	if child.status != "completed" {
		t.Errorf("child status = %q, want completed", child.status)
	}

	var depth sql.NullInt64
	if err := db.QueryRow("SELECT depth FROM pages WHERE url = ?", base+"/child").Scan(&depth); err != nil {
		t.Fatalf("select child depth: %v", err)
	}
	if !depth.Valid || depth.Int64 != 1 {
		t.Errorf("child depth = %v, want 1", depth)
	}
}

// A transport failure produces no HTTP response, so there is no Retry-After to
// honour — but it still has to be paced. A host that times out will time out
// again immediately, so retrying without a wait burns the whole budget in
// milliseconds and never gives it a chance to recover.
func TestNetworkErrorIsPaced(t *testing.T) {
	// A server that is closed refuses connections instantly, so any elapsed
	// time is the backoff rather than network latency.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := server.URL + "/"
	server.Close()

	dbPath := filepath.Join(t.TempDir(), "paced.db")
	store, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}

	cfg := transientCfg()
	cfg.SeedURLs = []string{deadURL}
	c, err := crawler.NewCrawler(cfg, store)
	if err != nil {
		t.Fatalf("NewCrawler: %v", err)
	}

	start := time.Now()
	if err := c.Start(context.Background(), cfg.SeedURLs); err != nil {
		t.Fatalf("Start: %v", err)
	}
	elapsed := time.Since(start)
	if err := store.Close(); err != nil {
		t.Fatalf("close storage: %v", err)
	}

	// Two waits between three attempts: base, then double.
	const minExpected = 2500 * time.Millisecond
	if elapsed < minExpected {
		t.Errorf("crawl took %v, want at least %v — a transport failure was retried without pacing", elapsed, minExpected)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open assertion connection: %v", err)
	}
	defer func() { _ = db.Close() }()

	row := readPage(t, db, deadURL)
	if row.retryCount != crawler.MaxRetries {
		t.Errorf("retry_count = %d, want %d", row.retryCount, crawler.MaxRetries)
	}
	if row.statusCode.Valid {
		t.Error("status_code should be NULL: no HTTP response ever arrived")
	}
}

// The wait has to survive a resume. It lives in the database precisely because
// the retry that honours it can outlive the run that scheduled it: a run that
// is interrupted mid-backoff must not come back and hammer the host
// immediately.
func TestRetryWaitSurvivesResume(t *testing.T) {
	var hits atomic.Int32
	firstHit := make(chan time.Time, 8)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		select {
		case firstHit <- time.Now():
		default:
		}
		w.Header().Set("Retry-After", "2")
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	dbPath := filepath.Join(t.TempDir(), "resume.db")
	seed := server.URL + "/"

	// First run: one attempt, then cancel while the row is waiting out its
	// Retry-After. The wait must be on disk when the run ends.
	store, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}
	cfg := transientCfg()
	cfg.SeedURLs = []string{seed}
	c, err := crawler.NewCrawler(cfg, store)
	if err != nil {
		t.Fatalf("NewCrawler: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Start(ctx, cfg.SeedURLs) }()

	var failedAt time.Time
	select {
	case failedAt = <-firstHit:
	case <-time.After(10 * time.Second):
		cancel()
		t.Fatal("the server was never contacted")
	}
	// Let the failure be recorded, then stop the run mid-wait.
	time.Sleep(300 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("Start did not return after cancellation")
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close storage: %v", err)
	}

	if got := hits.Load(); got != 1 {
		t.Fatalf("first run made %d attempts, want 1", got)
	}

	// The wait is on disk, in the future, written by the run that just ended.
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open assertion connection: %v", err)
	}
	var retryAfter sql.NullString
	if err := db.QueryRow("SELECT retry_after FROM pages WHERE url = ?", seed).Scan(&retryAfter); err != nil {
		t.Fatalf("select retry_after: %v", err)
	}
	_ = db.Close()
	if !retryAfter.Valid {
		t.Fatal("retry_after is NULL after a transient failure; the wait was not persisted")
	}

	// Second run against the same database, resuming with no seeds. It must
	// honour the wait the first run recorded rather than retrying at once.
	store2, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("reopen storage: %v", err)
	}
	c2, err := crawler.NewCrawler(cfg, store2)
	if err != nil {
		t.Fatalf("NewCrawler (resume): %v", err)
	}
	if err := c2.Start(context.Background(), nil); err != nil {
		t.Fatalf("Start (resume): %v", err)
	}
	_ = store2.Close()

	var secondHit time.Time
	select {
	case secondHit = <-firstHit:
	case <-time.After(time.Second):
		t.Fatal("the resumed run never retried")
	}

	// The server asked for 2 seconds from the moment it refused. The resumed
	// run must not have come back before then, even though its own queue was
	// ready immediately.
	if gap := secondHit.Sub(failedAt); gap < 1500*time.Millisecond {
		t.Errorf("the resumed run retried %v after the failure, want at least the ~2s Retry-After — the wait did not survive the restart", gap)
	}
}

// Cancellation must not spend a retry credit: the row is left in 'processing'
// for the next run's cleanup rather than being charged for an attempt the run
// never got to judge.
func TestCancellationDoesNotConsumeRetryCredit(t *testing.T) {
	release := make(chan struct{})
	started := make(chan struct{})
	var once bool

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !once {
			once = true
			close(started)
		}
		<-release
		w.Header().Set("Retry-After", "0")
		w.WriteHeader(http.StatusServiceUnavailable)
	})

	server := httptest.NewServer(handler)
	defer server.Close()
	defer close(release)

	dbPath := filepath.Join(t.TempDir(), "cancel.db")
	store, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}

	cfg := transientCfg()
	cfg.SeedURLs = []string{server.URL + "/"}
	c, err := crawler.NewCrawler(cfg, store)
	if err != nil {
		t.Fatalf("NewCrawler: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Start(ctx, cfg.SeedURLs) }()

	<-started
	cancel()

	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("Start did not return after cancellation")
	}
	if err := store.Close(); err != nil {
		t.Fatalf("close storage: %v", err)
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open assertion connection: %v", err)
	}
	defer func() { _ = db.Close() }()

	var status string
	var retryCount int
	if err := db.QueryRow(
		"SELECT status, retry_count FROM pages WHERE url = ?", cfg.SeedURLs[0],
	).Scan(&status, &retryCount); err != nil {
		t.Fatalf("select seed row: %v", err)
	}
	if retryCount != 0 {
		t.Errorf("retry_count = %d, want 0 — cancellation must not charge an attempt", retryCount)
	}
	if status != "processing" {
		t.Errorf("status = %q, want processing so the next run's cleanup requeues it", status)
	}
}
