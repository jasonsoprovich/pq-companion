package trigger

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Store persists trigger definitions in user.db.
type Store struct {
	db *sql.DB
}

// OpenStore opens (or creates) user.db at path and runs schema migrations.
//
// Three packages (trigger, character, backup) each open their own *sql.DB
// against the same user.db file. WAL mode lets readers and a single writer
// coexist, but concurrent writers still hit SQLITE_BUSY — the busy_timeout
// is how long SQLite will retry before giving up. 30s comfortably covers
// startup bursts like zeal.RefreshAllPersonas writing every character's AAs
// while the user clicks "Install trigger pack".
//
// modernc.org/sqlite expects PRAGMAs via the _pragma=NAME(VALUE) URI form;
// the mattn-style _journal_mode/_busy_timeout query params are silently
// ignored, which previously left the DB in default (DELETE) journal mode
// with a 0 busy_timeout — surfacing SQLITE_BUSY at the slightest contention.
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

// Close closes the underlying database connection.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS triggers (
			id                     TEXT    NOT NULL PRIMARY KEY,
			name                   TEXT    NOT NULL,
			enabled                INTEGER NOT NULL DEFAULT 1,
			pattern                TEXT    NOT NULL,
			actions                TEXT    NOT NULL DEFAULT '[]',
			pack_name              TEXT    NOT NULL DEFAULT '',
			created_at             INTEGER NOT NULL,
			timer_type             TEXT    NOT NULL DEFAULT 'none',
			timer_duration_secs    INTEGER NOT NULL DEFAULT 0,
			worn_off_pattern       TEXT    NOT NULL DEFAULT '',
			spell_id               INTEGER NOT NULL DEFAULT 0,
			display_threshold_secs INTEGER NOT NULL DEFAULT 0,
			characters             TEXT    NOT NULL DEFAULT '[]',
			timer_alerts           TEXT    NOT NULL DEFAULT '[]',
			exclude_patterns       TEXT    NOT NULL DEFAULT '[]'
		)
	`); err != nil {
		return err
	}

	// Idempotently add columns for databases created before each feature.
	addColumns := []string{
		`ALTER TABLE triggers ADD COLUMN timer_type TEXT NOT NULL DEFAULT 'none'`,
		`ALTER TABLE triggers ADD COLUMN timer_duration_secs INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE triggers ADD COLUMN worn_off_pattern TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE triggers ADD COLUMN spell_id INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE triggers ADD COLUMN display_threshold_secs INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE triggers ADD COLUMN characters TEXT NOT NULL DEFAULT '[]'`,
		`ALTER TABLE triggers ADD COLUMN timer_alerts TEXT NOT NULL DEFAULT '[]'`,
		`ALTER TABLE triggers ADD COLUMN exclude_patterns TEXT NOT NULL DEFAULT '[]'`,
	}
	for _, stmt := range addColumns {
		if _, err := s.db.Exec(stmt); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
			return fmt.Errorf("add column: %w", err)
		}
	}
	return nil
}

// BackfillCharactersIfNeeded is a one-time migration that populates the
// characters list of every existing trigger with the supplied character
// names. Triggered by PRAGMA user_version: runs only when the version is
// below 1, then bumps it. Safe to call on every startup. Skips entirely
// when names is empty so we don't lock in "no characters" before any have
// been recorded.
func (s *Store) BackfillCharactersIfNeeded(names []string) error {
	var version int
	if err := s.db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("read user_version: %w", err)
	}
	if version >= 1 {
		return nil
	}
	if len(names) == 0 {
		// No characters yet — defer migration to a future startup so we don't
		// permanently lock existing triggers into an empty (= all) list.
		return nil
	}
	payload, err := json.Marshal(names)
	if err != nil {
		return fmt.Errorf("marshal names: %w", err)
	}
	if _, err := s.db.Exec(`UPDATE triggers SET characters = ? WHERE characters = '[]' OR characters = '' OR characters IS NULL`, string(payload)); err != nil {
		return fmt.Errorf("backfill characters: %w", err)
	}
	if _, err := s.db.Exec(`PRAGMA user_version = 1`); err != nil {
		return fmt.Errorf("bump user_version: %w", err)
	}
	return nil
}

// Insert saves a new trigger to the database.
func (s *Store) Insert(t *Trigger) error {
	actJSON, err := json.Marshal(t.Actions)
	if err != nil {
		return fmt.Errorf("marshal actions: %w", err)
	}
	if t.Characters == nil {
		t.Characters = []string{}
	}
	charJSON, err := json.Marshal(t.Characters)
	if err != nil {
		return fmt.Errorf("marshal characters: %w", err)
	}
	if t.TimerAlerts == nil {
		t.TimerAlerts = []TimerAlert{}
	}
	alertJSON, err := json.Marshal(t.TimerAlerts)
	if err != nil {
		return fmt.Errorf("marshal timer_alerts: %w", err)
	}
	if t.ExcludePatterns == nil {
		t.ExcludePatterns = []string{}
	}
	excludeJSON, err := json.Marshal(t.ExcludePatterns)
	if err != nil {
		return fmt.Errorf("marshal exclude_patterns: %w", err)
	}
	if t.TimerType == "" {
		t.TimerType = TimerTypeNone
	}
	_, err = s.db.Exec(
		`INSERT INTO triggers (id, name, enabled, pattern, actions, pack_name, created_at,
		                       timer_type, timer_duration_secs, worn_off_pattern, spell_id,
		                       display_threshold_secs, characters, timer_alerts, exclude_patterns)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.Name, boolToInt(t.Enabled), t.Pattern, string(actJSON), t.PackName, t.CreatedAt.Unix(),
		string(t.TimerType), t.TimerDurationSecs, t.WornOffPattern, t.SpellID,
		t.DisplayThresholdSecs, string(charJSON), string(alertJSON), string(excludeJSON),
	)
	if err != nil {
		return fmt.Errorf("insert trigger: %w", err)
	}
	return nil
}

