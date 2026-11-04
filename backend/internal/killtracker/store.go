// Package killtracker records every mob kill the log shows (the Kill Log) and
// drives user-defined kill counters with milestones, resync lines and an
// optional event clock (Kill Counters) — e.g. the Bertoxxulous event in the
// Crypt of Decay, where bosses spawn off a server-side trash-kill count.
package killtracker

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Entry is one stored kill row.
type Entry struct {
	ID        int64  `json:"id"`
	Character string `json:"character"` // local log owner (for per-character scoping)
	Target    string `json:"target"`    // what died
	Killer    string `json:"killer"`    // "" when the log line names no killer
	Zone      string `json:"zone"`
	TS        int64  `json:"ts"`
	// IsNPC is false when the target resolves to no npc_types row — almost
	// always a player slain by a mob.
	IsNPC bool `json:"is_npc"`
	// Named is a best-effort "proper name" flag (see isProperName). Some
	// capitalized trash ("Knight of Decay") is flagged too.
	Named bool `json:"named"`
}

// Store wraps the user.db connection.
type Store struct {
	db *sql.DB
}

// OpenStore opens user.db at path and applies the kill tracker migration.
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
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS kill_events (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			character  TEXT    NOT NULL DEFAULT '',
			target     TEXT    NOT NULL,
			killer     TEXT    NOT NULL DEFAULT '',
			zone       TEXT    NOT NULL DEFAULT '',
			ts         INTEGER NOT NULL,
			seq        INTEGER NOT NULL DEFAULT 0,
			is_npc     INTEGER NOT NULL DEFAULT 1,
			named      INTEGER NOT NULL DEFAULT 0
		)`,
		// Dedup guard so a kill captured live and later backfilled (or a
		// double backfill) is stored once. seq distinguishes genuinely
		// separate same-second kills of the same name by the same killer
		// (an AE pull) — see recorder.
		`CREATE UNIQUE INDEX IF NOT EXISTS kill_unique ON kill_events(character, target, killer, ts, seq)`,
		`CREATE INDEX IF NOT EXISTS kill_char_ts ON kill_events(character, ts)`,
		`CREATE INDEX IF NOT EXISTS kill_zone ON kill_events(zone)`,
		`CREATE INDEX IF NOT EXISTS kill_target ON kill_events(target)`,
		`CREATE TABLE IF NOT EXISTS kill_counters (
			id               TEXT    PRIMARY KEY,
			sort_order       INTEGER NOT NULL DEFAULT 0,
			def              TEXT    NOT NULL,
			count            INTEGER NOT NULL DEFAULT 0,
			clock_started_at INTEGER NOT NULL DEFAULT 0,
			updated_at       INTEGER NOT NULL DEFAULT 0
		)`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}
	// Added later: the zone copy a zone counter's count belongs to.
	if _, err := s.db.Exec(`ALTER TABLE kill_counters ADD COLUMN zone_copy TEXT NOT NULL DEFAULT ''`); err != nil &&
		!strings.Contains(err.Error(), "duplicate column name") {
		return err
	}
	return nil
}

// Input is one kill row to persist.
type Input struct {
	Character string
	Target    string
	Killer    string
	Zone      string
	TS        time.Time
	Seq       int
	IsNPC     bool
	Named     bool
}

// Insert stores a kill row, ignoring exact duplicates. Returns the stored
// entry and whether a new row was written.
func (s *Store) Insert(in Input) (Entry, bool, error) {
	if in.Target == "" {
		return Entry{}, false, fmt.Errorf("target required")
	}
	ts := in.TS
	if ts.IsZero() {
		ts = time.Now()
	}
	res, err := s.db.Exec(`
		INSERT OR IGNORE INTO kill_events (character, target, killer, zone, ts, seq, is_npc, named)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, in.Character, in.Target, in.Killer, in.Zone, ts.Unix(), in.Seq, boolInt(in.IsNPC), boolInt(in.Named))
	if err != nil {
		return Entry{}, false, err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return Entry{}, false, nil
	}
	id, _ := res.LastInsertId()
	return Entry{
		ID: id, Character: in.Character, Target: in.Target, Killer: in.Killer,
		Zone: in.Zone, TS: ts.Unix(), IsNPC: in.IsNPC, Named: in.Named,
	}, true, nil
}

// Scope selects which kinds of deaths a List returns.
type Scope string

const (
	ScopeNPC   Scope = "npc"   // NPC deaths only (the default)
	ScopeNamed Scope = "named" // NPC deaths with a proper name
	ScopeAll   Scope = "all"   // everything, including player deaths
)

// Filters narrows the kill list.
type Filters struct {
	Character string // "" = all
	Search    string // matches target OR killer text
	Zone      string // exact zone filter
	Scope     Scope
	From, To  int64 // unix seconds; 0 = unbounded
	SortDesc  bool
	Limit     int
	Offset    int
}

