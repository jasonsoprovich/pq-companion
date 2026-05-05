// Package db provides read-only access to the PQ Companion SQLite database.
package db

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// DB wraps a sql.DB connection to the quarm.db SQLite database.
type DB struct {
	*sql.DB
}

// Open opens the SQLite database at the given path in read-only mode.
//
// quarm.db is never written to at runtime (it's regenerated offline by
// the dbconvert tool), so journal mode doesn't matter here — we only need
// busy_timeout in case a checkpoint/file-replace ever races a query.
//
// modernc.org/sqlite expects PRAGMAs via the _pragma=NAME(VALUE) URI form;
// mattn-style _busy_timeout was silently ignored before this change.
func Open(path string) (*DB, error) {
	dsn := fmt.Sprintf("file:%s?mode=ro&_pragma=busy_timeout(5000)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	// Single reader connection is sufficient for a read-only DB.
	db.SetMaxOpenConns(1)
	return &DB{db}, nil
}
