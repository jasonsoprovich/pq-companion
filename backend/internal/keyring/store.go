package keyring

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Entry is one row of keyring_entries — the persisted "this character has
// this key on their keyring as of the last /keys observation" record.
type Entry struct {
	Character   string `json:"character"`
	KeyItem     int    `json:"key_item"`
	FirstSeenAt int64  `json:"first_seen_at"`
	LastSeenAt  int64  `json:"last_seen_at"`
}

// Store persists per-character keyring snapshots in user.db.
type Store struct {
	db *sql.DB
}

// OpenStore opens user.db and runs the keyring_entries migration.
// Coexists with players / character / trigger / backup stores under WAL mode.
func OpenStore(path string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(30000)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open user.db: %w", err)
	}
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping user.db: %w", err)
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate user.db: %w", err)
	}
	return s, nil
}

// Close releases the underlying connection.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS keyring_entries (
			character     TEXT    NOT NULL,
			key_item      INTEGER NOT NULL,
			first_seen_at INTEGER NOT NULL,
			last_seen_at  INTEGER NOT NULL,
			PRIMARY KEY (character, key_item)
		)
	`); err != nil {
		return err
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS keyring_entries_character ON keyring_entries(character)`); err != nil {
		return err
	}
	return nil
}

// Snapshot replaces the character's keyring contents with the supplied set of
// key item IDs. Items present in the new set are upserted (first_seen_at
// preserved on existing rows, last_seen_at always bumped); rows for this
// character whose key_item isn't in the new set are deleted — the character
// no longer carries that key, so it shouldn't show as owned.
//
// Empty keyItems is treated as "this character has nothing on their
// keyring": we don't currently reach this path because the burst detector
// only flushes when ≥1 line was matched, but we still guard against the
// empty case for safety.
func (s *Store) Snapshot(character string, keyItems []int, observedAt time.Time) error {
	if character == "" {
		return fmt.Errorf("character required")
	}
	if observedAt.IsZero() {
		observedAt = time.Now()
	}
	now := observedAt.Unix()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	// Delete rows for this character whose key_item is not in the new set.
	// SQLite has no array binding, so build the placeholder list manually.
	// keyItems is bounded by the keyring_data master list (~38), so the
	// query stays tiny.
	if len(keyItems) == 0 {
		if _, err := tx.Exec(`DELETE FROM keyring_entries WHERE character = ?`, character); err != nil {
			return fmt.Errorf("delete all entries for %q: %w", character, err)
		}
	} else {
		placeholders := strings.Repeat("?,", len(keyItems))
		placeholders = placeholders[:len(placeholders)-1] // drop trailing comma
		args := make([]any, 0, len(keyItems)+1)
		args = append(args, character)
		for _, id := range keyItems {
			args = append(args, id)
		}
		q := fmt.Sprintf(`DELETE FROM keyring_entries WHERE character = ? AND key_item NOT IN (%s)`, placeholders)
		if _, err := tx.Exec(q, args...); err != nil {
			return fmt.Errorf("delete stale entries for %q: %w", character, err)
		}
	}

	// Upsert each item. On conflict, only last_seen_at changes — first_seen_at
	// stays put so the UI can show how long the character has had a key.
	for _, id := range keyItems {
		if _, err := tx.Exec(`
			INSERT INTO keyring_entries (character, key_item, first_seen_at, last_seen_at)
			VALUES (?, ?, ?, ?)
			ON CONFLICT(character, key_item) DO UPDATE SET last_seen_at = excluded.last_seen_at
		`, character, id, now, now); err != nil {
			return fmt.Errorf("upsert entry char=%q item=%d: %w", character, id, err)
		}
	}

	return tx.Commit()
}

// ListByCharacter returns every keyring entry for the named character,
// newest first. Empty slice (not nil) is returned when there are no rows.
func (s *Store) ListByCharacter(character string) ([]Entry, error) {
	rows, err := s.db.Query(`
		SELECT character, key_item, first_seen_at, last_seen_at
		FROM keyring_entries
		WHERE character = ?
		ORDER BY last_seen_at DESC
	`, character)
	if err != nil {
		return nil, fmt.Errorf("query keyring entries for %q: %w", character, err)
	}
	defer rows.Close()
	out := []Entry{}
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.Character, &e.KeyItem, &e.FirstSeenAt, &e.LastSeenAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Characters returns the distinct character names with at least one keyring
// entry, in alphabetical order. Used by the UI to render per-character tabs.
func (s *Store) Characters() ([]string, error) {
	rows, err := s.db.Query(`SELECT DISTINCT character FROM keyring_entries ORDER BY character COLLATE NOCASE`)
	if err != nil {
		return nil, fmt.Errorf("query distinct characters: %w", err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}
