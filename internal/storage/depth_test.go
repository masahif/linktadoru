package storage

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/masahif/linktadoru/internal/crawler"
)

// depthOfURL reads the recorded depth of a URL.
func depthOfURL(t *testing.T, s *SQLiteStorage, url string) (int, bool) {
	t.Helper()

	var depth *int
	if err := s.db.QueryRow("SELECT depth FROM pages WHERE url = ?", url).Scan(&depth); err != nil {
		t.Fatalf("depth query failed: %v", err)
	}
	if depth == nil {
		return 0, false
	}
	return *depth, true
}

func newTestStore(t *testing.T, name string) *SQLiteStorage {
	t.Helper()

	store, err := NewSQLiteStorage(filepath.Join(t.TempDir(), name))
	if err != nil {
		t.Fatalf("failed to open storage: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

// A link-graph node carries no depth until it is promoted, at which point it
// takes the depth it was queued at.
func TestAddToQueueWithDepthAssignsDepthOnPromotion(t *testing.T) {
	store := newTestStore(t, "promote.db")

	const target = "https://example.com/target"
	if err := store.SaveLinks([]*crawler.LinkData{{
		SourceURL: "https://example.com/source",
		TargetURL: target,
		LinkType:  "internal",
	}}); err != nil {
		t.Fatalf("failed to save link: %v", err)
	}

	if _, recorded := depthOfURL(t, store, target); recorded {
		t.Error("a discovered node must have no depth before it is queued")
	}
	if status, _ := store.GetURLStatus(target); status != "discovered" {
		t.Fatalf("status = %q, want discovered", status)
	}

	if err := store.AddToQueueWithDepth([]string{target}, 3); err != nil {
		t.Fatalf("AddToQueueWithDepth failed: %v", err)
	}

	depth, recorded := depthOfURL(t, store, target)
	if !recorded || depth != 3 {
		t.Errorf("depth = %d (recorded=%v), want 3 assigned at promotion", depth, recorded)
	}
	if status, _ := store.GetURLStatus(target); status != "pending" {
		t.Errorf("status = %q, want pending", status)
	}
}

// A row that has already been crawled keeps its depth: the layer barrier
// promotes each URL once, at its shortest depth, so a later re-queue attempt
// must not rewrite history.
func TestAddToQueueWithDepthLeavesExpandedRowsAlone(t *testing.T) {
	store := newTestStore(t, "expanded.db")

	const url = "https://example.com/page"
	if err := store.AddToQueueWithDepth([]string{url}, 1); err != nil {
		t.Fatalf("AddToQueueWithDepth failed: %v", err)
	}
	item, err := store.GetNextFromQueueAtDepth(1)
	if err != nil || item == nil {
		t.Fatalf("GetNextFromQueueAtDepth returned (%v, %v)", item, err)
	}
	if item.Depth != 1 {
		t.Errorf("claimed item depth = %d, want 1", item.Depth)
	}
	if err := store.UpdatePageStatus(item.ID, "completed"); err != nil {
		t.Fatalf("failed to complete page: %v", err)
	}

	if err := store.AddToQueueWithDepth([]string{url}, 5); err != nil {
		t.Fatalf("AddToQueueWithDepth failed: %v", err)
	}

	if depth, _ := depthOfURL(t, store, url); depth != 1 {
		t.Errorf("depth = %d, want the original 1", depth)
	}
	if status, _ := store.GetURLStatus(url); status != "completed" {
		t.Errorf("status = %q, want completed (a crawled page must not be requeued)", status)
	}
}

// Claiming is scoped to one layer, which is what the barrier rests on.
func TestGetNextFromQueueAtDepthIgnoresOtherLayers(t *testing.T) {
	store := newTestStore(t, "layers.db")

	if err := store.AddToQueueWithDepth([]string{"https://example.com/deep"}, 2); err != nil {
		t.Fatalf("queue failed: %v", err)
	}

	item, err := store.GetNextFromQueueAtDepth(1)
	if err != nil {
		t.Fatalf("GetNextFromQueueAtDepth failed: %v", err)
	}
	if item != nil {
		t.Errorf("claimed %q from depth 1, want nothing (it is queued at depth 2)", item.URL)
	}

	depth, err := store.MinUnfinishedDepth(3)
	if err != nil {
		t.Fatalf("MinUnfinishedDepth failed: %v", err)
	}
	if depth == nil || *depth != 2 {
		t.Errorf("MinUnfinishedDepth = %v, want 2", depth)
	}
}

// A layer counts as busy while a page is in flight, not just while pages are
// waiting: a page being fetched can still add children.
func TestHasQueuedItemsAtDepthCountsProcessing(t *testing.T) {
	store := newTestStore(t, "processing.db")

	if err := store.AddToQueueWithDepth([]string{"https://example.com/a"}, 0); err != nil {
		t.Fatalf("queue failed: %v", err)
	}
	if _, err := store.GetNextFromQueueAtDepth(0); err != nil {
		t.Fatalf("claim failed: %v", err)
	}

	busy, err := store.HasQueuedItemsAtDepth(0)
	if err != nil {
		t.Fatalf("HasQueuedItemsAtDepth failed: %v", err)
	}
	if !busy {
		t.Error("a layer with a page in flight must count as unfinished")
	}
}

// Only queued work without a depth blocks a bounded run; link-graph nodes
// legitimately have none.
func TestHasDepthlessWork(t *testing.T) {
	t.Run("queued row without depth blocks", func(t *testing.T) {
		store := newTestStore(t, "legacy.db")
		if err := store.AddToQueue([]string{"https://example.com/queued"}); err != nil {
			t.Fatalf("queue failed: %v", err)
		}

		blocked, err := store.HasDepthlessWork(3)
		if err != nil {
			t.Fatalf("HasDepthlessWork failed: %v", err)
		}
		if !blocked {
			t.Error("a pending row with no depth must be reported")
		}
	})

	t.Run("discovered row without depth does not block", func(t *testing.T) {
		store := newTestStore(t, "discovered.db")
		if err := store.SaveLinks([]*crawler.LinkData{{
			SourceURL: "https://example.com/source",
			TargetURL: "https://example.com/target",
			LinkType:  "internal",
		}}); err != nil {
			t.Fatalf("failed to save link: %v", err)
		}

		blocked, err := store.HasDepthlessWork(3)
		if err != nil {
			t.Fatalf("HasDepthlessWork failed: %v", err)
		}
		if blocked {
			t.Error("link-graph nodes carry no depth by design and must not block a bounded run")
		}
	})
}

// The depth column has to be added after the status rebuild, not before: the
// rebuild copies a fixed column list that predates depth, so running it against
// a table that already had the column would be fine, but adding the column to
// that list would make the rebuild select a column the legacy table lacks.
func TestMigrationAddsDepthAfterStatusRebuild(t *testing.T) {
	dbFile := filepath.Join(t.TempDir(), "migrate.db")

	legacySchema := strings.Replace(schemaSQL, ", 'discovered'", "", 1)
	if legacySchema == schemaSQL {
		t.Fatal("failed to derive legacy schema; marker not found")
	}
	// Strip the depth column and the index that reads it, so the fixture is a
	// database from before depth tracking existed.
	legacySchema = strings.Replace(legacySchema, "    depth INTEGER,\n", "", 1)
	legacySchema = strings.Replace(legacySchema,
		"CREATE INDEX IF NOT EXISTS idx_pages_status_depth ON pages(status, depth);\n", "", 1)
	if strings.Contains(legacySchema, "depth INTEGER") || strings.Contains(legacySchema, "idx_pages_status_depth") {
		t.Fatal("failed to derive a legacy schema without the depth column")
	}

	store, err := NewSQLiteStorage(dbFile)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = store.Close() }()

	if _, err := store.db.Exec("DROP VIEW IF EXISTS links; DROP VIEW IF EXISTS completed_pages; DROP VIEW IF EXISTS queue_status; DROP TABLE IF EXISTS link_relations; DROP TABLE pages;"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if _, err := store.db.Exec(legacySchema); err != nil {
		t.Fatalf("legacy schema: %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO pages (url, status) VALUES ('https://example.com/queued', 'pending')`); err != nil {
		t.Fatalf("seed legacy row: %v", err)
	}

	if err := store.InitSchema(); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	has, err := store.pagesHasColumn("depth")
	if err != nil {
		t.Fatalf("column check failed: %v", err)
	}
	if !has {
		t.Fatal("depth column was not added")
	}

	// The pre-existing row keeps its data and gains a NULL depth, which is what
	// blocks a bounded run against this database.
	if _, recorded := depthOfURL(t, store, "https://example.com/queued"); recorded {
		t.Error("a row that predates depth tracking must read as NULL, not 0")
	}
	blocked, err := store.HasDepthlessWork(3)
	if err != nil {
		t.Fatalf("HasDepthlessWork failed: %v", err)
	}
	if !blocked {
		t.Error("a migrated legacy queue must block a bounded run")
	}

	// Running it again changes nothing.
	if err := store.InitSchema(); err != nil {
		t.Fatalf("second migration failed: %v", err)
	}
}

// A layer whose only remaining work is a retryable failure is still
// unfinished. Reporting a deeper pending layer instead would let a resume step
// over it and break the barrier.
func TestMinUnfinishedDepthIncludesRetryableErrors(t *testing.T) {
	store := newTestStore(t, "unfinished.db")

	if err := store.AddToQueueWithDepth([]string{"https://example.com/shallow"}, 0); err != nil {
		t.Fatalf("queue failed: %v", err)
	}
	item, err := store.GetNextFromQueueAtDepth(0)
	if err != nil || item == nil {
		t.Fatalf("claim failed: (%v, %v)", item, err)
	}
	if err := store.SaveFailedAttempt(item.ID, nil, "network_error", "boom", time.Time{}); err != nil {
		t.Fatalf("failed to record error: %v", err)
	}

	if err := store.AddToQueueWithDepth([]string{"https://example.com/deep"}, 1); err != nil {
		t.Fatalf("queue failed: %v", err)
	}

	depth, err := store.MinUnfinishedDepth(3)
	if err != nil {
		t.Fatalf("MinUnfinishedDepth failed: %v", err)
	}
	if depth == nil || *depth != 0 {
		t.Errorf("MinUnfinishedDepth = %v, want 0 (the retryable failure is unfinished work)", depth)
	}

	// Once its retries are spent the layer is finished and the deeper one is next.
	depth, err = store.MinUnfinishedDepth(1)
	if err != nil {
		t.Fatalf("MinUnfinishedDepth failed: %v", err)
	}
	if depth == nil || *depth != 1 {
		t.Errorf("MinUnfinishedDepth = %v, want 1 once depth 0 has no retries left", depth)
	}
}

// A failure with retries left is unfinished work, so a bounded run must refuse
// it too when its depth was never recorded — otherwise the bound would quietly
// skip pages an unbounded run had queued.
func TestHasDepthlessWorkIncludesRetryableErrors(t *testing.T) {
	store := newTestStore(t, "depthless-retry.db")

	if err := store.AddToQueue([]string{"https://example.com/page"}); err != nil {
		t.Fatalf("queue failed: %v", err)
	}
	item, err := store.GetNextFromQueue()
	if err != nil || item == nil {
		t.Fatalf("claim failed: (%v, %v)", item, err)
	}
	if err := store.SaveFailedAttempt(item.ID, nil, "network_error", "boom", time.Time{}); err != nil {
		t.Fatalf("failed to record error: %v", err)
	}

	// Nothing is queued any more, but the retry is still owed.
	pending, processing, _, _, err := store.GetQueueStatus()
	if err != nil {
		t.Fatalf("queue status failed: %v", err)
	}
	if pending+processing != 0 {
		t.Fatalf("test setup: expected an empty queue, got %d pending and %d processing", pending, processing)
	}

	blocked, err := store.HasDepthlessWork(3)
	if err != nil {
		t.Fatalf("HasDepthlessWork failed: %v", err)
	}
	if !blocked {
		t.Error("a retryable failure with no depth must block a bounded run")
	}

	// Once its budget is spent it is finished, and no longer blocks.
	blocked, err = store.HasDepthlessWork(1)
	if err != nil {
		t.Fatalf("HasDepthlessWork failed: %v", err)
	}
	if blocked {
		t.Error("a failure with no retries left is finished and must not block")
	}
}
