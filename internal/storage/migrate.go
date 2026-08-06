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
// This list is deliberately frozen at the pre-depth column set. The columns
// added since — 'depth' by migratePagesAddDepth and 'retry_after' by
// migratePagesAddRetryAfter — arrive through ALTER TABLE, so a database that
// predates them does not have them, and "SELECT ... depth ..." against it would
// fail — which is exactly what the rebuild below does. Ordering saves us here:
// the rebuild runs first, then the ALTERs.
//
// Upgrade path: if a future migration also needs to rebuild the table, this
// fixed list must be replaced by the intersection of the real columns (PRAGMA
// table_info) and the target columns FIRST. Otherwise the rebuild silently
// drops the values of every ALTER-added column — 'depth' and 'retry_after'
// would survive as columns but come back NULL for every row.
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

// migratePagesAddRetryAfter adds the nullable 'retry_after' column to an
// existing pages table, on the same terms as depth above: idempotent, no table
// rewrite, and NULL on existing rows — which reads correctly as "no wait
// recorded", so a resumed database retries its old failures immediately rather
// than sitting on a pause nobody asked for.
//
// Must run AFTER migratePagesAddDiscovered, for the reason given there.
func (s *SQLiteStorage) migratePagesAddRetryAfter() error {
	return s.addColumnIfMissing("pages", "retry_after", "DATETIME")
}

// migrateCrawlErrorsAddAttemptDetail adds the per-attempt columns to an
// existing crawl_errors table. Old rows keep NULL for both, which is honest:
// those attempts were recorded before the crawler tracked which attempt they
// were or what status they saw.
func (s *SQLiteStorage) migrateCrawlErrorsAddAttemptDetail() error {
	if err := s.addColumnIfMissing("crawl_errors", "status_code", "INTEGER"); err != nil {
		return err
	}
	return s.addColumnIfMissing("crawl_errors", "attempt", "INTEGER")
}

// addColumnIfMissing adds a nullable column to a table that already exists,
// doing nothing if the column is already there or the table has yet to be
// created (a fresh database gets the column from schemaSQL).
func (s *SQLiteStorage) addColumnIfMissing(table, column, columnType string) error {
	has, err := s.tableHasColumn(table, column)
	if err != nil {
		return err
	}
	if has {
		return nil
	}

	exists, err := s.tableExists(table)
	if err != nil || !exists {
		return err
	}

	// Table and column names here are compile-time constants from this file,
	// never user input.
	stmt := fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s %s", table, column, columnType)
	if _, err := s.db.Exec(stmt); err != nil {
		return fmt.Errorf("failed to add %s.%s column: %w", table, column, err)
	}
	return nil
}

// pagesTableExists reports whether the pages table has been created yet.
func (s *SQLiteStorage) pagesTableExists() (bool, error) {
	return s.tableExists("pages")
}

// pagesHasColumn reports whether the pages table already carries a column.
func (s *SQLiteStorage) pagesHasColumn(column string) (bool, error) {
	return s.tableHasColumn("pages", column)
}

// tableExists reports whether a table has been created yet.
func (s *SQLiteStorage) tableExists(table string) (bool, error) {
	var name string
	err := s.db.QueryRow(
		"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table,
	).Scan(&name)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("failed to read %s table definition: %w", table, err)
	}
	return true, nil
}

// tableHasColumn reports whether a table already carries a column.
func (s *SQLiteStorage) tableHasColumn(table, column string) (bool, error) {
	// PRAGMA does not accept a bound parameter for the table name; the callers
	// pass compile-time constants from this file, never user input.
	rows, err := s.db.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, fmt.Errorf("failed to read %s columns: %w", table, err)
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
			return false, fmt.Errorf("failed to scan %s columns: %w", table, err)
		}
		if name == column {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, fmt.Errorf("failed to read %s columns: %w", table, err)
	}
	return false, nil
}