// List returns all triggers ordered by creation time ascending.
func (s *Store) List() ([]*Trigger, error) {
	rows, err := s.db.Query(
		`SELECT id, name, enabled, pattern, actions, pack_name, created_at,
		        timer_type, timer_duration_secs, worn_off_pattern, spell_id,
		        display_threshold_secs, characters, timer_alerts, exclude_patterns
		 FROM triggers ORDER BY created_at ASC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list triggers: %w", err)
	}
	defer rows.Close()

	var triggers []*Trigger
	for rows.Next() {
		t, err := scanTrigger(rows)
		if err != nil {
			return nil, err
		}
		triggers = append(triggers, t)
	}
	return triggers, rows.Err()
}

// Get returns the trigger with the given ID, or ErrNotFound.
func (s *Store) Get(id string) (*Trigger, error) {
	row := s.db.QueryRow(
		`SELECT id, name, enabled, pattern, actions, pack_name, created_at,
		        timer_type, timer_duration_secs, worn_off_pattern, spell_id,
		        display_threshold_secs, characters, timer_alerts, exclude_patterns
		 FROM triggers WHERE id = ?`, id,
	)
	t, err := scanTrigger(row)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get trigger %s: %w", id, err)
	}
	return t, nil
}

// Update saves changes to an existing trigger.
func (s *Store) Update(t *Trigger) error {
	actJSON, err := json.Marshal(t.Actions)
	if err != nil {
		return fmt.Errorf("marshal actions: %w", err)
	}
	if t.Characters == nil {
		t.Characters = []string{}
	}
	charJSON, err := json.Marshal(t.Characters)
	if err != nil {
		return fmt.Errorf("marshal characters: %w", err)
	}
	if t.TimerAlerts == nil {
		t.TimerAlerts = []TimerAlert{}
	}
	alertJSON, err := json.Marshal(t.TimerAlerts)
	if err != nil {
		return fmt.Errorf("marshal timer_alerts: %w", err)
	}
	if t.ExcludePatterns == nil {
		t.ExcludePatterns = []string{}
	}
	excludeJSON, err := json.Marshal(t.ExcludePatterns)
	if err != nil {
		return fmt.Errorf("marshal exclude_patterns: %w", err)
	}
	if t.TimerType == "" {
		t.TimerType = TimerTypeNone
	}
	res, err := s.db.Exec(
		`UPDATE triggers SET name=?, enabled=?, pattern=?, actions=?, pack_name=?,
		                     timer_type=?, timer_duration_secs=?, worn_off_pattern=?, spell_id=?,
		                     display_threshold_secs=?, characters=?, timer_alerts=?, exclude_patterns=?
		 WHERE id=?`,
		t.Name, boolToInt(t.Enabled), t.Pattern, string(actJSON), t.PackName,
		string(t.TimerType), t.TimerDurationSecs, t.WornOffPattern, t.SpellID,
		t.DisplayThresholdSecs, string(charJSON), string(alertJSON), string(excludeJSON),
		t.ID,
	)
	if err != nil {
		return fmt.Errorf("update trigger: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete removes the trigger with the given ID.
func (s *Store) Delete(id string) error {
	res, err := s.db.Exec(`DELETE FROM triggers WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete trigger: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteByPack removes all triggers belonging to the named pack.
func (s *Store) DeleteByPack(packName string) error {
	_, err := s.db.Exec(`DELETE FROM triggers WHERE pack_name = ?`, packName)
	if err != nil {
		return fmt.Errorf("delete pack %s: %w", packName, err)
	}
	return nil
}

// DeleteAll removes every trigger in one statement. Used by the Clear All
// flow on the Triggers page so the frontend doesn't have to fan out N
// per-id deletes (each of which would otherwise trigger its own engine
// reload on the API side).
func (s *Store) DeleteAll() error {
	if _, err := s.db.Exec(`DELETE FROM triggers`); err != nil {
		return fmt.Errorf("delete all triggers: %w", err)
	}
	return nil
}

type scanner interface {
	Scan(...any) error
}

func scanTrigger(row scanner) (*Trigger, error) {
	var t Trigger
	var enabledInt int
	var actJSON, charJSON, alertJSON, excludeJSON string
	var unixSec int64
	var timerType string
	if err := row.Scan(
		&t.ID, &t.Name, &enabledInt, &t.Pattern, &actJSON, &t.PackName, &unixSec,
		&timerType, &t.TimerDurationSecs, &t.WornOffPattern, &t.SpellID,
		&t.DisplayThresholdSecs, &charJSON, &alertJSON, &excludeJSON,
	); err != nil {
		return nil, err
	}
	t.Enabled = enabledInt != 0
	t.CreatedAt = time.Unix(unixSec, 0).UTC()
	t.TimerType = TimerType(timerType)
	if t.TimerType == "" {
		t.TimerType = TimerTypeNone
	}
	if err := json.Unmarshal([]byte(actJSON), &t.Actions); err != nil {
		t.Actions = []Action{}
	}
	if t.Actions == nil {
		t.Actions = []Action{}
	}
	if charJSON != "" {
		if err := json.Unmarshal([]byte(charJSON), &t.Characters); err != nil {
			t.Characters = []string{}
		}
	}
	if t.Characters == nil {
		t.Characters = []string{}
	}
	if alertJSON != "" {
		if err := json.Unmarshal([]byte(alertJSON), &t.TimerAlerts); err != nil {
			t.TimerAlerts = []TimerAlert{}
		}
	}
	if t.TimerAlerts == nil {
		t.TimerAlerts = []TimerAlert{}
	}
	if excludeJSON != "" {
		if err := json.Unmarshal([]byte(excludeJSON), &t.ExcludePatterns); err != nil {
			t.ExcludePatterns = []string{}
		}
	}
	if t.ExcludePatterns == nil {
		t.ExcludePatterns = []string{}
	}
	return &t, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
