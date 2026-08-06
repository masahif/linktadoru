package crawler_test

// Integration regressions for the audit-fix review findings: the retry phase
// must actually run after a completed crawl, and cancellation must make Start
// return promptly after its workers wind down.

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/masahif/linktadoru/internal/crawler"
	"github.com/masahif/linktadoru/internal/storage"

	_ "modernc.org/sqlite"
)

// The retry phase was unreachable for the project's entire history: the last
// exiting worker cancelled the crawl context (to stop the stats reporter),
// which made Start take its "cancelled" branch instead of calling
// performRetries. A retry_count above 1 proves the phase now runs.
//
// It reaches crawler.MaxRetries because an uninterrupted run spends the whole
// per-URL budget before returning rather than making a single pass. The budget
// itself is per URL across the database (retry_count is persisted, so a
// cancelled run carries its remainder into the resume); what matters here is
// that a run left to finish does not leave attempts unmade. With a database
// created fresh per crawl there is no later run to spend them, and the
// transient failure they would have recovered from is indistinguishable from a
// page that is genuinely gone.
func TestRetryPhaseRunsAfterCrawlCompletes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := server.URL
	server.Close() // connection refused from here on

	dbPath := filepath.Join(t.TempDir(), "retry.db")
	store, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	cfg := baseCfg()
	cfg.SeedURLs = []string{deadURL}
	cfg.RequestTimeout = 2 * time.Second
	c, err := crawler.NewCrawler(cfg, store)
	if err != nil {
		t.Fatalf("NewCrawler: %v", err)
	}
	t.Cleanup(func() { _ = c.Stop() })

	done := make(chan error, 1)
	go func() { done <- c.Start(context.Background(), cfg.SeedURLs) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Start: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("crawler did not terminate")
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open assertion connection: %v", err)
	}
	defer func() { _ = db.Close() }()

	var status string
	var retryCount int
	if err := db.QueryRow(
		"SELECT status, retry_count FROM pages WHERE url = ?", deadURL,
	).Scan(&status, &retryCount); err != nil {
		t.Fatalf("select seed row: %v", err)
	}
	if status != "error" {
		t.Errorf("status = %q, want error", status)
	}
	if retryCount != crawler.MaxRetries {
		t.Errorf("retry_count = %d, want %d (an uninterrupted run spends the whole per-URL budget)",
			retryCount, crawler.MaxRetries)
	}
}

// On cancellation Start must wait for its workers (graceful shutdown) but then
// return promptly — the in-flight HTTP request carries the crawl context, so a
// worker stuck on a slow server unblocks as soon as the context is cancelled.
func TestStartReturnsPromptlyOnCancel(t *testing.T) {
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release // hold every request until the test ends
	}))
	defer server.Close()
	defer close(release)

	cfg := baseCfg()
	cfg.SeedURLs = []string{server.URL}
	cfg.RequestTimeout = 60 * time.Second // only cancellation can unblock
	store := newStore(t)
	c, err := crawler.NewCrawler(cfg, store)
	if err != nil {
		t.Fatalf("NewCrawler: %v", err)
	}
	t.Cleanup(func() { _ = c.Stop() })

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Start(ctx, cfg.SeedURLs) }()

	time.AfterFunc(200*time.Millisecond, cancel)

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Start: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Start did not return after context cancellation")
	}
}
