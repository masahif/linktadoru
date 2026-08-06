package crawler_test

// Scheduling coverage for issue #72: at max_depth 1 a slow seed must not hold
// back the depth-1 work other seeds have already produced.
//
// The layered modes are covered where they already were —
// TestMaxDepthCrawlsLayerByLayer and TestMaxDepthRetriesWithinLayerBeforeAdvancing
// in depth_test.go — and the bound itself by TestMaxDepthBoundsAChain.

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/masahif/linktadoru/internal/config"
	"github.com/masahif/linktadoru/internal/crawler"
	"github.com/masahif/linktadoru/internal/storage"

	_ "modernc.org/sqlite"
)

func depthOneCfg() *config.CrawlConfig {
	return &config.CrawlConfig{
		MaxDepth:        1,
		Concurrency:     2,
		RequestDelay:    0.001,
		RequestTimeout:  10 * time.Second,
		UserAgent:       "LinkTadoru-Test/1.0",
		IgnoreRobotsTxt: true,
		AllowedSchemes:  []string{"http://", "https://"},
	}
}

// runScheduled crawls to completion and returns a connection to the database.
func runScheduled(t *testing.T, cfg *config.CrawlConfig, seeds []string, timeout time.Duration) *sql.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "scheduled.db")
	store, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}

	cfg.SeedURLs = seeds
	c, err := crawler.NewCrawler(cfg, store)
	if err != nil {
		_ = store.Close()
		t.Fatalf("NewCrawler: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- c.Start(context.Background(), seeds) }()
	select {
	case err := <-done:
		if err != nil {
			_ = store.Close()
			t.Fatalf("Start: %v", err)
		}
	case <-time.After(timeout):
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
	return db
}

func depthOfURL(t *testing.T, db *sql.DB, url string) sql.NullInt64 {
	t.Helper()
	var depth sql.NullInt64
	if err := db.QueryRow("SELECT depth FROM pages WHERE url = ?", url).Scan(&depth); err != nil {
		t.Fatalf("select depth of %s: %v", url, err)
	}
	return depth
}

// The headline of issue #72: one slow seed must not stop every worker.
//
// Seed A answers at once and links to /a-child. Seed B blocks. Under the strict
// barrier no depth-1 row can be claimed until every depth-0 row has landed, so
// /a-child would wait for B — and this test would deadlock until its timeout.
func TestMaxDepthOneSlowSeedDoesNotBlockOtherChildren(t *testing.T) {
	release := make(chan struct{})
	childReached := make(chan struct{})
	var childOnce sync.Once

	mux := http.NewServeMux()
	mux.HandleFunc("/a", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<html><body><a href="/a-child">child</a></body></html>`)
	})
	mux.HandleFunc("/a-child", func(w http.ResponseWriter, r *http.Request) {
		childOnce.Do(func() { close(childReached) })
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<html><body>child</body></html>`)
	})
	mux.HandleFunc("/b", func(w http.ResponseWriter, r *http.Request) {
		<-release // the slow seed
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<html><body>slow seed</body></html>`)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	dbPath := filepath.Join(t.TempDir(), "hol.db")
	store, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStorage: %v", err)
	}
	defer func() { _ = store.Close() }()

	cfg := depthOneCfg() // Concurrency 2: one worker on the slow seed, one free
	seeds := []string{server.URL + "/a", server.URL + "/b"}
	cfg.SeedURLs = seeds

	c, err := crawler.NewCrawler(cfg, store)
	if err != nil {
		t.Fatalf("NewCrawler: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- c.Start(context.Background(), seeds) }()

	select {
	case <-childReached:
		// The depth-1 child started while the depth-0 seed was still in flight.
	case <-time.After(15 * time.Second):
		close(release)
		<-done
		t.Fatal("a depth-1 child never started while a depth-0 seed was in flight; the slow seed blocked the crawl")
	}

	close(release)
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Start: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("crawler did not terminate")
	}
}

// Retries run after the normal queue drains, and whatever they recover has to
// be drained too — including depth-1 pages that did not exist until the seed
// finally answered.
func TestMaxDepthOneDeferredRetryDrainsNewChildren(t *testing.T) {
	var seedHits atomic.Int32
	var childSeen atomic.Bool

	mux := http.NewServeMux()
	mux.HandleFunc("/flaky", func(w http.ResponseWriter, r *http.Request) {
		if seedHits.Add(1) == 1 {
			w.Header().Set("Retry-After", "0")
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<html><body><a href="/late-child">c</a></body></html>`)
	})
	mux.HandleFunc("/late-child", func(w http.ResponseWriter, r *http.Request) {
		childSeen.Store(true)
		w.Header().Set("Content-Type", "text/html")
		_, _ = fmt.Fprint(w, `<html><body>child</body></html>`)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	db := runScheduled(t, depthOneCfg(), []string{server.URL + "/flaky"}, 30*time.Second)

	if !childSeen.Load() {
		t.Error("the child discovered by the retry was never crawled")
	}
	if d := depthOfURL(t, db, server.URL+"/late-child"); !d.Valid || d.Int64 != 1 {
		t.Errorf("late child depth = %v, want 1", d)
	}
}
