package loot

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Entry is one stored loot row.
type Entry struct {
	ID        int64  `json:"id"`
	Character string `json:"character"` // local log owner (for per-character scoping)
	Player    string `json:"player"`    // the looter
	Item      string `json:"item"`
	Zone      string `json:"zone"`
	NPC       string `json:"npc"` // best-effort; currently always "" (not in the log line)
	TS        int64  `json:"ts"`
}

// Store wraps the user.db connection.
type Store struct {
	db *sql.DB
}

// OpenStore opens user.db at path and applies the loot migration.
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
		`CREATE TABLE IF NOT EXISTS loot_events (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			character  TEXT    NOT NULL DEFAULT '',
			player     TEXT    NOT NULL,
			item       TEXT    NOT NULL,
			zone       TEXT    NOT NULL DEFAULT '',
			npc        TEXT    NOT NULL DEFAULT '',
			ts         INTEGER NOT NULL
		)`,
		// Dedup guard so a line captured live then later backfilled (or a
		// double backfill) doesn't duplicate.
		`CREATE UNIQUE INDEX IF NOT EXISTS loot_unique ON loot_events(character, player, item, ts)`,
		`CREATE INDEX IF NOT EXISTS loot_char_ts ON loot_events(character, ts)`,
		`CREATE INDEX IF NOT EXISTS loot_player ON loot_events(player)`,
		`CREATE INDEX IF NOT EXISTS loot_item ON loot_events(item)`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

// Input is one loot row to persist.
type Input struct {
	Character string
	Player    string
	Item      string
	Zone      string
	NPC       string
	TS        time.Time
}

// Insert stores a loot row, ignoring exact duplicates. Returns whether a new
// row was written.
func (s *Store) Insert(in Input) (bool, error) {
	if in.Player == "" || in.Item == "" {
		return false, fmt.Errorf("player and item required")
	}
	ts := in.TS
	if ts.IsZero() {
		ts = time.Now()
	}
	res, err := s.db.Exec(`
		INSERT OR IGNORE INTO loot_events (character, player, item, zone, npc, ts)
		VALUES (?, ?, ?, ?, ?, ?)
	`, in.Character, in.Player, in.Item, in.Zone, in.NPC, ts.Unix())
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// Filters narrows the loot list.
type Filters struct {
	Character string // "" = all
	Search    string // matches item OR player text
	Player    string // exact looter filter
	Zone      string // exact zone filter
	From, To  int64  // unix seconds; 0 = unbounded
	SortDesc  bool
	Limit     int
	Offset    int
}

// List returns loot rows matching the filters.
func (s *Store) List(f Filters) ([]Entry, error) {
	q := `SELECT id, character, player, item, zone, npc, ts FROM loot_events WHERE 1=1`
	var args []any
	if f.Character != "" {
		q += ` AND character = ? COLLATE NOCASE`
		args = append(args, f.Character)
	}
	if f.Search != "" {
		q += ` AND (item LIKE ? COLLATE NOCASE OR player LIKE ? COLLATE NOCASE)`
		args = append(args, "%"+f.Search+"%", "%"+f.Search+"%")
	}
	if f.Player != "" {
		q += ` AND player = ? COLLATE NOCASE`
		args = append(args, f.Player)
	}
	if f.Zone != "" {
		q += ` AND zone = ? COLLATE NOCASE`
		args = append(args, f.Zone)
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
		if err := rows.Scan(&e.ID, &e.Character, &e.Player, &e.Item, &e.Zone, &e.NPC, &e.TS); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// distinct returns the distinct non-empty values of col, optionally scoped to a
// character, sorted case-insensitively.
func (s *Store) distinct(col, character string) ([]string, error) {
	q := `SELECT DISTINCT ` + col + ` FROM loot_events WHERE ` + col + ` <> ''`
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

// Players returns distinct looter names (scoped to a character if given).
func (s *Store) Players(character string) ([]string, error) { return s.distinct("player", character) }

// Zones returns distinct zones present (scoped to a character if given).
func (s *Store) Zones(character string) ([]string, error) { return s.distinct("zone", character) }

// Characters returns the distinct local character names that have loot rows.
func (s *Store) Characters() ([]string, error) { return s.distinct("character", "") }

// Clear wipes loot rows, optionally scoped to a character. Returns rows deleted.
func (s *Store) Clear(character string) (int, error) {
	q := `DELETE FROM loot_events WHERE 1=1`
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
