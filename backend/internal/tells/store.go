// Package tells persists direct player-to-player tells (incoming "X tells
// you, '…'" and outgoing "You told X, '…'") so the user can browse a history
// of their conversations grouped by the other player. Channel chatter
// (raid/guild/group/named channels) and NPC merchant/banker/trainer/pet
// replies are filtered out by the consumer before they ever reach the store —
// see parse.go.
package tells

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Direction values for a stored tell.
const (
	DirectionIn  = "in"  // someone tells the active character
	DirectionOut = "out" // the active character tells someone
)

// Tell is one stored tell line.
type Tell struct {
	ID        int64  `json:"id"`
	Character string `json:"character"` // the local character who saw/sent it
	Peer      string `json:"peer"`      // the other player
	Direction string `json:"direction"` // "in" | "out"
	Message   string `json:"message"`
	Zone      string `json:"zone"`
	TS        int64  `json:"ts"` // unix seconds
}

// Conversation is a per-peer summary row for the list view.
type Conversation struct {
	Peer          string `json:"peer"`
	Count         int    `json:"count"`
	FirstTS       int64  `json:"first_ts"`
	LastTS        int64  `json:"last_ts"`
	LastMessage   string `json:"last_message"`
	LastDirection string `json:"last_direction"`
}

// Store wraps the user.db connection.
type Store struct {
	db *sql.DB
}

// OpenStore opens user.db at path and applies the tells migration.
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
		CREATE TABLE IF NOT EXISTS tells (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			character  TEXT    NOT NULL DEFAULT '',
			peer       TEXT    NOT NULL,
			direction  TEXT    NOT NULL,
			message    TEXT    NOT NULL DEFAULT '',
			zone       TEXT    NOT NULL DEFAULT '',
			ts         INTEGER NOT NULL
		)
	`); err != nil {
		return err
	}
	// Dedup guard: the same line scanned from a log after it was captured live
	// (or scanned twice) must not produce duplicate rows. Insert uses
	// INSERT OR IGNORE against this unique tuple.
	if _, err := s.db.Exec(`
		CREATE UNIQUE INDEX IF NOT EXISTS tells_unique
		ON tells(character, peer, direction, ts, message)
	`); err != nil {
		return err
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS tells_char_peer ON tells(character, peer)`); err != nil {
		return err
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS tells_ts ON tells(ts)`); err != nil {
		return err
	}
	return nil
}

// Input is one tell to persist.
type Input struct {
	Character string
	Peer      string
	Direction string
	Message   string
	Zone      string
	TS        time.Time
}

// Insert stores a tell, ignoring exact duplicates (see tells_unique). Returns
// (inserted, error) where inserted is false when the row was a duplicate.
func (s *Store) Insert(in Input) (bool, error) {
	if in.Peer == "" || in.Direction == "" {
		return false, fmt.Errorf("peer and direction required")
	}
	ts := in.TS
	if ts.IsZero() {
		ts = time.Now()
	}
	res, err := s.db.Exec(`
		INSERT OR IGNORE INTO tells (character, peer, direction, message, zone, ts)
		VALUES (?, ?, ?, ?, ?, ?)
	`, in.Character, in.Peer, in.Direction, in.Message, in.Zone, ts.Unix())
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// ConversationFilters narrows the conversation list.
type ConversationFilters struct {
	Character    string // "" = all characters
	PeerContains string
	SortDesc     bool // true = newest activity first
	Limit        int
	Offset       int
}

// charPred builds an optional "AND <col> = ?" predicate for a character scope.
func charPred(col, character string) (string, []any) {
	if character == "" {
		return "", nil
	}
	return " AND " + col + " = ? COLLATE NOCASE", []any{character}
}

// Conversations returns per-peer summaries (newest-or-oldest activity first).
func (s *Store) Conversations(f ConversationFilters) ([]Conversation, error) {
	mainPred, mainArgs := charPred("character", f.Character)
	subPred, subArgs := charPred("l.character", f.Character)

	q := `SELECT t.peer, COUNT(*), MIN(t.ts), MAX(t.ts),
	             (SELECT message   FROM tells l WHERE l.peer = t.peer COLLATE NOCASE` + subPred + ` ORDER BY l.ts DESC, l.id DESC LIMIT 1),
	             (SELECT direction FROM tells l WHERE l.peer = t.peer COLLATE NOCASE` + subPred + ` ORDER BY l.ts DESC, l.id DESC LIMIT 1)
	      FROM tells t
	      WHERE 1=1` + mainPred
	args := []any{}
	args = append(args, subArgs...) // first subquery
	args = append(args, subArgs...) // second subquery
	args = append(args, mainArgs...)
	if f.PeerContains != "" {
		q += ` AND t.peer LIKE ? COLLATE NOCASE`
		args = append(args, "%"+f.PeerContains+"%")
	}
	q += ` GROUP BY t.peer COLLATE NOCASE`
	if f.SortDesc {
		q += ` ORDER BY MAX(t.ts) DESC`
	} else {
		q += ` ORDER BY MAX(t.ts) ASC`
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
	var out []Conversation
	for rows.Next() {
		var c Conversation
		if err := rows.Scan(&c.Peer, &c.Count, &c.FirstTS, &c.LastTS, &c.LastMessage, &c.LastDirection); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Messages returns the full thread with one peer, oldest-first.
func (s *Store) Messages(character, peer string, sortDesc bool) ([]Tell, error) {
	pred, args := charPred("character", character)
	q := `SELECT id, character, peer, direction, message, zone, ts
	      FROM tells WHERE peer = ? COLLATE NOCASE` + pred
	allArgs := append([]any{peer}, args...)
	if sortDesc {
		q += ` ORDER BY ts DESC, id DESC`
	} else {
		q += ` ORDER BY ts ASC, id ASC`
	}
	rows, err := s.db.Query(q, allArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Tell
	for rows.Next() {
		var t Tell
		if err := rows.Scan(&t.ID, &t.Character, &t.Peer, &t.Direction, &t.Message, &t.Zone, &t.TS); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Characters returns the distinct, non-empty local character names that have
// stored tells, sorted case-insensitively.
func (s *Store) Characters() ([]string, error) {
	rows, err := s.db.Query(`SELECT DISTINCT character FROM tells WHERE character <> '' ORDER BY character COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// DeletePeer removes all tells with one peer (optionally scoped to a character).
func (s *Store) DeletePeer(character, peer string) error {
	pred, args := charPred("character", character)
	_, err := s.db.Exec(`DELETE FROM tells WHERE peer = ? COLLATE NOCASE`+pred, append([]any{peer}, args...)...)
	return err
}

// Clear wipes all tells (optionally scoped to a character). Returns the number
// of rows deleted.
func (s *Store) Clear(character string) (int, error) {
	pred, args := charPred("character", character)
	res, err := s.db.Exec(`DELETE FROM tells WHERE 1=1`+pred, args...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
