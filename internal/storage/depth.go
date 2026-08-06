package storage

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/masahif/linktadoru/internal/crawler"
)

// Depth-aware queue operations, used only by a bounded crawl (--max-depth > 0).
//
// They live beside the existing unbounded queue methods rather than replacing
// them: an unbounded run must keep the behaviour it has always had, and the
// depth column it writes would be a first-discovery approximation rather than
// the shortest path, so it stays NULL there.

// AddToQueueWithDepth queues URLs at a known depth, recording that depth on the
// row. A URL that exists only as a 'discovered' link-graph node is promoted to
// 'pending' and takes the depth it is promoted at.
//
// Under the layer barrier a URL is promoted exactly once, at the first layer
// that reaches it, and that layer is by construction the shortest one — so the
// depth written here is final and needs no later relaxation.
func (s *SQLiteStorage) AddToQueueWithDepth(urls []string, depth int) error {
	if len(urls) == 0 {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt, err := tx.Prepare(`
		INSERT INTO pages (url, status, added_at, depth)
		VALUES (?, 'pending', ?, ?)
		ON CONFLICT(url) DO UPDATE SET
			status = 'pending',
			added_at = excluded.added_at,
			depth = excluded.depth
		WHERE pages.status = 'discovered'
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer func() { _ = stmt.Close() }()

	now := sqlTime(time.Now())
	for _, url := range urls {
		if _, err := stmt.Exec(url, now, depth); err != nil {
			return fmt.Errorf("failed to insert URL %s: %w", url, err)
		}
	}

	return tx.Commit()
}

// GetNextFromQueueByDepthPriority atomically claims the shallowest pending URL.
//
// It prefers depth 0 but only looks at 'pending' rows, so a seed still being
// fetched is not a candidate rather than a blocker — that is the head-of-line
// blocking this mode exists to avoid. url makes the order total: every URL in
// one batch shares an added_at, and SQLite's tie-break among equal keys is
// unspecified.
func (s *SQLiteStorage) GetNextFromQueueByDepthPriority() (*crawler.URLItem, error) {
	var item crawler.URLItem

	err := s.db.QueryRow(`
		UPDATE pages
		SET status = 'processing', processing_started_at = ?
		WHERE id = (
			SELECT id FROM pages
			WHERE status = 'pending'
			ORDER BY depth ASC, added_at ASC, url ASC
			LIMIT 1
		) AND status = 'pending'
		RETURNING id, url, depth, retry_count
	`, sqlTime(time.Now())).Scan(&item.ID, &item.URL, &item.Depth, &item.RetryCount)

	if err == sql.ErrNoRows {
		return nil, nil // nothing pending
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get next from queue by depth priority: %w", err)
	}

	return &item, nil
}

// GetNextFromQueueAtDepth atomically claims the next pending URL at exactly the
// given depth. Restricting the claim to one layer is what implements the
// barrier: a worker cannot start on depth d+1 while depth d is still open.
func (s *SQLiteStorage) GetNextFromQueueAtDepth(depth int) (*crawler.URLItem, error) {
	var item crawler.URLItem

	err := s.db.QueryRow(`
		UPDATE pages
		SET status = 'processing', processing_started_at = ?
		WHERE id = (
			SELECT id FROM pages
			WHERE status = 'pending' AND depth = ?
			ORDER BY added_at ASC
			LIMIT 1
		) AND status = 'pending'
		RETURNING id, url, depth, retry_count
	`, sqlTime(time.Now()), depth).Scan(&item.ID, &item.URL, &item.Depth, &item.RetryCount)

	if err == sql.ErrNoRows {
		return nil, nil // no work at this depth
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get next from queue at depth %d: %w", depth, err)
	}

	return &item, nil
}

// MinUnfinishedDepth returns the shallowest depth that still has work, or nil
// when the crawl is done. Rows with a NULL depth are not part of a bounded
// crawl and are ignored.
//
// "Work" is deliberately wider than the pending queue: a page sitting in
// 'error' with retries left is unfinished too. Counting only pending rows would
// break the barrier across a resume — a run cancelled (or stopped by the page
// limit) before its retries could leave a shallow failure behind, and the next
// run would step over it into a deeper layer, expanding pages before a
// shallower path to them had been established.
func (s *SQLiteStorage) MinUnfinishedDepth(maxRetries int) (*int, error) {
	var depth sql.NullInt64
	err := s.db.QueryRow(`
		SELECT MIN(depth) FROM pages
		WHERE depth IS NOT NULL
		  AND (
			status = 'pending'
			OR (status = 'error' AND retry_count < ?
			    AND last_error_type IN (SELECT value FROM json_each(?)))
		  )
	`, maxRetries, retryableErrorTypesJSON).Scan(&depth)
	if err != nil {
		return nil, fmt.Errorf("failed to get minimum unfinished depth: %w", err)
	}
	if !depth.Valid {
		return nil, nil
	}

	d := int(depth.Int64)
	return &d, nil
}

// HasQueuedItemsAtDepth reports whether a layer still holds work. It counts
// 'processing' as well as 'pending': a layer is only finished once the rows
// being fetched right now have landed, otherwise the next layer would open
// while the current one is still producing children.
func (s *SQLiteStorage) HasQueuedItemsAtDepth(depth int) (bool, error) {
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM pages
		WHERE status IN ('pending', 'processing') AND depth = ?
	`, depth).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check queued items at depth %d: %w", depth, err)
	}
	return count > 0, nil
}

