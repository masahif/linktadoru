package crawler_test

// Integration coverage for issue #46: include_patterns / exclude_patterns must
// actually prevent non-matching URLs from being crawled. These tests drive the
// real crawler against an httptest server and a real SQLite store, then assert
// the resulting page statuses. They live in an external test package to avoid
// the storage -> crawler import cycle.

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/masahif/linktadoru/internal/config"
	"github.com/masahif/linktadoru/internal/crawler"
	"github.com/masahif/linktadoru/internal/storage"
)

func newStore(t *testing.T) *storage.SQLiteStorage {
	t.Helper()
	store, err := storage.NewSQLiteStorage(filepath.Join(t.TempDir(), "issue46.db"))
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func statusOf(t *testing.T, s *storage.SQLiteStorage, url string) (string, bool) {
	t.Helper()
	return s.GetURLStatus(url)
}

// runCrawl serves a seed page that links to the given relative paths, crawls it,
// and returns the store plus the server base URL for inspection.
func runCrawl(t *testing.T, cfg *config.CrawlConfig, seedLinks []string) (*storage.SQLiteStorage, string) {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		if r.URL.Path == "/" {
			body := "<html><body>"
			for _, l := range seedLinks {
				body += `<a href="` + l + `">link</a>`
			}
			body += "</body></html>"
			_, _ = w.Write([]byte(body))
			return
		}
		// Leaf pages: valid HTML, no further links.
		_, _ = w.Write([]byte("<html><body>leaf</body></html>"))
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	cfg.SeedURLs = []string{server.URL}
	store := newStore(t)
	c, err := crawler.NewCrawler(cfg, store)
	if err != nil {
		t.Fatalf("NewCrawler: %v", err)
	}
	t.Cleanup(func() { _ = c.Stop() })

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := c.Start(ctx, cfg.SeedURLs); err != nil {
		t.Fatalf("Start: %v", err)
	}
	return store, server.URL
}

func baseCfg() *config.CrawlConfig {
	return &config.CrawlConfig{
		Limit:           10,
		Concurrency:     1,
		RequestDelay:    0.001,
		RequestTimeout:  5 * time.Second,
		UserAgent:       "LinkTadoru-Test/1.0",
		IgnoreRobotsTxt: true,
	}
}

// Test C (exclude): a URL matching exclude_patterns is recorded as a link-graph
// node ('discovered') but is never crawled.
func TestCrawlRespectsExcludePatterns(t *testing.T) {
	cfg := baseCfg()
	cfg.ExcludePatterns = []string{"/pickup/"}
	store, base := runCrawl(t, cfg, []string{"/articles/ok", "/pickup/no"})

	if got, _ := statusOf(t, store, base+"/articles/ok"); got != "completed" {
		t.Errorf("/articles/ok status = %q, want completed", got)
	}
	got, exists := statusOf(t, store, base+"/pickup/no")
	if !exists {
		t.Fatal("/pickup/no should exist as a link-graph node")
	}
	if got != "discovered" {
		t.Errorf("/pickup/no status = %q, want discovered (excluded, not crawled)", got)
	}
}

// Test C (include): when include_patterns is set, a non-matching URL is recorded
// as 'discovered' but never crawled.
func TestCrawlRespectsIncludePatterns(t *testing.T) {
	cfg := baseCfg()
	cfg.IncludePatterns = []string{"/articles/"}
	store, base := runCrawl(t, cfg, []string{"/articles/ok", "/other/no"})

	if got, _ := statusOf(t, store, base+"/articles/ok"); got != "completed" {
		t.Errorf("/articles/ok status = %q, want completed", got)
	}
	got, exists := statusOf(t, store, base+"/other/no")
	if !exists {
		t.Fatal("/other/no should exist as a link-graph node")
	}
	if got != "discovered" {
		t.Errorf("/other/no status = %q, want discovered (not in include, not crawled)", got)
	}
}

// Regression: a seed whose fetch fails at the network layer must not hang the
// crawler. The processor returns such failures as result.Error with Page == nil,
// which must still move the row out of 'processing' to a terminal state — else
// the HasQueuedItems()-based worker exit never fires (see handleProcessingResult).
func TestNetworkErrorDoesNotHangAndMarksTerminal(t *testing.T) {
	// Stand up a server, capture its URL, then close it so the address refuses
	// connections — a deterministic network-layer failure.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := server.URL
	server.Close()

	cfg := baseCfg()
	cfg.SeedURLs = []string{deadURL}
	cfg.RequestTimeout = 2 * time.Second
	store := newStore(t)
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
			t.Fatalf("Start returned error: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("crawler hung on a network-errored seed (row stuck in 'processing')")
	}

	status, exists := statusOf(t, store, deadURL)
	if !exists {
		t.Fatal("seed row should exist")
	}
	if status == "processing" || status == "pending" {
		t.Errorf("seed status = %q, want a terminal state (not stuck in the queue)", status)
	}
	if status != "error" {
		t.Errorf("seed status = %q, want error", status)
	}
}

// Regression: a malformed seed URL (one that fails url.Parse in the rate
// limiter) must be driven to a terminal state, not left in 'processing' where it
// would hang the HasQueuedItems()-based worker exit.
func TestMalformedURLDoesNotHangAndMarksTerminal(t *testing.T) {
	// A control character makes net/url.Parse fail.
	badURL := "http://example.com/\x7f"

	cfg := baseCfg()
	cfg.SeedURLs = []string{badURL}
	store := newStore(t)
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
			t.Fatalf("Start returned error: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("crawler hung on a malformed seed URL (row stuck in 'processing')")
	}

	if got, _ := statusOf(t, store, badURL); got != "error" {
		t.Errorf("malformed seed status = %q, want error (terminal)", got)
	}
}

// Regression: a database carrying a stale 'processing' row from a previously
// interrupted/crashed run must not hang a resumed crawl. Start() resets stale
// 'processing' rows to 'pending'; without that, HasQueuedItems() would count the
// stale row forever and workers would never exit.
func TestStaleProcessingRowDoesNotHangOnResume(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := server.URL
	server.Close()

	store := newStore(t)
	// Simulate a previous run that claimed the URL (pending -> processing) and
	// then crashed before completing it.
	if err := store.AddToQueue([]string{deadURL}); err != nil {
		t.Fatalf("AddToQueue: %v", err)
	}
	if item, err := store.GetNextFromQueue(); err != nil || item == nil {
		t.Fatalf("GetNextFromQueue (claim): item=%v err=%v", item, err)
	}
	if got, _ := statusOf(t, store, deadURL); got != "processing" {
		t.Fatalf("precondition status = %q, want processing", got)
	}

	cfg := baseCfg()
	cfg.RequestTimeout = 2 * time.Second
	c, err := crawler.NewCrawler(cfg, store)
	if err != nil {
		t.Fatalf("NewCrawler: %v", err)
	}
	t.Cleanup(func() { _ = c.Stop() })

	done := make(chan error, 1)
	go func() { done <- c.Start(context.Background(), nil) }() // resume, no seeds

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Start returned error: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("crawler hung on a stale 'processing' row at resume")
	}

	// The stale row must have been reset and then driven to a terminal state.
	if got, _ := statusOf(t, store, deadURL); got == "processing" || got == "pending" {
		t.Errorf("stale row status = %q, want a terminal state", got)
	}
}

// Test D: a database that holds only 'discovered' nodes (e.g. a seedless resume)
// must not spin forever — workers must exit promptly.
func TestSeedlessResumeWithOnlyDiscoveredDoesNotHang(t *testing.T) {
	store := newStore(t)
	if err := store.SaveLinks([]*crawler.LinkData{
		{SourceURL: "https://example.com/a", TargetURL: "https://example.com/b", LinkType: "internal"},
	}); err != nil {
		t.Fatalf("SaveLinks: %v", err)
	}

	cfg := baseCfg()
	c, err := crawler.NewCrawler(cfg, store)
	if err != nil {
		t.Fatalf("NewCrawler: %v", err)
	}
	t.Cleanup(func() { _ = c.Stop() })

	done := make(chan error, 1)
	go func() { done <- c.Start(context.Background(), nil) }()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Start returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("crawler did not exit on a discovered-only database (infinite sleep)")
	}
}
