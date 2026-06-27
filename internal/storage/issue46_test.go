package storage

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/masahif/linktadoru/internal/crawler"
)

// newTempStorage creates an isolated SQLite storage backed by a temp file.
func newTempStorage(t *testing.T) *SQLiteStorage {
	t.Helper()
	dbFile := filepath.Join(t.TempDir(), "issue46.db")
	store, err := NewSQLiteStorage(dbFile)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func mustStatus(t *testing.T, s *SQLiteStorage, url string) string {
	t.Helper()
	status, exists := s.GetURLStatus(url)
	if !exists {
		t.Fatalf("expected URL %q to exist", url)
	}
	return status
}

// Test A: saving links creates link-graph nodes as 'discovered', and those nodes
// are NOT handed out by GetNextFromQueue (i.e. they are not queued for crawling).
func TestSaveLinksCreatesDiscoveredNotQueued(t *testing.T) {
	store := newTempStorage(t)

	source := "https://example.com/from"
	target := "https://example.com/to"
	if err := store.SaveLinks([]*crawler.LinkData{
		{SourceURL: source, TargetURL: target, LinkType: "internal"},
	}); err != nil {
		t.Fatalf("SaveLinks failed: %v", err)
	}

	if got := mustStatus(t, store, source); got != "discovered" {
		t.Errorf("source status = %q, want discovered", got)
	}
	if got := mustStatus(t, store, target); got != "discovered" {
		t.Errorf("target status = %q, want discovered", got)
	}

	// Nothing should be crawlable: GetNextFromQueue only returns 'pending' rows.
	item, err := store.GetNextFromQueue()
	if err != nil {
		t.Fatalf("GetNextFromQueue failed: %v", err)
	}
	if item != nil {
		t.Errorf("expected empty queue, got %+v", item)
	}
}

// Test A (singular): SaveLink also creates its source/target as 'discovered'
// link-graph nodes that are not queued. SaveLink goes through getOrCreatePageID,
// a separate path from SaveLinks/saveLinksBatch (issue #46).
func TestSaveLinkCreatesDiscoveredNotQueued(t *testing.T) {
	store := newTempStorage(t)

	source := "https://example.com/single-from"
	target := "https://example.com/single-to"
	if err := store.SaveLink(&crawler.LinkData{
		SourceURL: source, TargetURL: target, LinkType: "internal",
	}); err != nil {
		t.Fatalf("SaveLink failed: %v", err)
	}

	if got := mustStatus(t, store, source); got != "discovered" {
		t.Errorf("source status = %q, want discovered", got)
	}
	if got := mustStatus(t, store, target); got != "discovered" {
		t.Errorf("target status = %q, want discovered", got)
	}

	item, err := store.GetNextFromQueue()
	if err != nil {
		t.Fatalf("GetNextFromQueue failed: %v", err)
	}
	if item != nil {
		t.Errorf("expected empty queue, got %+v", item)
	}
}

// Test B: AddToQueue inserts new URLs as 'pending', promotes 'discovered' nodes
// to 'pending', and leaves every terminal/in-flight status untouched.
func TestAddToQueuePromotesOnlyDiscovered(t *testing.T) {
	t.Run("new URL becomes pending", func(t *testing.T) {
		store := newTempStorage(t)
		url := "https://example.com/new"
		if err := store.AddToQueue([]string{url}); err != nil {
			t.Fatalf("AddToQueue: %v", err)
		}
		if got := mustStatus(t, store, url); got != "pending" {
			t.Errorf("status = %q, want pending", got)
		}
	})

	t.Run("discovered URL is promoted to pending", func(t *testing.T) {
		store := newTempStorage(t)
		url := "https://example.com/disc"
		if err := store.SaveLinks([]*crawler.LinkData{
			{SourceURL: "https://example.com/src", TargetURL: url, LinkType: "internal"},
		}); err != nil {
			t.Fatalf("SaveLinks: %v", err)
		}
		if got := mustStatus(t, store, url); got != "discovered" {
			t.Fatalf("precondition status = %q, want discovered", got)
		}
		if err := store.AddToQueue([]string{url}); err != nil {
			t.Fatalf("AddToQueue: %v", err)
		}
		if got := mustStatus(t, store, url); got != "pending" {
			t.Errorf("status = %q, want pending", got)
		}
	})

	// Each terminal/in-flight status must survive a re-queue attempt unchanged.
	for _, tc := range []struct {
		name   string
		setup  func(t *testing.T, s *SQLiteStorage, id int)
		expect string
	}{
		{
			name:   "completed",
			setup:  func(t *testing.T, s *SQLiteStorage, id int) { mustSaveResult(t, s, id) },
			expect: "completed",
		},
		{
			name:   "processing",
			setup:  func(t *testing.T, s *SQLiteStorage, id int) {}, // GetNextFromQueue already set it
			expect: "processing",
		},
		{
			name: "skipped",
			setup: func(t *testing.T, s *SQLiteStorage, id int) {
				if err := s.SavePageSkipped(id, "robots", "blocked"); err != nil {
					t.Fatalf("SavePageSkipped: %v", err)
				}
			},
			expect: "skipped",
		},
		{
			name: "error",
			setup: func(t *testing.T, s *SQLiteStorage, id int) {
				if err := s.SavePageError(id, "network_timeout", "boom"); err != nil {
					t.Fatalf("SavePageError: %v", err)
				}
			},
			expect: "error",
		},
	} {
		t.Run(tc.name+" is left untouched", func(t *testing.T) {
			store := newTempStorage(t)
			url := "https://example.com/" + tc.name
			if err := store.AddToQueue([]string{url}); err != nil {
				t.Fatalf("AddToQueue: %v", err)
			}
			item, err := store.GetNextFromQueue() // pending -> processing
			if err != nil || item == nil {
				t.Fatalf("GetNextFromQueue: item=%v err=%v", item, err)
			}
			tc.setup(t, store, item.ID)
			if got := mustStatus(t, store, url); got != tc.expect {
				t.Fatalf("precondition status = %q, want %q", got, tc.expect)
			}

			// Re-queueing must NOT change a non-discovered row.
			if err := store.AddToQueue([]string{url}); err != nil {
				t.Fatalf("AddToQueue (re): %v", err)
			}
			if got := mustStatus(t, store, url); got != tc.expect {
				t.Errorf("status after re-queue = %q, want %q (unchanged)", got, tc.expect)
			}
		})
	}
}

// CleanupStaleProcessing must reset every 'processing' row — including one whose
// processing_started_at is NULL. `NULL < ?` is never true in SQL, so without the
// explicit IS NULL clause such a row would survive and keep HasQueuedItems() true
// forever, re-introducing the worker hang (review finding).
func TestCleanupStaleProcessingResetsNullAndOldTimestamps(t *testing.T) {
	store := newTempStorage(t)

	// Anomalous: 'processing' with a NULL start time.
	if _, err := store.db.Exec(
		"INSERT INTO pages (url, status, processing_started_at) VALUES ('https://example.com/null-ts', 'processing', NULL)",
	); err != nil {
		t.Fatalf("seed null-ts: %v", err)
	}
	// Normal stale: 'processing' claimed an hour ago.
	hourAgo := time.Now().Add(-time.Hour)
	if _, err := store.db.Exec(
		"INSERT INTO pages (url, status, added_at, processing_started_at) VALUES ('https://example.com/old-ts', 'processing', ?, ?)",
		hourAgo, hourAgo,
	); err != nil {
		t.Fatalf("seed old-ts: %v", err)
	}

	if err := store.CleanupStaleProcessing(0); err != nil {
		t.Fatalf("CleanupStaleProcessing: %v", err)
	}

	for _, u := range []string{"https://example.com/null-ts", "https://example.com/old-ts"} {
		if got := mustStatus(t, store, u); got != "pending" {
			t.Errorf("%s status = %q, want pending (reset)", u, got)
		}
	}
}

func mustSaveResult(t *testing.T, s *SQLiteStorage, id int) {
	t.Helper()
	if err := s.SavePageResult(id, &crawler.PageData{
		StatusCode:  200,
		HTTPHeaders: map[string]string{"content-type": "text/html"},
	}); err != nil {
		t.Fatalf("SavePageResult: %v", err)
	}
}

// Migration test: a database created before issue #46 (CHECK without 'discovered')
// is rebuilt by InitSchema so the new status is accepted and existing rows survive.
func TestMigratePagesAddDiscovered(t *testing.T) {
	dbFile := filepath.Join(t.TempDir(), "legacy.db")

	// Build a legacy database by stripping 'discovered' from the current schema.
	legacySchema := strings.Replace(schemaSQL, ", 'discovered'", "", 1)
	if legacySchema == schemaSQL {
		t.Fatal("failed to derive legacy schema; marker not found")
	}

	legacy := &SQLiteStorage{}
	{
		store, err := NewSQLiteStorage(dbFile)
		if err != nil {
			t.Fatalf("open: %v", err)
		}
		// Replace the up-to-date pages table with the legacy one.
		if _, err := store.db.Exec("DROP VIEW IF EXISTS links; DROP VIEW IF EXISTS completed_pages; DROP VIEW IF EXISTS queue_status; DROP TABLE IF EXISTS link_relations; DROP TABLE pages;"); err != nil {
			t.Fatalf("reset: %v", err)
		}
		if _, err := store.db.Exec(legacySchema); err != nil {
			t.Fatalf("legacy schema: %v", err)
		}
		// Seed legacy rows + a link relation so views have real data to read back.
		if _, err := store.db.Exec(`
			INSERT INTO pages (id, url, status, response_http_headers) VALUES
				(1, 'https://example.com/legacy', 'completed', '{"content-type":"text/html"}'),
				(2, 'https://example.com/target', 'pending', NULL);
			INSERT INTO link_relations (source_page_id, target_page_id, link_type)
				VALUES (1, 2, 'internal');
		`); err != nil {
			t.Fatalf("seed rows: %v", err)
		}
		if _, err := store.db.Exec("INSERT INTO pages (url, status) VALUES ('https://example.com/x', 'discovered')"); err == nil {
			t.Fatal("legacy schema unexpectedly accepted 'discovered'")
		}
		legacy = store
	}

	// Run the migration (and schema recreate) on the legacy DB.
	if err := legacy.InitSchema(); err != nil {
		t.Fatalf("InitSchema (migration) failed: %v", err)
	}

	// Existing data preserved.
	if got := mustStatus(t, legacy, "https://example.com/legacy"); got != "completed" {
		t.Errorf("legacy row status = %q, want completed", got)
	}
	// 'discovered' now accepted.
	if _, err := legacy.db.Exec("INSERT INTO pages (url, status) VALUES ('https://example.com/now', 'discovered')"); err != nil {
		t.Errorf("post-migration insert of 'discovered' failed: %v", err)
	}

	// Views recreated and readable against the preserved data.
	for _, view := range []string{"queue_status", "completed_pages", "links"} {
		if _, err := legacy.db.Exec("SELECT count(*) FROM " + view); err != nil {
			t.Errorf("view %q not usable after migration: %v", view, err)
		}
	}
	// completed_pages must expose the preserved completed row.
	var completed int
	if err := legacy.db.QueryRow(
		"SELECT count(*) FROM completed_pages WHERE url = 'https://example.com/legacy'",
	).Scan(&completed); err != nil || completed != 1 {
		t.Errorf("completed_pages missing preserved row: count=%d err=%v", completed, err)
	}
	// links view must resolve the preserved relation back to URLs.
	var src, tgt string
	if err := legacy.db.QueryRow(
		"SELECT source_url, target_url FROM links LIMIT 1",
	).Scan(&src, &tgt); err != nil {
		t.Errorf("links view unreadable after migration: %v", err)
	} else if src != "https://example.com/legacy" || tgt != "https://example.com/target" {
		t.Errorf("links view rows = (%q -> %q), want legacy -> target", src, tgt)
	}

	// Index restored on the rebuilt table.
	var idx int
	if err := legacy.db.QueryRow(
		"SELECT count(*) FROM sqlite_master WHERE type='index' AND name='idx_pages_status'",
	).Scan(&idx); err != nil || idx != 1 {
		t.Errorf("idx_pages_status not restored: count=%d err=%v", idx, err)
	}
	// Foreign keys re-enabled after the migration.
	var fk int
	if err := legacy.db.QueryRow("PRAGMA foreign_keys").Scan(&fk); err != nil || fk != 1 {
		t.Errorf("foreign_keys not restored to ON: fk=%d err=%v", fk, err)
	}
}
