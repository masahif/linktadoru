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
//
// This list is deliberately frozen at the pre-depth column set. 'depth' is added
// by migratePagesAddDepth with ALTER TABLE, so a database that predates it does
// not have the column and "SELECT ... depth ..." against it would fail — which
// is exactly what the rebuild below does. Ordering saves us here: the rebuild
// runs first, then the ALTER.
//
// Upgrade path: if a future migration also needs to rebuild the table, this
// fixed list must be replaced by the intersection of the real columns (PRAGMA
// table_info) and the target columns FIRST. Otherwise the rebuild silently
// drops the values of every ALTER-added column — 'depth' would survive as a
// column but come back NULL for every row.
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

// migratePagesAddDepth adds the nullable 'depth' column to an existing pages
// table. It is idempotent and cheap: ALTER TABLE ADD COLUMN does not rewrite
// the table, and existing rows get NULL, which is the correct reading for them
// — their depth was never established.
//
// This must run AFTER migratePagesAddDiscovered: that migration rebuilds the
// table from its stored DDL and copies pagesBaseColumns, which does not (and
// must not) mention depth.
func (s *SQLiteStorage) migratePagesAddDepth() error {
	has, err := s.pagesHasColumn("depth")
	if err != nil {
		return err
	}
	if has {
		return nil
	}

	// A fresh database has no pages table yet; schemaSQL creates it with the
	// column already present.
	exists, err := s.pagesTableExists()
	if err != nil || !exists {
		return err
	}

	if _, err := s.db.Exec("ALTER TABLE pages ADD COLUMN depth INTEGER"); err != nil {
		return fmt.Errorf("failed to add pages.depth column: %w", err)
	}
	return nil
}

// pagesTableExists reports whether the pages table has been created yet.
func (s *SQLiteStorage) pagesTableExists() (bool, error) {
	var name string
	err := s.db.QueryRow(
		"SELECT name FROM sqlite_master WHERE type='table' AND name='pages'",
	).Scan(&name)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to read pages table definition: %w", err)
	}
	return true, nil
}

// pagesHasColumn reports whether the pages table already carries a column.
func (s *SQLiteStorage) pagesHasColumn(column string) (bool, error) {
	rows, err := s.db.Query("PRAGMA table_info(pages)")
	if err != nil {
		return false, fmt.Errorf("failed to read pages columns: %w", err)
	}
	defer func() { _ = rows.Close() }()

	for rows.Next() {
		var (
			cid        int
			name       string
			ctype      string
			notNull    int
			defaultVal sql.NullString
			pk         int
		)
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &defaultVal, &pk); err != nil {
			return false, fmt.Errorf("failed to scan pages columns: %w", err)
		}
		if name == column {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("failed to read pages columns: %w", err)
	}
	return false, nil
}
