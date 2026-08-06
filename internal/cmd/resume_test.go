package cmd

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/masahif/linktadoru/internal/crawler"
	"github.com/masahif/linktadoru/internal/storage"
)

// seedRetryableError leaves the database in the state an interrupted run does:
// one page in 'error' with retries still owed, and nothing queued.
func seedRetryableError(t *testing.T, dbPath, url string) {
	t.Helper()

	store, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to open storage: %v", err)
	}
	defer func() { _ = store.Close() }()

	if err := store.AddToQueue([]string{url}); err != nil {
		t.Fatalf("failed to queue URL: %v", err)
	}
	item, err := store.GetNextFromQueue()
	if err != nil || item == nil {
		t.Fatalf("failed to claim URL: (%v, %v)", item, err)
	}
	if err := store.SaveFailedAttempt(item.ID, nil, "network_error", "interrupted", time.Time{}); err != nil {
		t.Fatalf("failed to record error: %v", err)
	}

	pending, processing, _, _, err := store.GetQueueStatus()
	if err != nil {
		t.Fatalf("queue status failed: %v", err)
	}
	if pending+processing != 0 {
		t.Fatalf("test setup: expected an empty queue, got %d pending and %d processing", pending, processing)
	}
}

// resumeCmd builds the command runCrawler reads its flags from.
func resumeCmd(t *testing.T, dbPath string) *cobra.Command {
	t.Helper()

	cmd := &cobra.Command{}
	cmd.Flags().Bool("show-config", false, "")
	cmd.Flags().String("seed-file", "", "")
	cmd.Flags().String("database", dbPath, "")
	cmd.Flags().Int("max-depth", 0, "")
	if err := viper.BindPFlag("database_path", cmd.Flags().Lookup("database")); err != nil {
		t.Fatalf("failed to bind database flag: %v", err)
	}
	if err := viper.BindPFlag("max_depth", cmd.Flags().Lookup("max-depth")); err != nil {
		t.Fatalf("failed to bind max-depth flag: %v", err)
	}
	return cmd
}

// A database whose only remaining work is a retryable failure must be resumed,
// not dismissed as empty: the crawler still has a retry to spend on it.
func TestResumeRunsWithOnlyRetryableErrors(t *testing.T) {
	viper.Reset()
	defer viper.Reset()

	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body>recovered</body></html>"))
	}))
	defer server.Close()

	dbPath := filepath.Join(t.TempDir(), "retry.db")
	seedRetryableError(t, dbPath, server.URL+"/page")

	cmd := resumeCmd(t, dbPath)
	cmd.SetContext(context.Background())
	viper.Set("ignore_robots_txt", true)
	viper.Set("request_delay", 0.1)

	if err := runCrawler(cmd, []string{}); err != nil {
		t.Fatalf("resume failed: %v", err)
	}

	if hits.Load() == 0 {
		t.Fatal("the retryable failure was never retried; the run exited as if there were nothing to crawl")
	}

	store, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to reopen storage: %v", err)
	}
	defer func() { _ = store.Close() }()

	if status, _ := store.GetURLStatus(server.URL + "/page"); status != "completed" {
		t.Errorf("status = %q, want completed after the retry succeeded", status)
	}
}

// The same database under --max-depth must be refused before anything is
// fetched: those rows have no recorded depth, so the bound has no origin.
func TestBoundedResumeRefusesDepthlessRetryableErrors(t *testing.T) {
	viper.Reset()
	defer viper.Reset()

	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
	}))
	defer server.Close()

	dbPath := filepath.Join(t.TempDir(), "legacy-retry.db")
	seedRetryableError(t, dbPath, server.URL+"/page")

	cmd := resumeCmd(t, dbPath)
	cmd.SetContext(context.Background())
	if err := cmd.Flags().Set("max-depth", "1"); err != nil {
		t.Fatalf("failed to set max-depth: %v", err)
	}
	viper.Set("ignore_robots_txt", true)
	viper.Set("request_delay", 0.1)

	err := runCrawler(cmd, []string{})
	if err == nil {
		t.Fatal("expected a bounded resume to refuse a database whose failures have no depth")
	}
	if !strings.Contains(err.Error(), "--max-depth cannot be used") {
		t.Errorf("unexpected error: %v", err)
	}
	if got := hits.Load(); got != 0 {
		t.Errorf("server was contacted %d time(s); the run must be refused before fetching", got)
	}
}

