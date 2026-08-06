package storage

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/masahif/linktadoru/internal/crawler"
)

// A database created before issue #71 must gain the new columns without losing
// anything, and must be usable immediately afterwards. The columns arrive by
// ALTER TABLE, so existing rows read NULL — which is the correct meaning for
// them: no wait was ever recorded, and nobody knows which attempt those old
// crawl_errors rows were.
func TestMigrateAddsTransientColumnsToLegacyDatabase(t *testing.T) {
	dbFile := filepath.Join(t.TempDir(), "pre71.db")

	// Derive a pre-#71 schema by removing the columns this issue added.
	legacySchema := schemaSQL
	for _, marker := range []string{
		"\n    retry_after DATETIME",
		"\n    status_code INTEGER,\n    -- 1 for the first attempt at this URL, counting up.\n    attempt INTEGER,",
	} {
		stripped := strings.Replace(legacySchema, marker, "", 1)
		if stripped == legacySchema {
			t.Fatalf("failed to derive legacy schema; marker not found: %q", marker)
		}
		legacySchema = stripped
	}
	// The pages row above left a trailing comma on last_error_message.
	legacySchema = strings.Replace(legacySchema, "last_error_message TEXT,\n", "last_error_message TEXT\n", 1)

	store, err := NewSQLiteStorage(dbFile)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if _, err := store.db.Exec(`
		DROP VIEW IF EXISTS links;
		DROP VIEW IF EXISTS completed_pages;
		DROP VIEW IF EXISTS queue_status;
		DROP TABLE IF EXISTS link_relations;
		DROP TABLE pages;
		DROP TABLE crawl_errors;
	`); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if _, err := store.db.Exec(legacySchema); err != nil {
		t.Fatalf("legacy schema: %v", err)
	}

	// Seed rows that must survive, including a queued one with a depth.
	if _, err := store.db.Exec(`
		INSERT INTO pages (id, url, status, depth, retry_count, response_http_headers) VALUES
			(1, 'https://example.com/done', 'completed', 0, 0, '{"content-type":"text/html"}'),
			(2, 'https://example.com/queued', 'pending', 1, 2, NULL);
		INSERT INTO crawl_errors (url, error_type, error_message)
			VALUES ('https://example.com/old', 'network_error', 'refused');
	`); err != nil {
		t.Fatalf("seed rows: %v", err)
	}

	if err := store.InitSchema(); err != nil {
		t.Fatalf("InitSchema (migration): %v", err)
	}

	// Existing values preserved, including the depth an earlier migration added.
	var status string
	var depth, retryCount sql.NullInt64
	var retryAfter sql.NullString
	if err := store.db.QueryRow(
		"SELECT status, depth, retry_count, retry_after FROM pages WHERE id = 2",
	).Scan(&status, &depth, &retryCount, &retryAfter); err != nil {
		t.Fatalf("select migrated row: %v", err)
	}
	if status != "pending" || !depth.Valid || depth.Int64 != 1 || !retryCount.Valid || retryCount.Int64 != 2 {
		t.Errorf("row 2 = (%q, %v, %v), want (pending, 1, 2)", status, depth, retryCount)
	}
	if retryAfter.Valid {
		t.Errorf("retry_after = %v, want NULL so old work retries immediately", retryAfter)
	}

	// Old crawl_errors rows gain the columns as NULL rather than a false zero.
	var oldStatus, oldAttempt sql.NullInt64
	if err := store.db.QueryRow(
		"SELECT status_code, attempt FROM crawl_errors WHERE url = 'https://example.com/old'",
	).Scan(&oldStatus, &oldAttempt); err != nil {
		t.Fatalf("select migrated crawl_errors row: %v", err)
	}
	if oldStatus.Valid || oldAttempt.Valid {
		t.Errorf("status_code=%v attempt=%v, want NULL for a pre-migration attempt", oldStatus, oldAttempt)
	}

	// The migrated database is immediately usable for the new path.
	item := queueOne(t, store, "https://example.com/fresh")
	if err := store.SaveFailedAttempt(item.ID, transientPage(503), "http_503", "unavailable", time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("SaveFailedAttempt on migrated DB: %v", err)
	}
	if err := store.SaveError(&crawler.CrawlError{
		URL: "https://example.com/fresh", ErrorType: "http_503", StatusCode: 503, Attempt: 1,
		OccurredAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("SaveError on migrated DB: %v", err)
	}
	if _, err := store.GetRunSummary(); err != nil {
		t.Fatalf("GetRunSummary on migrated DB: %v", err)
	}
}

// Re-running the migrations must preserve every ALTER-added column.
//
// This guards the hazard documented on pagesBaseColumns: that list is frozen at
// the pre-depth column set, so any migration that rebuilds the pages table
// would copy only those columns and silently return NULL for the ones added
// since — 'depth' and 'retry_after'. Today the ordering saves us (the rebuild
// runs before the ALTERs), and this test is what would notice if a future
// migration broke that arrangement.
func TestMigrationsPreserveAlterAddedColumns(t *testing.T) {
	store, err := NewSQLiteStorage(filepath.Join(t.TempDir(), "idempotent.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if err := store.AddToQueueWithDepth([]string{"https://example.com/a"}, 2); err != nil {
		t.Fatalf("AddToQueueWithDepth: %v", err)
	}
	item, err := store.GetNextFromQueueAtDepth(2)
	if err != nil || item == nil {
		t.Fatalf("GetNextFromQueueAtDepth: item=%v err=%v", item, err)
	}

	due := time.Now().Add(time.Hour)
	if err := store.SaveFailedAttempt(item.ID, transientPage(503), "http_503", "unavailable", due); err != nil {
		t.Fatalf("SaveFailedAttempt: %v", err)
	}

	read := func(what string) (sql.NullInt64, sql.NullString) {
		t.Helper()
		var depth sql.NullInt64
		var retryAfter sql.NullString
		if err := store.db.QueryRow(
			"SELECT depth, retry_after FROM pages WHERE id = ?", item.ID,
		).Scan(&depth, &retryAfter); err != nil {
			t.Fatalf("select %s: %v", what, err)
		}
		return depth, retryAfter
	}

	beforeDepth, beforeRetry := read("before")
	if !beforeDepth.Valid || beforeDepth.Int64 != 2 {
		t.Fatalf("depth = %v before re-migration, want 2", beforeDepth)
	}
	if !beforeRetry.Valid {
		t.Fatal("retry_after is NULL before re-migration; the test cannot detect a loss")
	}

	for i := 0; i < 2; i++ {
		if err := store.InitSchema(); err != nil {
			t.Fatalf("InitSchema pass %d: %v", i, err)
		}
	}

	afterDepth, afterRetry := read("after")
	if afterDepth != beforeDepth {
		t.Errorf("depth changed across re-migration: %v -> %v", beforeDepth, afterDepth)
	}
	if afterRetry.String != beforeRetry.String {
		t.Errorf("retry_after changed across re-migration: %q -> %q", beforeRetry.String, afterRetry.String)
	}
}
