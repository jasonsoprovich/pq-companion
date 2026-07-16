package lockout

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Entry is one persisted lockout row for a character: a single boss / legacy
// item and the absolute instant the player's lockout on it expires.
type Entry struct {
	Character  string  `json:"character"`
	Section    Section `json:"section"`
	Position   int     `json:"position"`
	TargetName string  `json:"target_name"`
	// ExpiresAt is unix seconds. 0 means the target was "Available" at snapshot
	// time (no active lockout). The frontend derives the live countdown from
	// this absolute instant, which is why it keeps working after the app has
	// been closed.
	ExpiresAt int64 `json:"expires_at"`
	// ObservedAt is unix seconds — when this `/sll` snapshot was captured. Lets
	// the UI show how stale the data is.
	ObservedAt int64 `json:"observed_at"`
}

// Store persists per-character `/sll` lockout snapshots in user.db. Coexists
// with the players / keyring / character / trigger / backup stores under WAL
// mode (each opens its own connection to the same file).
type Store struct {
	db *sql.DB
}

// OpenStore opens user.db at path and runs the lockout_entries migration.
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
		CREATE TABLE IF NOT EXISTS lockout_entries (
			character    TEXT    NOT NULL,
			section      TEXT    NOT NULL,
			position     INTEGER NOT NULL,
			target_name  TEXT    NOT NULL,
			expires_at   INTEGER NOT NULL DEFAULT 0,
			observed_at  INTEGER NOT NULL,
			PRIMARY KEY (character, section, position)
		)
	`); err != nil {
		return err
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS lockout_entries_character ON lockout_entries(character)`); err != nil {
		return err
	}
	return nil
}

// Snapshot replaces the character's entire lockout set with rows. `/sll` prints
// a full snapshot every time it runs, and the same target name can legitimately
// appear more than once (distinct instances), so we never upsert by name — we
// delete everything for the character and re-insert the ordered set in one
// transaction. position preserves the original `/sll` ordering and keeps
// duplicate names distinct. observedAt is stamped on every row.
//
// An empty rows slice clears the character's lockouts (the consumer never
// commits an empty burst, but the store handles it safely).
func (s *Store) Snapshot(character string, rows []Entry, observedAt time.Time) error {
	if character == "" {
		return fmt.Errorf("character required")
	}
	if observedAt.IsZero() {
		observedAt = time.Now()
	}
	obs := observedAt.Unix()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.Exec(`DELETE FROM lockout_entries WHERE character = ?`, character); err != nil {
		return fmt.Errorf("clear lockouts for %q: %w", character, err)
	}
	for i, r := range rows {
		if _, err := tx.Exec(`
			INSERT INTO lockout_entries (character, section, position, target_name, expires_at, observed_at)
			VALUES (?, ?, ?, ?, ?, ?)
		`, character, string(r.Section), i, r.TargetName, r.ExpiresAt, obs); err != nil {
			return fmt.Errorf("insert lockout char=%q pos=%d: %w", character, i, err)
		}
	}
	return tx.Commit()
}

// UpsertEntry records a single target's lockout, independent of an `/sll`
// snapshot — used for the per-target "You have incurred a lockout..." kill
// notice. Updates the row in place if the character already has an entry with
// this section+target_name (so re-killing the same boss just refreshes the
// expiry); otherwise appends a new row after the character's current highest
// position. A later `/sll` snapshot is authoritative and will overwrite/
// correct rows inserted this way.
func (s *Store) UpsertEntry(character string, section Section, targetName string, expiresAt, observedAt time.Time) error {
	if character == "" {
		return fmt.Errorf("character required")
	}
	if targetName == "" {
		return fmt.Errorf("target name required")
	}
	obs := observedAt.Unix()
	if obs == 0 {
		obs = time.Now().Unix()
	}
	exp := expiresAt.Unix()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	res, err := tx.Exec(`
		UPDATE lockout_entries
		SET expires_at = ?, observed_at = ?
		WHERE character = ? AND section = ? AND target_name = ?
	`, exp, obs, character, string(section), targetName)
	if err != nil {
		return fmt.Errorf("update lockout char=%q target=%q: %w", character, targetName, err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		if _, err := tx.Exec(`
			INSERT INTO lockout_entries (character, section, position, target_name, expires_at, observed_at)
			VALUES (?, ?, (SELECT COALESCE(MAX(position), -1) + 1 FROM lockout_entries WHERE character = ?), ?, ?, ?)
		`, character, string(section), character, targetName, exp, obs); err != nil {
			return fmt.Errorf("insert lockout char=%q target=%q: %w", character, targetName, err)
		}
	}
	return tx.Commit()
}

// ListByCharacter returns every lockout entry for the named character in
// snapshot order (section, then position). Empty slice (not nil) when none.
func (s *Store) ListByCharacter(character string) ([]Entry, error) {
	rows, err := s.db.Query(`
		SELECT character, section, position, target_name, expires_at, observed_at
		FROM lockout_entries
		WHERE character = ?
		ORDER BY section, position
	`, character)
	if err != nil {
		return nil, fmt.Errorf("query lockouts for %q: %w", character, err)
	}
	defer rows.Close()
	out := []Entry{}
	for rows.Next() {
		var e Entry
		if err := rows.Scan(&e.Character, &e.Section, &e.Position, &e.TargetName, &e.ExpiresAt, &e.ObservedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// Characters returns the distinct character names with at least one lockout
// entry, alphabetical. Used by the UI to render per-character tabs.
func (s *Store) Characters() ([]string, error) {
	rows, err := s.db.Query(`SELECT DISTINCT character FROM lockout_entries ORDER BY character COLLATE NOCASE`)
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

// Delete removes all lockout rows for a single character.
func (s *Store) Delete(character string) error {
	_, err := s.db.Exec(`DELETE FROM lockout_entries WHERE character = ? COLLATE NOCASE`, character)
	return err
}
