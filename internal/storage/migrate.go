// Package storage — schema migrations.
//
// This file rebuilds an existing `pages` table whose CHECK constraint predates
// the 'discovered' status added for issue #46. SQLite cannot ALTER a CHECK
// constraint in place, so the table is rebuilt with the standard rename/copy
// procedure. The migration is a no-op on a fresh database (the table does not
// exist yet) and on a database already carrying the 'discovered' status.
package storage

import (
	"database/sql"
	"fmt"
	"strings"
)

// pagesBaseColumns are the non-generated columns of the pages table, in a stable
// order. Generated columns (content_type, content_length, last_modified, server,
// content_encoding, x_cache) are derived and must NOT be copied explicitly.
const pagesBaseColumns = "id, url, status, added_at, processing_started_at, " +
	"status_code, title, meta_description, meta_robots, canonical_url, " +
	"content_hash, ttfb_ms, download_time_ms, response_size_bytes, " +
	"response_http_headers, crawled_at, retry_count, last_error_type, " +
	"last_error_message"

// migratePagesAddDiscovered widens the pages.status CHECK constraint to include
// 'discovered' on databases created before issue #46. It detects the need for
// migration from the stored table DDL, then rebuilds the table preserving all
// rows and ids. Indexes and views dropped by the rebuild are recreated by the
// subsequent schemaSQL run in InitSchema.
//
// Scope: this migration is guaranteed only for databases created with the
// current released pages schema (status CHECK ... 'skipped', 'error'). It is
// intentionally conservative — it copies the fixed set of base columns
// (pagesBaseColumns) and locates the CHECK list by its exact text. Against an
// older/foreign schema where that text is absent, it ABORTS with an error
// rather than risk a lossy rebuild; the caller surfaces the error and the
// database is left untouched. Broader cross-version migration (dynamic column
// intersection, status normalisation) is out of scope here.
func (s *SQLiteStorage) migratePagesAddDiscovered() error {
	var ddl string
	err := s.db.QueryRow(
		"SELECT sql FROM sqlite_master WHERE type='table' AND name='pages'",
	).Scan(&ddl)
	if err == sql.ErrNoRows {
		return nil // fresh database — schemaSQL will create the up-to-date table
	}
	if err != nil {
		return fmt.Errorf("failed to read pages table definition: %w", err)
	}
	if strings.Contains(ddl, "'discovered'") {
		return nil // already migrated
	}

	// Build the new table DDL from the existing one so any prior columns are
	// preserved verbatim; only the table name and the CHECK list change.
	paren := strings.Index(ddl, "(")
	if paren < 0 {
		return fmt.Errorf("unexpected pages table definition: %q", ddl)
	}
	newDDL := "CREATE TABLE pages_new " + ddl[paren:]
	widened := strings.Replace(newDDL,
		"'skipped', 'error')", "'skipped', 'error', 'discovered')", 1)
	if widened == newDDL {
		return fmt.Errorf("could not locate pages status CHECK constraint to widen; aborting migration")
	}
	newDDL = widened

	// Foreign keys must be off while the referenced table is rebuilt; this PRAGMA
	// is a no-op inside a transaction, so toggle it around the transaction.
	if _, err := s.db.Exec("PRAGMA foreign_keys = OFF"); err != nil {
		return fmt.Errorf("failed to disable foreign keys: %w", err)
	}
	defer func() { _, _ = s.db.Exec("PRAGMA foreign_keys = ON") }()

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin migration transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmts := []string{
		// Drop views that reference pages; schemaSQL recreates them afterwards.
		"DROP VIEW IF EXISTS links",
		"DROP VIEW IF EXISTS completed_pages",
		"DROP VIEW IF EXISTS queue_status",
		newDDL,
		fmt.Sprintf("INSERT INTO pages_new (%s) SELECT %s FROM pages",
			pagesBaseColumns, pagesBaseColumns),
		"DROP TABLE pages",
		"ALTER TABLE pages_new RENAME TO pages",
	}
	for _, stmt := range stmts {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("migration step failed (%s): %w", stmt, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit pages migration: %w", err)
	}
	return nil
}
