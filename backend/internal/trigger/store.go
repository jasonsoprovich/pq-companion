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
func OpenStore(path string) (*Store, error) {
	dsn := fmt.Sprintf("file:%s?_journal_mode=WAL&_busy_timeout=5000", path)
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
			id                 TEXT    NOT NULL PRIMARY KEY,
			name               TEXT    NOT NULL,
			enabled            INTEGER NOT NULL DEFAULT 1,
			pattern            TEXT    NOT NULL,
			actions            TEXT    NOT NULL DEFAULT '[]',
			pack_name          TEXT    NOT NULL DEFAULT '',
			created_at         INTEGER NOT NULL,
			timer_type         TEXT    NOT NULL DEFAULT 'none',
			timer_duration_secs INTEGER NOT NULL DEFAULT 0,
			worn_off_pattern   TEXT    NOT NULL DEFAULT '',
			spell_id           INTEGER NOT NULL DEFAULT 0
		)
	`); err != nil {
		return err
	}

	// Idempotently add timer columns for databases created before the timer feature.
	addColumns := []string{
		`ALTER TABLE triggers ADD COLUMN timer_type TEXT NOT NULL DEFAULT 'none'`,
		`ALTER TABLE triggers ADD COLUMN timer_duration_secs INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE triggers ADD COLUMN worn_off_pattern TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE triggers ADD COLUMN spell_id INTEGER NOT NULL DEFAULT 0`,
	}
	for _, stmt := range addColumns {
		if _, err := s.db.Exec(stmt); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
			return fmt.Errorf("add column: %w", err)
		}
	}
	return nil
}

// Insert saves a new trigger to the database.
func (s *Store) Insert(t *Trigger) error {
	actJSON, err := json.Marshal(t.Actions)
	if err != nil {
		return fmt.Errorf("marshal actions: %w", err)
	}
	if t.TimerType == "" {
		t.TimerType = TimerTypeNone
	}
	_, err = s.db.Exec(
		`INSERT INTO triggers (id, name, enabled, pattern, actions, pack_name, created_at,
		                       timer_type, timer_duration_secs, worn_off_pattern, spell_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.Name, boolToInt(t.Enabled), t.Pattern, string(actJSON), t.PackName, t.CreatedAt.Unix(),
		string(t.TimerType), t.TimerDurationSecs, t.WornOffPattern, t.SpellID,
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
		        timer_type, timer_duration_secs, worn_off_pattern, spell_id
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
		        timer_type, timer_duration_secs, worn_off_pattern, spell_id
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
	if t.TimerType == "" {
		t.TimerType = TimerTypeNone
	}
	res, err := s.db.Exec(
		`UPDATE triggers SET name=?, enabled=?, pattern=?, actions=?, pack_name=?,
		                     timer_type=?, timer_duration_secs=?, worn_off_pattern=?, spell_id=?
		 WHERE id=?`,
		t.Name, boolToInt(t.Enabled), t.Pattern, string(actJSON), t.PackName,
		string(t.TimerType), t.TimerDurationSecs, t.WornOffPattern, t.SpellID,
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

type scanner interface {
	Scan(...any) error
}

func scanTrigger(row scanner) (*Trigger, error) {
	var t Trigger
	var enabledInt int
	var actJSON string
	var unixSec int64
	var timerType string
	if err := row.Scan(
		&t.ID, &t.Name, &enabledInt, &t.Pattern, &actJSON, &t.PackName, &unixSec,
		&timerType, &t.TimerDurationSecs, &t.WornOffPattern, &t.SpellID,
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
	return &t, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
