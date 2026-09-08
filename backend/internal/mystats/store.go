package mystats

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// StoredSnapshot is one persisted `/mystats` capture for a character.
type StoredSnapshot struct {
	ID         int64    `json:"id"`
	Character  string   `json:"character"`
	CapturedAt int64    `json:"captured_at"` // unix seconds
	Raw        string   `json:"raw"`         // the original block text
	Snapshot   Snapshot `json:"snapshot"`
}

// Store persists stat snapshots in user.db.
type Store struct {
	db *sql.DB
}

// OpenStore opens user.db at path and applies the snapshot migration.
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
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS character_stat_snapshots (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			character   TEXT    NOT NULL COLLATE NOCASE,
			captured_at INTEGER NOT NULL,
			raw_text    TEXT    NOT NULL,
			parsed_json TEXT    NOT NULL
		)`)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
		CREATE INDEX IF NOT EXISTS character_stat_snapshots__char_time
		ON character_stat_snapshots (character, captured_at DESC)`)
	return err
}

// Insert stores a snapshot. It is a no-op (returns id 0, false) when the given
// raw block is byte-identical to the character's most recent stored snapshot —
// this drops the accidental `/mystats` `/mystats` double-tap without blocking a
// deliberate re-capture later with changed gear.
func (s *Store) Insert(character string, capturedAt int64, raw string, snap Snapshot) (int64, bool, error) {
	if character == "" {
		return 0, false, nil
	}
	var lastRaw string
	err := s.db.QueryRow(
		`SELECT raw_text FROM character_stat_snapshots
		 WHERE character = ? COLLATE NOCASE ORDER BY captured_at DESC, id DESC LIMIT 1`,
		character,
	).Scan(&lastRaw)
	if err != nil && err != sql.ErrNoRows {
		return 0, false, err
	}
	if err == nil && lastRaw == raw {
		return 0, false, nil
	}

	blob, err := json.Marshal(snap)
	if err != nil {
		return 0, false, err
	}
	res, err := s.db.Exec(
		`INSERT INTO character_stat_snapshots (character, captured_at, raw_text, parsed_json)
		 VALUES (?, ?, ?, ?)`,
		character, capturedAt, raw, string(blob),
	)
	if err != nil {
		return 0, false, err
	}
	id, _ := res.LastInsertId()
	return id, true, nil
}

// List returns a character's snapshots, newest first.
func (s *Store) List(character string) ([]StoredSnapshot, error) {
	rows, err := s.db.Query(
		`SELECT id, character, captured_at, raw_text, parsed_json
		 FROM character_stat_snapshots
		 WHERE character = ? COLLATE NOCASE
		 ORDER BY captured_at DESC, id DESC`,
		character,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []StoredSnapshot{}
	for rows.Next() {
		var r StoredSnapshot
		var blob string
		if err := rows.Scan(&r.ID, &r.Character, &r.CapturedAt, &r.Raw, &blob); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(blob), &r.Snapshot); err != nil {
			// A parser-format change could leave old rows unreadable; skip
			// rather than fail the whole list (the raw text is still there).
			continue
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Delete removes one snapshot by id, scoped to a character so a stale client
// can't delete another character's row by guessing ids.
func (s *Store) Delete(character string, id int64) error {
	_, err := s.db.Exec(
		`DELETE FROM character_stat_snapshots WHERE id = ? AND character = ? COLLATE NOCASE`,
		id, character,
	)
	return err
}

// PruneOlderThan deletes snapshots captured before cutoff for all characters.
// Not wired to anything yet; here so a retention policy can be added without a
// migration.
func (s *Store) PruneOlderThan(cutoff time.Time) (int64, error) {
	res, err := s.db.Exec(
		`DELETE FROM character_stat_snapshots WHERE captured_at < ?`, cutoff.Unix())
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}
