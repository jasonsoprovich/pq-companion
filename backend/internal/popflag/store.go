package popflag

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Source precedence for the effective state of a (character, flag) row.
// manual > seer > auto: a manual toggle is a deliberate user correction that a
// later Seer reading or live-event inference must never overwrite.
const (
	SourceManual = "manual"
	SourceSeer   = "seer"
	SourceAuto   = "auto"
)

// State is one persisted per-character flag row.
type State struct {
	FlagID    string `json:"flag_id"`
	Done      bool   `json:"done"`
	Source    string `json:"source"`
	UpdatedAt int64  `json:"updated_at"`
}

// Store persists per-character PoP flag progress in user.db.
type Store struct {
	db *sql.DB
}

// OpenStore opens user.db and runs the pop_flag_state migration. Coexists with
// the keyring / players / character / trigger stores under WAL mode.
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
		CREATE TABLE IF NOT EXISTS pop_flag_state (
			character  TEXT    NOT NULL COLLATE NOCASE,
			flag_id    TEXT    NOT NULL,
			done       INTEGER NOT NULL DEFAULT 0,
			source     TEXT    NOT NULL,
			updated_at INTEGER NOT NULL,
			PRIMARY KEY (character, flag_id)
		)
	`); err != nil {
		return err
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS pop_flag_state_character ON pop_flag_state(character)`); err != nil {
		return err
	}
	return nil
}

// Get returns every stored flag row for the named character. Empty slice (not
// nil) when there are none.
func (s *Store) Get(character string) ([]State, error) {
	rows, err := s.db.Query(`
		SELECT flag_id, done, source, updated_at
		FROM pop_flag_state
		WHERE character = ?
	`, character)
	if err != nil {
		return nil, fmt.Errorf("query pop_flag_state for %q: %w", character, err)
	}
	defer rows.Close()
	out := []State{}
	for rows.Next() {
		var st State
		var done int
		if err := rows.Scan(&st.FlagID, &done, &st.Source, &st.UpdatedAt); err != nil {
			return nil, err
		}
		st.Done = done != 0
		out = append(out, st)
	}
	return out, rows.Err()
}

// SetManual records a deliberate user toggle (done=1 confirms, done=0 retracts
// a false auto/seer positive). It always wins, so it upserts unconditionally
// with source='manual'.
func (s *Store) SetManual(character, flagID string, done bool) error {
	if character == "" {
		return fmt.Errorf("character required")
	}
	if _, ok := ByID(flagID); !ok {
		return fmt.Errorf("unknown flag id %q", flagID)
	}
	return s.upsert(character, flagID, done, SourceManual)
}

func (s *Store) upsert(character, flagID string, done bool, source string) error {
	d := 0
	if done {
		d = 1
	}
	_, err := s.db.Exec(`
		INSERT INTO pop_flag_state (character, flag_id, done, source, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(character, flag_id) DO UPDATE SET
			done = excluded.done,
			source = excluded.source,
			updated_at = excluded.updated_at
	`, character, flagID, d, source, time.Now().Unix())
	if err != nil {
		return fmt.Errorf("upsert pop_flag_state char=%q flag=%q: %w", character, flagID, err)
	}
	return nil
}

// Characters returns the distinct character names with at least one stored
// flag row, alphabetically. Used by the UI to render per-character tabs.
func (s *Store) Characters() ([]string, error) {
	rows, err := s.db.Query(`SELECT DISTINCT character FROM pop_flag_state ORDER BY character COLLATE NOCASE`)
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
