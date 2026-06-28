package popflag

import (
	"database/sql"
	"encoding/json"
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
	// Raw Seer snapshot per character — kept for audit and re-derivation when
	// the dataset's completion rules change.
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS pop_seer_snapshot (
			character TEXT PRIMARY KEY COLLATE NOCASE,
			qglobals  TEXT NOT NULL,
			raw_text  TEXT NOT NULL,
			taken_at  INTEGER NOT NULL
		)
	`); err != nil {
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

// Snapshot is the stored raw Seer reading for a character.
type Snapshot struct {
	Character string            `json:"character"`
	Qglobals  map[string]string `json:"qglobals"`
	RawText   string            `json:"raw_text"`
	TakenAt   int64             `json:"taken_at"`
}

// ApplySeer records a Seer guided-meditation reading: it replaces all
// non-manual rows for the character with seer-sourced rows for the flags the
// reading marks complete, and stores the raw snapshot for audit.
//
// Precedence (manual > seer > auto) is enforced here: existing manual rows are
// never touched (the seer insert hits ON CONFLICT DO NOTHING), while stale
// seer/auto rows are cleared first so a re-reading can retract a flag the
// character no longer shows.
func (s *Store) ApplySeer(character string, qglobals map[string]string, rawText string, observedAt time.Time) ([]string, error) {
	if character == "" {
		return nil, fmt.Errorf("character required")
	}
	if observedAt.IsZero() {
		observedAt = time.Now()
	}
	now := observedAt.Unix()
	done := DeriveCompletion(qglobals)

	qjson, err := json.Marshal(qglobals)
	if err != nil {
		return nil, fmt.Errorf("marshal qglobals: %w", err)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback() //nolint:errcheck

	// Clear non-manual rows so retractions take effect and seer supersedes auto.
	if _, err := tx.Exec(`DELETE FROM pop_flag_state WHERE character = ? AND source != 'manual'`, character); err != nil {
		return nil, fmt.Errorf("clear non-manual rows for %q: %w", character, err)
	}
	// Insert seer rows; a surviving manual row wins (DO NOTHING).
	for _, id := range done {
		if _, err := tx.Exec(`
			INSERT INTO pop_flag_state (character, flag_id, done, source, updated_at)
			VALUES (?, ?, 1, ?, ?)
			ON CONFLICT(character, flag_id) DO NOTHING
		`, character, id, SourceSeer, now); err != nil {
			return nil, fmt.Errorf("insert seer row char=%q flag=%q: %w", character, id, err)
		}
	}
	if _, err := tx.Exec(`
		INSERT INTO pop_seer_snapshot (character, qglobals, raw_text, taken_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(character) DO UPDATE SET
			qglobals = excluded.qglobals,
			raw_text = excluded.raw_text,
			taken_at = excluded.taken_at
	`, character, string(qjson), rawText, now); err != nil {
		return nil, fmt.Errorf("upsert snapshot for %q: %w", character, err)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return done, nil
}

// GetSnapshot returns the stored raw Seer snapshot for a character, or nil when
// none has been recorded.
func (s *Store) GetSnapshot(character string) (*Snapshot, error) {
	row := s.db.QueryRow(`SELECT qglobals, raw_text, taken_at FROM pop_seer_snapshot WHERE character = ?`, character)
	var qjson, rawText string
	var takenAt int64
	if err := row.Scan(&qjson, &rawText, &takenAt); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, err
	}
	snap := &Snapshot{Character: character, RawText: rawText, TakenAt: takenAt}
	if err := json.Unmarshal([]byte(qjson), &snap.Qglobals); err != nil {
		return nil, fmt.Errorf("unmarshal snapshot qglobals: %w", err)
	}
	return snap, nil
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