// HasRetryablePagesAtDepth reports whether a layer still has failures worth
// retrying. The layer is not finished until this is false: a page that only
// succeeds on its second attempt still has children, and they belong to the
// next layer, not to some later point after the whole crawl has moved on.
func (s *SQLiteStorage) HasRetryablePagesAtDepth(maxRetries, depth int) (bool, error) {
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM pages
		WHERE status = 'error'
		  AND depth = ?
		  AND retry_count < ?
		  AND last_error_type IN (SELECT value FROM json_each(?))
	`, depth, maxRetries, retryableErrorTypesJSON).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("failed to check retryable pages at depth %d: %w", depth, err)
	}
	return count > 0, nil
}

// RequeueErrorPagesAtDepth moves this layer's retryable failures back to
// 'pending' so the layer's workers pick them up again. Rows still waiting out
// a Retry-After are left alone; see EarliestRetryTimeAtDepth.
func (s *SQLiteStorage) RequeueErrorPagesAtDepth(maxRetries, depth int) (int, error) {
	result, err := s.db.Exec(`
		UPDATE pages
		SET status = 'pending', processing_started_at = NULL
		WHERE status = 'error'
		  AND depth = ?
		  AND retry_count < ?
		  AND last_error_type IN (SELECT value FROM json_each(?))
		  AND (retry_after IS NULL OR retry_after <= ?)
	`, depth, maxRetries, retryableErrorTypesJSON, sqlTime(time.Now()))
	if err != nil {
		return 0, fmt.Errorf("failed to requeue error pages at depth %d: %w", depth, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get affected rows: %w", err)
	}

	return int(rowsAffected), nil
}

// EarliestRetryTimeAtDepth is EarliestRetryTime scoped to one layer.
func (s *SQLiteStorage) EarliestRetryTimeAtDepth(maxRetries, depth int) (*time.Time, error) {
	return s.earliestRetryTime(`
		SELECT MIN(COALESCE(retry_after, '')) FROM pages
		WHERE status = 'error'
		  AND depth = ?
		  AND retry_count < ?
		  AND last_error_type IN (SELECT value FROM json_each(?))
	`, depth, maxRetries, retryableErrorTypesJSON)
}

// HasDepthlessWork reports whether the database holds unfinished work whose
// depth was never established: rows left behind by a run without --max-depth,
// or by a release that predates the depth column.
//
// "Unfinished" matches what MinUnfinishedDepth counts — pending, in flight, or
// failed with retries left. A retryable failure has to be included: it is work
// the crawler would still do, and skipping it silently would leave a bounded
// run quietly ignoring pages an unbounded run had queued.
//
// 'discovered' rows are deliberately not counted. They carry NULL as a matter
// of course — depth is assigned when they are promoted — so counting them would
// reject every healthy database.
func (s *SQLiteStorage) HasDepthlessWork(maxRetries int) (bool, error) {
	var exists int
	err := s.db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM pages
			WHERE depth IS NULL
			  AND (
				status IN ('pending', 'processing')
				OR (status = 'error' AND retry_count < ?
				    AND last_error_type IN (SELECT value FROM json_each(?)))
			  )
		)
	`, maxRetries, retryableErrorTypesJSON).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check for work without depth: %w", err)
	}
	return exists == 1, nil
}

// HasResumableWork reports whether a database still has anything to crawl:
// queued or in-flight pages, or failures with retries left.
//
// The CLI uses this to decide whether a run with no seed URLs has work to
// resume. Asking only about the queue would end a run that still had retries
// pending — the crawler would have processed them, but it never gets the
// chance.
func (s *SQLiteStorage) HasResumableWork(maxRetries int) (bool, error) {
	var exists int
	err := s.db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM pages
			WHERE status IN ('pending', 'processing')
			   OR (status = 'error' AND retry_count < ?
			       AND last_error_type IN (SELECT value FROM json_each(?)))
		)
	`, maxRetries, retryableErrorTypesJSON).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("failed to check for resumable work: %w", err)
	}
	return exists == 1, nil
}
