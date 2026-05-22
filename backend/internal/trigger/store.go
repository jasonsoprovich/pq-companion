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
			exclude_patterns       TEXT    NOT NULL DEFAULT '[]',
			source                 TEXT    NOT NULL DEFAULT 'log',
			pipe_condition         TEXT    NOT NULL DEFAULT '',
			dedup_key              TEXT    NOT NULL DEFAULT '',
			cooldown_secs          INTEGER NOT NULL DEFAULT 0
		)
	`); err != nil {
		return err
	}

	// Tracks one-time additive pack default updates so each runs at most
	// once. See ApplyDefaultUpdates / DefaultUpdates for the migration list.
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS pack_default_updates (
			key        TEXT    NOT NULL PRIMARY KEY,
			applied_at INTEGER NOT NULL
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
		`ALTER TABLE triggers ADD COLUMN source TEXT NOT NULL DEFAULT 'log'`,
		`ALTER TABLE triggers ADD COLUMN pipe_condition TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE triggers ADD COLUMN dedup_key TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE triggers ADD COLUMN cooldown_secs INTEGER NOT NULL DEFAULT 0`,
	}
	for _, stmt := range addColumns {
		if _, err := s.db.Exec(stmt); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
			return fmt.Errorf("add column: %w", err)
		}
	}
	return nil
}

// MigrateGroupAwarenessToGeneralTriggers is a one-time migration that
// renames any installed "Group Awareness" pack to "General Triggers" and
// inserts the two new class-agnostic triggers (Spell Resist, Spell
// Interrupt) shipped with the rename. Idempotent via the
// pack_default_updates ledger.
//
// Why: the Triggers page used to host a separate "Global Alerts" tab with
// hardcoded death/zone/resist/interrupt event handlers. That subsystem
// was removed; the resist/interrupt cases now live as regular triggers
// inside the renamed pack so users get one unified surface.
func (s *Store) MigrateGroupAwarenessToGeneralTriggers() error {
	const key = "GroupAwareness:RenameAndAddSpellTriggers:v1"
	applied, err := s.IsDefaultUpdateApplied(key)
	if err != nil {
		return err
	}
	if applied {
		return nil
	}
	if _, err := s.db.Exec(
		`UPDATE triggers SET pack_name = 'General Triggers' WHERE pack_name = 'Group Awareness'`,
	); err != nil {
		return fmt.Errorf("rename Group Awareness pack: %w", err)
	}
	hasPack, err := s.packHasAnyTrigger("General Triggers")
	if err != nil {
		return err
	}
	if hasPack {
		additions := []Trigger{
			{
				Name:     "Spell Resist",
				Enabled:  true,
				Pattern:  `Your target resisted the (.+) spell\.`,
				PackName: "General Triggers",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "RESISTED!", DurationSecs: 3, Color: "#ffaa00"},
				},
			},
			{
				Name:     "Spell Interrupt",
				Enabled:  true,
				Pattern:  `Your(?: (.+))? spell is interrupted\.`,
				PackName: "General Triggers",
				Actions: []Action{
					{Type: ActionOverlayText, Text: "INTERRUPTED!", DurationSecs: 3, Color: "#ffaa00"},
				},
			},
		}
		for i := range additions {
			t := &additions[i]
			existing, err := s.FindByPackAndName(t.PackName, t.Name)
			if err != nil {
				return err
			}
			if existing != nil {
				continue
			}
			id, err := NewID()
			if err != nil {
				return err
			}
			t.ID = id
			t.CreatedAt = time.Now().UTC()
			if err := s.Insert(t); err != nil {
				return fmt.Errorf("insert %s: %w", t.Name, err)
			}
		}
	}
	return s.MarkDefaultUpdateApplied(key)
}

// MigrateMezBrokeTTSPronunciation rewrites the TTS text on the installed
// "Mez Broke" triggers from "Mez broke" to "Mezz broke" so Windows SAPI
// reads the EQ term correctly. Idempotent via pack_default_updates.
//
// Built-in pack definitions ship the corrected spelling; this migration
// only matters for users who installed the Enchanter / Bard pack before
// the rename.
func (s *Store) MigrateMezBrokeTTSPronunciation() error {
	const key = "MezBroke:TTSPronunciation:v1"
	applied, err := s.IsDefaultUpdateApplied(key)
	if err != nil {
		return err
	}
	if applied {
		return nil
	}
	for _, packName := range []string{"Enchanter", "Bard"} {
		t, err := s.FindByPackAndName(packName, "Mez Broke")
		if err != nil {
			return err
		}
		if t == nil {
			continue
		}
		changed := false
		for i := range t.Actions {
			if t.Actions[i].Type == ActionTextToSpeech && t.Actions[i].Text == "Mez broke" {
				t.Actions[i].Text = "Mezz broke"
				changed = true
			}
		}
		if !changed {
			continue
		}
		if err := s.Update(t); err != nil {
			return fmt.Errorf("update %s mez broke: %w", packName, err)
		}
	}
	return s.MarkDefaultUpdateApplied(key)
}

func (s *Store) packHasAnyTrigger(pack string) (bool, error) {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM triggers WHERE pack_name = ?`, pack).Scan(&n); err != nil {
		return false, fmt.Errorf("count pack %s: %w", pack, err)
	}
	return n > 0, nil
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
	source, pipeJSON := normalizeSourceAndCondition(t)
	_, err = s.db.Exec(
		`INSERT INTO triggers (id, name, enabled, pattern, actions, pack_name, created_at,
		                       timer_type, timer_duration_secs, worn_off_pattern, spell_id,
		                       display_threshold_secs, characters, timer_alerts, exclude_patterns,
		                       source, pipe_condition, dedup_key, cooldown_secs)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.Name, boolToInt(t.Enabled), t.Pattern, string(actJSON), t.PackName, t.CreatedAt.Unix(),
		string(t.TimerType), t.TimerDurationSecs, t.WornOffPattern, t.SpellID,
		t.DisplayThresholdSecs, string(charJSON), string(alertJSON), string(excludeJSON),
		source, pipeJSON, t.DedupKey, t.CooldownSecs,
	)
	if err != nil {
		return fmt.Errorf("insert trigger: %w", err)
	}
	return nil
}

// normalizeSourceAndCondition resolves the persisted Source value (defaults
// to "log" when empty) and marshals the optional PipeCondition to JSON. A
// nil PipeCondition serializes as the empty string so the column reads back
// cleanly. Pipe-source triggers must have a non-nil PipeCondition; the
// engine ignores any whose JSON fails to parse.
func normalizeSourceAndCondition(t *Trigger) (string, string) {
	src := t.Source
	if src == "" {
		src = SourceLog
	}
	if t.PipeCondition == nil {
		return src, ""
	}
	b, err := json.Marshal(t.PipeCondition)
	if err != nil {
		return src, ""
	}
	return src, string(b)
}

// List returns all triggers ordered by creation time ascending.
func (s *Store) List() ([]*Trigger, error) {
	rows, err := s.db.Query(
		`SELECT id, name, enabled, pattern, actions, pack_name, created_at,
		        timer_type, timer_duration_secs, worn_off_pattern, spell_id,
		        display_threshold_secs, characters, timer_alerts, exclude_patterns,
		        source, pipe_condition, dedup_key, cooldown_secs
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
		        display_threshold_secs, characters, timer_alerts, exclude_patterns,
		        source, pipe_condition, dedup_key, cooldown_secs
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
	source, pipeJSON := normalizeSourceAndCondition(t)
	res, err := s.db.Exec(
		`UPDATE triggers SET name=?, enabled=?, pattern=?, actions=?, pack_name=?,
		                     timer_type=?, timer_duration_secs=?, worn_off_pattern=?, spell_id=?,
		                     display_threshold_secs=?, characters=?, timer_alerts=?, exclude_patterns=?,
		                     source=?, pipe_condition=?, dedup_key=?, cooldown_secs=?
		 WHERE id=?`,
		t.Name, boolToInt(t.Enabled), t.Pattern, string(actJSON), t.PackName,
		string(t.TimerType), t.TimerDurationSecs, t.WornOffPattern, t.SpellID,
		t.DisplayThresholdSecs, string(charJSON), string(alertJSON), string(excludeJSON),
		source, pipeJSON, t.DedupKey, t.CooldownSecs,
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

// FindByPackAndName returns the (single) trigger with the given pack name
// and trigger name, or (nil, nil) if no match exists. Used by the default-
// updates pipeline to locate built-in pack triggers without iterating the
// whole list at the call site. Returns the first match if duplicates somehow
// exist; the trigger UI prevents duplicate names within a pack.
func (s *Store) FindByPackAndName(packName, name string) (*Trigger, error) {
	row := s.db.QueryRow(
		`SELECT id, name, enabled, pattern, actions, pack_name, created_at,
		        timer_type, timer_duration_secs, worn_off_pattern, spell_id,
		        display_threshold_secs, characters, timer_alerts, exclude_patterns,
		        source, pipe_condition, dedup_key, cooldown_secs
		 FROM triggers WHERE pack_name = ? AND name = ? LIMIT 1`,
		packName, name,
	)
	t, err := scanTrigger(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find trigger %s/%s: %w", packName, name, err)
	}
	return t, nil
}

// FindByDedupKey returns the trigger that currently owns the given
// dedup_key, or (nil, nil) when no trigger has claimed it. Used by
// InstallPack to decide whether to skip a pack trigger because another
// pack already provides the same conceptual entry (e.g. Root shared by
// Wizard and Enchanter packs), and by DeleteByPack's promote-on-uninstall
// path to detect orphaned keys.
func (s *Store) FindByDedupKey(key string) (*Trigger, error) {
	if key == "" {
		return nil, nil
	}
	row := s.db.QueryRow(
		`SELECT id, name, enabled, pattern, actions, pack_name, created_at,
		        timer_type, timer_duration_secs, worn_off_pattern, spell_id,
		        display_threshold_secs, characters, timer_alerts, exclude_patterns,
		        source, pipe_condition, dedup_key, cooldown_secs
		 FROM triggers WHERE dedup_key = ? LIMIT 1`, key,
	)
	t, err := scanTrigger(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find trigger by dedup_key %s: %w", key, err)
	}
	return t, nil
}

// IsDefaultUpdateApplied reports whether the named default-update key has
// already run. ApplyDefaultUpdates uses this to keep one-time additive
// migrations idempotent across restarts.
func (s *Store) IsDefaultUpdateApplied(key string) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM pack_default_updates WHERE key = ?`, key).Scan(&n)
	if err != nil {
		return false, fmt.Errorf("check default update %s: %w", key, err)
	}
	return n > 0, nil
}

// MarkDefaultUpdateApplied records that the named default-update key has
// run. Idempotent (INSERT OR IGNORE) so a duplicate call from a parallel
// startup path doesn't error.
func (s *Store) MarkDefaultUpdateApplied(key string) error {
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO pack_default_updates (key, applied_at) VALUES (?, ?)`,
		key, time.Now().UTC().Unix(),
	)
	if err != nil {
		return fmt.Errorf("mark default update %s: %w", key, err)
	}
	return nil
}

