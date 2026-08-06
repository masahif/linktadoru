package storage

import (
	"database/sql"
	"fmt"

	"github.com/masahif/linktadoru/internal/crawler"
)

// GetRunSummary counts the rows behind the end-of-run report.
//
// The categories exist because "error" is not one thing. A row that failed
// with a status code was answered by the server and could not be resolved; a
// row that failed without one was never reached at all. Collapsing the two —
// or worse, collapsing either into "missing" — is what makes a comparison
// between two crawls read a transient outage as a deleted page.
//
// Statuses are counted once each, so the numbers add up to the table.
func (s *SQLiteStorage) GetRunSummary() (crawler.RunSummary, error) {
	var summary crawler.RunSummary

	err := s.db.QueryRow(`
		SELECT
			COUNT(*) FILTER (WHERE status = 'completed'),
			COUNT(*) FILTER (WHERE status = 'error' AND status_code IS NOT NULL),
			COUNT(*) FILTER (WHERE status = 'error' AND status_code IS NULL),
			COUNT(*) FILTER (WHERE status = 'skipped'),
			COUNT(*) FILTER (WHERE status IN ('pending', 'processing')),
			COUNT(*) FILTER (WHERE status = 'discovered')
		FROM pages
	`).Scan(
		&summary.Completed,
		&summary.Unavailable,
		&summary.Unreachable,
		&summary.Skipped,
		&summary.Unfinished,
		&summary.Discovered,
	)
	if err != nil && err != sql.ErrNoRows {
		return summary, fmt.Errorf("failed to summarise the run: %w", err)
	}

	return summary, nil
}
