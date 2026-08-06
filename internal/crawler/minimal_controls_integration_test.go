package crawler_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/masahif/linktadoru/internal/config"
	"github.com/masahif/linktadoru/internal/crawler"
)

func minimalConfig(seeds []string, maxDepth, concurrency int) *config.CrawlConfig {
	cfg := config.DefaultConfig()
	cfg.SeedURLs = seeds
	cfg.MaxDepth = maxDepth
	cfg.Concurrency = concurrency
	cfg.RequestDelay = 0.001
	cfg.RequestTimeout = 3 * time.Second
	cfg.IgnoreRobotsTxt = true
	cfg.Limit = 0
	return cfg
}

func startCrawler(t *testing.T, cfg *config.CrawlConfig, store crawler.Storage) {
	t.Helper()
	c, err := crawler.NewCrawler(cfg, store)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = c.Stop() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Start(ctx, cfg.SeedURLs); err != nil {
		t.Fatal(err)
	}
}

func TestMaxDepthOneStopsAfterDirectLinks(t *testing.T) {
	var grandchildRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		switch r.URL.Path {
		case "/":
			_, _ = fmt.Fprint(w, `<a href="/child">child</a>`)
		case "/child":
			_, _ = fmt.Fprint(w, `<a href="/grandchild">grandchild</a>`)
		case "/grandchild":
			grandchildRequests.Add(1)
			_, _ = fmt.Fprint(w, "leaf")
		}
	}))
	defer server.Close()

	store := newStore(t)
	startCrawler(t, minimalConfig([]string{server.URL}, 1, 2), store)

	for _, u := range []string{server.URL, server.URL + "/child"} {
		if got, _ := statusOf(t, store, u); got != "completed" {
			t.Fatalf("%s status = %q, want completed", u, got)
		}
	}
	if got, exists := statusOf(t, store, server.URL+"/grandchild"); !exists || got != "discovered" {
		t.Fatalf("grandchild status = %q exists=%v, want discovered", got, exists)
	}
	if got := grandchildRequests.Load(); got != 0 {
		t.Fatalf("grandchild was fetched %d times", got)
	}
}

func TestMaxDepthZeroKeepsExistingUnboundedBehavior(t *testing.T) {
	var grandchildRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		switch r.URL.Path {
		case "/":
			_, _ = fmt.Fprint(w, `<a href="/child">child</a>`)
		case "/child":
			_, _ = fmt.Fprint(w, `<a href="/grandchild">grandchild</a>`)
		case "/grandchild":
			grandchildRequests.Add(1)
		}
	}))
	defer server.Close()

	store := newStore(t)
	startCrawler(t, minimalConfig([]string{server.URL}, 0, 2), store)
	if got := grandchildRequests.Load(); got != 1 {
		t.Fatalf("grandchild requests = %d, want 1", got)
	}
}

func TestMaxDepthOneDoesNotWaitForSlowSeed(t *testing.T) {
	releaseSlow := make(chan struct{})
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(releaseSlow) }) }
	slowStarted := make(chan struct{}, 1)
	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slowStarted <- struct{}{}
		<-releaseSlow
		w.Header().Set("Content-Type", "text/html")
	}))
	defer slow.Close()
	defer release()

	childRequested := make(chan struct{}, 1)
	fast := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		if r.URL.Path == "/" {
			_, _ = fmt.Fprint(w, `<a href="/child">child</a>`)
			return
		}
		childRequested <- struct{}{}
	}))
	defer fast.Close()

	store := newStore(t)
	cfg := minimalConfig([]string{slow.URL, fast.URL}, 1, 2)
	c, err := crawler.NewCrawler(cfg, store)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Stop() }()
	done := make(chan error, 1)
	go func() { done <- c.Start(context.Background(), cfg.SeedURLs) }()

	select {
	case <-slowStarted:
	case <-time.After(time.Second):
		t.Fatal("slow seed was not requested")
	}
	select {
	case <-childRequested:
	case <-time.After(2 * time.Second):
		t.Fatal("fast seed child waited for the slow seed")
	}
	release()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestTransientHTTPRetriesAndRecoversLinks(t *testing.T) {
	var seedRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		if r.URL.Path == "/child" {
			_, _ = fmt.Fprint(w, "child")
			return
		}
		attempt := seedRequests.Add(1)
		if attempt < 3 {
			w.Header().Set("X-Attempt", fmt.Sprint(attempt))
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = fmt.Fprint(w, "temporary")
			return
		}
		_, _ = fmt.Fprint(w, `<a href="/child">child</a>`)
	}))
	defer server.Close()

	store := newStore(t)
	startCrawler(t, minimalConfig([]string{server.URL}, 1, 1), store)
	if got := seedRequests.Load(); got != 3 {
		t.Fatalf("seed requests = %d, want 3", got)
	}
	for _, u := range []string{server.URL, server.URL + "/child"} {
		if got, _ := statusOf(t, store, u); got != "completed" {
			t.Fatalf("%s status = %q, want completed", u, got)
		}
	}
}

func TestTransientHTTPStopsAfterThreeAttempts(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "text/html")
		w.Header().Set("X-Test", "preserved")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = fmt.Fprint(w, "temporary")
	}))
	defer server.Close()

	store := newStore(t)
	cfg := minimalConfig([]string{server.URL}, 1, 1)
	cfg.Limit = 1
	startCrawler(t, cfg, store)
	if got := requests.Load(); got != 3 {
		t.Fatalf("requests = %d, want 3", got)
	}
	if got, _ := statusOf(t, store, server.URL); got != "error" {
		t.Fatalf("status = %q, want error", got)
	}
}

func TestPermanentHTTPStatusIsNotRetried(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	store := newStore(t)
	startCrawler(t, minimalConfig([]string{server.URL}, 1, 1), store)
	if got := requests.Load(); got != 1 {
		t.Fatalf("requests = %d, want 1", got)
	}
	if got, _ := statusOf(t, store, server.URL); got != "completed" {
		t.Fatalf("status = %q, want completed", got)
	}
}

func TestMaxDepthOneRejectsMissingSeeds(t *testing.T) {
	store := newStore(t)
	cfg := minimalConfig(nil, 1, 1)
	c, err := crawler.NewCrawler(cfg, store)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Stop() }()
	if err := c.Start(context.Background(), nil); err == nil {
		t.Fatal("max_depth=1 run without seeds was accepted")
	}
}

func TestMaxDepthOneRejectsNonEmptyDatabase(t *testing.T) {
	store := newStore(t)
	const seed = "https://example.com"
	if err := store.AddToQueue([]string{seed}); err != nil {
		t.Fatal(err)
	}
	cfg := minimalConfig([]string{seed}, 1, 1)
	c, err := crawler.NewCrawler(cfg, store)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = c.Stop() }()
	if err := c.Start(context.Background(), cfg.SeedURLs); err == nil {
		t.Fatal("max_depth=1 accepted a non-empty database")
	}
}