// InstalledPackNames returns the set of distinct pack_name values that
// currently have at least one trigger in the store. Empty pack_name
// (user-authored triggers) is excluded. Used by UninstallPack to know
// which other packs are candidates for promoting orphaned dedup_keys.
func (s *Store) InstalledPackNames() (map[string]bool, error) {
	rows, err := s.db.Query(`SELECT DISTINCT pack_name FROM triggers WHERE pack_name <> ''`)
	if err != nil {
		return nil, fmt.Errorf("list installed packs: %w", err)
	}
	defer rows.Close()
	out := make(map[string]bool)
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out[name] = true
	}
	return out, rows.Err()
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
	var actJSON, charJSON, alertJSON, excludeJSON, source, pipeJSON string
	var unixSec int64
	var timerType string
	if err := row.Scan(
		&t.ID, &t.Name, &enabledInt, &t.Pattern, &actJSON, &t.PackName, &unixSec,
		&timerType, &t.TimerDurationSecs, &t.WornOffPattern, &t.SpellID,
		&t.DisplayThresholdSecs, &charJSON, &alertJSON, &excludeJSON,
		&source, &pipeJSON, &t.DedupKey, &t.CooldownSecs,
	); err != nil {
		return nil, err
	}
	t.Source = source
	if t.Source == "" {
		t.Source = SourceLog
	}
	if pipeJSON != "" {
		var pc PipeCondition
		if err := json.Unmarshal([]byte(pipeJSON), &pc); err == nil && pc.Kind != "" {
			t.PipeCondition = &pc
		}
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