// List returns kill rows matching the filters.
func (s *Store) List(f Filters) ([]Entry, error) {
	q := `SELECT id, character, target, killer, zone, ts, is_npc, named FROM kill_events WHERE 1=1`
	var args []any
	if f.Character != "" {
		q += ` AND character = ? COLLATE NOCASE`
		args = append(args, f.Character)
	}
	if f.Search != "" {
		q += ` AND (target LIKE ? COLLATE NOCASE OR killer LIKE ? COLLATE NOCASE)`
		args = append(args, "%"+f.Search+"%", "%"+f.Search+"%")
	}
	if f.Zone != "" {
		q += ` AND zone = ? COLLATE NOCASE`
		args = append(args, f.Zone)
	}
	switch f.Scope {
	case ScopeAll:
	case ScopeNamed:
		q += ` AND is_npc = 1 AND named = 1`
	default:
		q += ` AND is_npc = 1`
	}
	if f.From > 0 {
		q += ` AND ts >= ?`
		args = append(args, f.From)
	}
	if f.To > 0 {
		q += ` AND ts <= ?`
		args = append(args, f.To)
	}
	if f.SortDesc {
		q += ` ORDER BY ts DESC, id DESC`
	} else {
		q += ` ORDER BY ts ASC, id ASC`
	}
	if f.Limit > 0 {
		q += ` LIMIT ?`
		args = append(args, f.Limit)
		if f.Offset > 0 {
			q += ` OFFSET ?`
			args = append(args, f.Offset)
		}
	}
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Entry{}
	for rows.Next() {
		var e Entry
		var isNPC, named int
		if err := rows.Scan(&e.ID, &e.Character, &e.Target, &e.Killer, &e.Zone, &e.TS, &isNPC, &named); err != nil {
			return nil, err
		}
		e.IsNPC, e.Named = isNPC != 0, named != 0
		out = append(out, e)
	}
	return out, rows.Err()
}

// distinct returns the distinct non-empty values of col, optionally scoped to a
// character, sorted case-insensitively. col is always a package constant.
func (s *Store) distinct(col, character string) ([]string, error) {
	q := `SELECT DISTINCT ` + col + ` FROM kill_events WHERE ` + col + ` <> ''`
	var args []any
	if character != "" {
		q += ` AND character = ? COLLATE NOCASE`
		args = append(args, character)
	}
	q += ` ORDER BY ` + col + ` COLLATE NOCASE`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// Zones returns distinct zones present (scoped to a character if given).
func (s *Store) Zones(character string) ([]string, error) { return s.distinct("zone", character) }

// Characters returns the distinct local character names that have kill rows.
func (s *Store) Characters() ([]string, error) { return s.distinct("character", "") }

// Clear wipes kill rows, optionally scoped to a character. Returns rows deleted.
func (s *Store) Clear(character string) (int, error) {
	q := `DELETE FROM kill_events WHERE 1=1`
	var args []any
	if character != "" {
		q += ` AND character = ? COLLATE NOCASE`
		args = append(args, character)
	}
	res, err := s.db.Exec(q, args...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// ── Counters ────────────────────────────────────────────────────────────────

// LoadCounters returns every stored counter in display order.
func (s *Store) LoadCounters() ([]*Counter, error) {
	rows, err := s.db.Query(`
		SELECT def, count, clock_started_at, updated_at, zone_copy
		FROM kill_counters ORDER BY sort_order, id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Counter{}
	for rows.Next() {
		var def, zoneCopy string
		var count int
		var clockStarted, updated int64
		if err := rows.Scan(&def, &count, &clockStarted, &updated, &zoneCopy); err != nil {
			return nil, err
		}
		var c Counter
		if err := json.Unmarshal([]byte(def), &c.Def); err != nil {
			// A corrupt row shouldn't take down every other counter.
			continue
		}
		c.Count = count
		c.ClockStartedAt = clockStarted
		c.UpdatedAt = updated
		c.ZoneCopy = zoneCopy
		out = append(out, &c)
	}
	return out, rows.Err()
}

// SaveCounter upserts one counter's definition and live state.
func (s *Store) SaveCounter(c *Counter, order int) error {
	def, err := json.Marshal(c.Def)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
		INSERT INTO kill_counters (id, sort_order, def, count, clock_started_at, updated_at, zone_copy)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			sort_order = excluded.sort_order,
			def = excluded.def,
			count = excluded.count,
			clock_started_at = excluded.clock_started_at,
			updated_at = excluded.updated_at,
			zone_copy = excluded.zone_copy
	`, c.Def.ID, order, string(def), c.Count, c.ClockStartedAt, c.UpdatedAt, c.ZoneCopy)
	return err
}

// DeleteCounter removes one counter.
func (s *Store) DeleteCounter(id string) error {
	_, err := s.db.Exec(`DELETE FROM kill_counters WHERE id = ?`, id)
	return err
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
