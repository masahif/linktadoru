package storage

import (
	"database/sql"
	"testing"
	"time"

	"github.com/masahif/linktadoru/internal/crawler"
)

func TestQueueClaimsDepthThenAdmissionOrder(t *testing.T) {
	store := newTempStorage(t)
	rows, err := store.db.Query(`PRAGMA index_info(idx_pages_status_depth_added_id)`)
	if err != nil {
		t.Fatal(err)
	}
	var indexedColumns []string
	for rows.Next() {
		var sequence, columnID int
		var name string
		if err := rows.Scan(&sequence, &columnID, &name); err != nil {
			t.Fatal(err)
		}
		indexedColumns = append(indexedColumns, name)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	wantIndex := []string{"status", "depth", "added_at", "id"}
	if len(indexedColumns) != len(wantIndex) {
		t.Fatalf("queue index columns = %v, want %v", indexedColumns, wantIndex)
	}
	for i := range wantIndex {
		if indexedColumns[i] != wantIndex[i] {
			t.Fatalf("queue index columns = %v, want %v", indexedColumns, wantIndex)
		}
	}

	if err := store.AddToQueue([]string{"https://example.com/deep"}, 2); err != nil {
		t.Fatal(err)
	}
	if err := store.AddToQueue([]string{"https://example.com/z", "https://example.com/a"}, 1); err != nil {
		t.Fatal(err)
	}

	for i, want := range []string{"https://example.com/z", "https://example.com/a", "https://example.com/deep"} {
		item, err := store.GetNextFromQueue()
		if err != nil {
			t.Fatal(err)
		}
		if item == nil || item.URL != want {
			t.Fatalf("claim %d = %+v, want %s", i, item, want)
		}
	}
}

func TestNormalDuplicateKeepsFirstDepthAndSeedRefreshResetsRow(t *testing.T) {
	store := newTempStorage(t)
	const pageURL = "https://example.com/page"
	if err := store.AddToQueue([]string{pageURL}, 2); err != nil {
		t.Fatal(err)
	}
	item, err := store.GetNextFromQueue()
	if err != nil || item == nil {
		t.Fatalf("claim: item=%+v err=%v", item, err)
	}
	page := &crawler.PageData{
		URL:          pageURL,
		StatusCode:   503,
		Title:        "stale",
		TTFB:         time.Second,
		DownloadTime: 2 * time.Second,
		ResponseSize: 42,
		HTTPHeaders:  map[string]string{"server": "old"},
		CrawledAt:    time.Now(),
	}
	if err := store.SavePageResponseError(item.ID, page, "http_transient", "HTTP 503"); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveLinks([]*crawler.LinkData{{
		SourceURL: pageURL,
		TargetURL: "https://example.com/old-child",
		LinkType:  "internal",
		CrawledAt: time.Now(),
	}}); err != nil {
		t.Fatal(err)
	}

	// Normal rediscovery never rewrites a terminal row or its first depth.
	if err := store.AddToQueue([]string{pageURL}, 1); err != nil {
		t.Fatal(err)
	}
	var status string
	var depth, retryCount int
	if err := store.db.QueryRow(`SELECT status, depth, retry_count FROM pages WHERE url = ?`, pageURL).Scan(&status, &depth, &retryCount); err != nil {
		t.Fatal(err)
	}
	if status != "error" || depth != 2 || retryCount != 1 {
		t.Fatalf("normal duplicate = status %s depth %d retry %d", status, depth, retryCount)
	}

	if err := store.AddSeeds([]string{pageURL}); err != nil {
		t.Fatal(err)
	}
	var allObservationsCleared bool
	if err := store.db.QueryRow(`
		SELECT status, depth, retry_count,
			processing_started_at IS NULL
			AND status_code IS NULL
			AND title IS NULL
			AND ttfb_ms IS NULL
			AND response_http_headers IS NULL
			AND crawled_at IS NULL
			AND last_error_type IS NULL
			AND last_error_message IS NULL
		FROM pages WHERE url = ?
	`, pageURL).Scan(&status, &depth, &retryCount, &allObservationsCleared); err != nil {
		t.Fatal(err)
	}
	if status != "pending" || depth != 0 || retryCount != 0 || !allObservationsCleared {
		t.Fatalf("seed refresh = status %s depth %d retry %d cleared=%v", status, depth, retryCount, allObservationsCleared)
	}
	var edges int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM link_relations WHERE source_page_id = ?`, item.ID).Scan(&edges); err != nil {
		t.Fatal(err)
	}
	if edges != 1 {
		t.Fatalf("seed refresh removed %d historical outgoing edges, want 1 retained", 1-edges)
	}
}

func TestReseedingPreservesUnfetchedSeedPriority(t *testing.T) {
	store := newTempStorage(t)
	seeds := []string{
		"https://example.com/a",
		"https://example.com/b",
		"https://example.com/c",
	}
	if err := store.AddSeeds(seeds); err != nil {
		t.Fatal(err)
	}

	for run, want := range []string{seeds[0], seeds[1], seeds[2]} {
		if run > 0 {
			if err := store.AddSeeds(seeds); err != nil {
				t.Fatal(err)
			}
		}
		item, err := store.GetNextFromQueue()
		if err != nil {
			t.Fatal(err)
		}
		if item == nil || item.URL != want {
			t.Fatalf("run %d claimed %+v, want %s", run+1, item, want)
		}
		if err := store.SavePageResult(item.ID, &crawler.PageData{
			URL:         item.URL,
			HTTPHeaders: map[string]string{},
			CrawledAt:   time.Now(),
		}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLegacyDepthValidationAndNamedSeedRescue(t *testing.T) {
	store := newTempStorage(t)
	const named = "https://example.com/named"
	if _, err := store.db.Exec(`INSERT INTO pages (url, status, depth) VALUES (?, 'pending', NULL)`, named); err != nil {
		t.Fatal(err)
	}
	if err := store.ValidateDepthTracking(nil); err == nil {
		t.Fatal("depthless pending row was accepted")
	}
	if err := store.ValidateDepthTracking([]string{named}); err != nil {
		t.Fatalf("named seed was not exempted: %v", err)
	}
	if err := store.AddSeeds([]string{named}); err != nil {
		t.Fatalf("named seed rescue failed: %v", err)
	}
	var depth int
	if err := store.db.QueryRow(`SELECT depth FROM pages WHERE url = ?`, named).Scan(&depth); err != nil {
		t.Fatal(err)
	}
	if depth != 0 {
		t.Fatalf("rescued depth = %d, want 0", depth)
	}
}

func TestLegacyDepthValidationCoversEveryUnfinishedStatus(t *testing.T) {
	tests := []struct {
		name      string
		status    string
		errorType any
		retries   int
	}{
		{name: "pending", status: "pending"},
		{name: "processing", status: "processing"},
		{name: "retryable error", status: "error", errorType: "network_error", retries: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newTempStorage(t)
			if _, err := store.db.Exec(`
				INSERT INTO pages (url, status, depth, last_error_type, retry_count)
				VALUES (?, ?, NULL, ?, ?)
			`, "https://example.com/legacy", tt.status, tt.errorType, tt.retries); err != nil {
				t.Fatal(err)
			}
			if err := store.ValidateDepthTracking(nil); err == nil {
				t.Fatalf("depthless %s row was accepted", tt.status)
			}
		})
	}
}

func TestSeedTransactionFailsBeforeMutatingWhenOtherLegacyWorkExists(t *testing.T) {
	store := newTempStorage(t)
	const named = "https://example.com/named"
	const other = "https://example.com/other"
	if _, err := store.db.Exec(`
		INSERT INTO pages (url, status, depth, status_code)
		VALUES (?, 'pending', NULL, 200), (?, 'pending', NULL, NULL)
	`, named, other); err != nil {
		t.Fatal(err)
	}
	if err := store.AddSeeds([]string{named}); err == nil {
		t.Fatal("seed transaction accepted unrelated depthless work")
	}
	var depth sql.NullInt64
	var statusCode sql.NullInt64
	if err := store.db.QueryRow(`SELECT depth, status_code FROM pages WHERE url = ?`, named).Scan(&depth, &statusCode); err != nil {
		t.Fatal(err)
	}
	if depth.Valid || !statusCode.Valid || statusCode.Int64 != 200 {
		t.Fatalf("failed seed transaction mutated row: depth=%v status_code=%v", depth, statusCode)
	}
}

func TestStaleProcessingCleanupPreservesDepthAndRetryCount(t *testing.T) {
	store := newTempStorage(t)
	const pageURL = "https://example.com/interrupted"
	if err := store.AddToQueue([]string{pageURL}, 2); err != nil {
		t.Fatal(err)
	}
	item, err := store.GetNextFromQueue()
	if err != nil || item == nil {
		t.Fatalf("claim: item=%+v err=%v", item, err)
	}
	if _, err := store.db.Exec(`UPDATE pages SET retry_count = 1 WHERE id = ?`, item.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.CleanupStaleProcessing(0); err != nil {
		t.Fatal(err)
	}

	var status string
	var depth, retries int
	if err := store.db.QueryRow(`
		SELECT status, depth, retry_count FROM pages WHERE id = ?
	`, item.ID).Scan(&status, &depth, &retries); err != nil {
		t.Fatal(err)
	}
	if status != "pending" || depth != 2 || retries != 1 {
		t.Fatalf("cleaned row = status %s depth %d retries %d", status, depth, retries)
	}
}
