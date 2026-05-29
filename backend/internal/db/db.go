// Package db provides read-only access to the PQ Companion SQLite database.
package db

import (
	"database/sql"
	"fmt"
	"sync"

	_ "modernc.org/sqlite"
)

// DB wraps a sql.DB connection to the quarm.db SQLite database.
type DB struct {
	*sql.DB

	// Duplicate-name collapse indexes, computed once from the loaded DB on
	// first use (see variants.go). The EQMacEmu content dump ships multiple
	// rows per item/spell name with different ids; these indexes pick a
	// canonical row per name for list/search views and expose the rest as
	// fetchable "variants." Computed lazily so opening the DB stays cheap and
	// so tests that never touch items/spells don't pay for it.
	variantsOnce  sync.Once
	itemVariants  *variantIndex
	spellVariants *variantIndex
}

// Open opens the SQLite database at the given path in read-only mode.
//
// quarm.db is never written to at runtime (it's regenerated offline by
// the dbconvert tool), so journal mode doesn't matter here — we only need
// busy_timeout in case a checkpoint/file-replace ever races a query.
//
// immutable=1 is required so the file opens cleanly when installed under
// a non-writable directory (e.g. C:\Program Files). The database ships in
// WAL mode, and without immutable SQLite would try to create the -shm
// shared-memory file alongside the DB on first query, failing with
// SQLITE_CANTOPEN (14) for standard Windows accounts.
//
// modernc.org/sqlite expects PRAGMAs via the _pragma=NAME(VALUE) URI form;
// mattn-style _busy_timeout was silently ignored before this change.
func Open(path string) (*DB, error) {
	dsn := fmt.Sprintf("file:%s?mode=ro&immutable=1&_pragma=busy_timeout(5000)", path)
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
	return &DB{DB: db}, nil
}