// The CLI's resume check and the crawler's retry budget must be the same
// number, or the run either starts with nothing to do or skips work.
func TestResumeUsesTheCrawlerRetryBudget(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "budget.db")
	seedRetryableError(t, dbPath, "https://example.com/page")

	store, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to open storage: %v", err)
	}
	defer func() { _ = store.Close() }()

	resumable, err := store.HasResumableWork(crawler.MaxRetries)
	if err != nil {
		t.Fatalf("HasResumableWork failed: %v", err)
	}
	if !resumable {
		t.Error("a failure with retries left must count as resumable work")
	}

	// With no budget left there is nothing to resume.
	resumable, err = store.HasResumableWork(1)
	if err != nil {
		t.Fatalf("HasResumableWork failed: %v", err)
	}
	if resumable {
		t.Error("a failure that has used its budget must not count as resumable work")
	}
}

// The bounded path must reach its retries through the CLI too: a database whose
// only work is a depth-tagged retryable failure has nothing pending, so the run
// has to get past the resume check, into Start, and through the layer loop for
// the retry to happen at all.
func TestBoundedResumeRetriesDepthfulErrorsOnly(t *testing.T) {
	viper.Reset()
	defer viper.Reset()

	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body>recovered</body></html>"))
	}))
	defer server.Close()

	dbPath := filepath.Join(t.TempDir(), "bounded-retry.db")
	pageURL := server.URL + "/page"

	func() {
		store, err := storage.NewSQLiteStorage(dbPath)
		if err != nil {
			t.Fatalf("failed to open storage: %v", err)
		}
		defer func() { _ = store.Close() }()

		// A bounded run interrupted after the page failed: depth recorded,
		// retries still owed, nothing queued.
		if err := store.AddToQueueWithDepth([]string{pageURL}, 0); err != nil {
			t.Fatalf("failed to queue URL: %v", err)
		}
		item, err := store.GetNextFromQueueAtDepth(0)
		if err != nil || item == nil {
			t.Fatalf("failed to claim URL: (%v, %v)", item, err)
		}
		if err := store.SaveFailedAttempt(item.ID, nil, "network_error", "interrupted", time.Time{}); err != nil {
			t.Fatalf("failed to record error: %v", err)
		}

		pending, processing, _, _, err := store.GetQueueStatus()
		if err != nil {
			t.Fatalf("queue status failed: %v", err)
		}
		if pending+processing != 0 {
			t.Fatalf("test setup: expected an empty queue, got %d pending and %d processing", pending, processing)
		}
	}()

	cmd := resumeCmd(t, dbPath)
	cmd.SetContext(context.Background())
	if err := cmd.Flags().Set("max-depth", "1"); err != nil {
		t.Fatalf("failed to set max-depth: %v", err)
	}
	viper.Set("ignore_robots_txt", true)
	viper.Set("request_delay", 0.1)

	if err := runCrawler(cmd, []string{}); err != nil {
		t.Fatalf("bounded resume failed: %v", err)
	}

	if hits.Load() == 0 {
		t.Fatal("the depth-tagged retryable failure was never retried")
	}

	store, err := storage.NewSQLiteStorage(dbPath)
	if err != nil {
		t.Fatalf("failed to reopen storage: %v", err)
	}
	defer func() { _ = store.Close() }()

	if status, _ := store.GetURLStatus(pageURL); status != "completed" {
		t.Errorf("status = %q, want completed after the retry succeeded", status)
	}
}
