package crawler_test

import (
	"database/sql"
)

// storageQuery and storageDepth read the pages table directly. The Storage
// interface deliberately exposes no general query method, and these assertions
// are about what was persisted rather than about crawler behaviour.

func storageQuery(dbPath, query string) ([]string, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = db.Close() }()

	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var values []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		values = append(values, v)
	}
	return values, rows.Err()
}

func storageDepth(dbPath, url string) (int, bool, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return 0, false, err
	}
	defer func() { _ = db.Close() }()

	var depth sql.NullInt64
	err = db.QueryRow("SELECT depth FROM pages WHERE url = ?", url).Scan(&depth)
	if err == sql.ErrNoRows {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	if !depth.Valid {
		return 0, false, nil
	}
	return int(depth.Int64), true, nil
}
