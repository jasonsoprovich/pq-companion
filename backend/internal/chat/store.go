// Package chat persists player chat across tracked channels (direct tells plus
// guild/raid/group/ooc/auction/shout and named custom channels) so the user
// can browse history. Direct tells keep a per-peer conversation view; other
// channels are a flat chronological feed. Channel chatter that isn't player
// speech, and NPC merchant/pet tell replies, are filtered out by the parser
// before reaching the store. A configurable retention window (Purge) keeps the
// single table lean.
package chat

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Message is one stored chat line.
type Message struct {
	ID        int64  `json:"id"`
	Character string `json:"character"` // the local character who saw/sent it
	Channel   string `json:"channel"`
	Direction string `json:"direction"` // "in" | "out"
	Peer      string `json:"peer"`      // tell: other player; channel-in: speaker; channel-out: ""
	Message   string `json:"message"`
	Zone      string `json:"zone"`
	TS        int64  `json:"ts"`
}

// Conversation is a per-peer tell summary for the conversation list.
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

// OpenStore opens user.db at path and applies the chat migration.
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
		`CREATE TABLE IF NOT EXISTS chat_messages (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			character  TEXT    NOT NULL DEFAULT '',
			channel    TEXT    NOT NULL,
			direction  TEXT    NOT NULL,
			peer       TEXT    NOT NULL DEFAULT '',
			message    TEXT    NOT NULL DEFAULT '',
			zone       TEXT    NOT NULL DEFAULT '',
			ts         INTEGER NOT NULL
		)`,
		// Dedup guard so a line captured live then later backfilled (or a
		// double backfill) never duplicates.
		`CREATE UNIQUE INDEX IF NOT EXISTS chat_unique
			ON chat_messages(character, channel, direction, peer, ts, message)`,
		`CREATE INDEX IF NOT EXISTS chat_char_channel_ts ON chat_messages(character, channel, ts)`,
		`CREATE INDEX IF NOT EXISTS chat_char_peer_ts ON chat_messages(character, peer, ts)`,
		`CREATE INDEX IF NOT EXISTS chat_ts ON chat_messages(ts)`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

// Input is one chat line to persist.
type Input struct {
	Character string
	Channel   string
	Direction string
	Peer      string
	Message   string
	Zone      string
	TS        time.Time
}

// Insert stores a chat line, ignoring exact duplicates. Returns whether a new
// row was written.
func (s *Store) Insert(in Input) (bool, error) {
	if in.Channel == "" || in.Direction == "" {
		return false, fmt.Errorf("channel and direction required")
	}
	ts := in.TS
	if ts.IsZero() {
		ts = time.Now()
	}
	res, err := s.db.Exec(`
		INSERT OR IGNORE INTO chat_messages (character, channel, direction, peer, message, zone, ts)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, in.Character, in.Channel, in.Direction, in.Peer, in.Message, in.Zone, ts.Unix())
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// charPred builds an optional "AND <col> = ?" predicate for a character scope.
func charPred(col, character string) (string, []any) {
	if character == "" {
		return "", nil
	}
	return " AND " + col + " = ? COLLATE NOCASE", []any{character}
}

// dateRange appends ts >= from / ts <= to predicates (0 = unbounded).
func dateRange(col string, from, to int64) (string, []any) {
	q := ""
	var args []any
	if from > 0 {
		q += " AND " + col + " >= ?"
		args = append(args, from)
	}
	if to > 0 {
		q += " AND " + col + " <= ?"
		args = append(args, to)
	}
	return q, args
}

// ConversationFilters narrows the tell conversation list.
type ConversationFilters struct {
	Character    string
	PeerContains string
	From, To     int64 // unix seconds; 0 = unbounded
	SortDesc     bool
	Limit        int
	Offset       int
}

// Conversations returns per-peer tell summaries (channel = tell only).
func (s *Store) Conversations(f ConversationFilters) ([]Conversation, error) {
	mainPred, mainArgs := charPred("character", f.Character)
	subPred, subArgs := charPred("l.character", f.Character)
	dr, drArgs := dateRange("t.ts", f.From, f.To)

	q := `SELECT t.peer, COUNT(*), MIN(t.ts), MAX(t.ts),
	             (SELECT message   FROM chat_messages l WHERE l.channel='tell' AND l.peer = t.peer COLLATE NOCASE` + subPred + ` ORDER BY l.ts DESC, l.id DESC LIMIT 1),
	             (SELECT direction FROM chat_messages l WHERE l.channel='tell' AND l.peer = t.peer COLLATE NOCASE` + subPred + ` ORDER BY l.ts DESC, l.id DESC LIMIT 1)
	      FROM chat_messages t
	      WHERE t.channel='tell'` + mainPred + dr
	args := []any{}
	args = append(args, subArgs...)
	args = append(args, subArgs...)
	args = append(args, mainArgs...)
	args = append(args, drArgs...)
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

// Thread returns the full tell history with one peer, oldest-first by default.
func (s *Store) Thread(character, peer string, sortDesc bool) ([]Message, error) {
	pred, args := charPred("character", character)
	q := `SELECT id, character, channel, direction, peer, message, zone, ts
	      FROM chat_messages WHERE channel='tell' AND peer = ? COLLATE NOCASE` + pred
	allArgs := append([]any{peer}, args...)
	if sortDesc {
		q += ` ORDER BY ts DESC, id DESC`
	} else {
		q += ` ORDER BY ts ASC, id ASC`
	}
	return s.queryMessages(q, allArgs)
}

// FeedFilters narrows the flat per-channel message feed.
type FeedFilters struct {
	Character string
	Channel   string // required for the feed view
	Search    string // matches peer OR message text
	From, To  int64
	SortDesc  bool
	Limit     int
	Offset    int
}

// Feed returns a flat chronological message list for one channel.
func (s *Store) Feed(f FeedFilters) ([]Message, error) {
	pred, args := charPred("character", f.Character)
	dr, drArgs := dateRange("ts", f.From, f.To)
	q := `SELECT id, character, channel, direction, peer, message, zone, ts
	      FROM chat_messages WHERE channel = ? COLLATE NOCASE` + pred + dr
	allArgs := append([]any{f.Channel}, args...)
	allArgs = append(allArgs, drArgs...)
	if f.Search != "" {
		q += ` AND (peer LIKE ? COLLATE NOCASE OR message LIKE ? COLLATE NOCASE)`
		allArgs = append(allArgs, "%"+f.Search+"%", "%"+f.Search+"%")
	}
	if f.SortDesc {
		q += ` ORDER BY ts DESC, id DESC`
	} else {
		q += ` ORDER BY ts ASC, id ASC`
	}
	if f.Limit > 0 {
		q += ` LIMIT ?`
		allArgs = append(allArgs, f.Limit)
		if f.Offset > 0 {
			q += ` OFFSET ?`
			allArgs = append(allArgs, f.Offset)
		}
	}
	return s.queryMessages(q, allArgs)
}

func (s *Store) queryMessages(q string, args []any) ([]Message, error) {
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Message
	for rows.Next() {
		var m Message
		if err := rows.Scan(&m.ID, &m.Character, &m.Channel, &m.Direction, &m.Peer, &m.Message, &m.Zone, &m.TS); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// Channels returns the distinct channels present (optionally scoped to a
// character), so the UI can populate its channel dropdown with channels that
// actually have messages.
func (s *Store) Channels(character string) ([]string, error) {
	pred, args := charPred("character", character)
	rows, err := s.db.Query(`SELECT DISTINCT channel FROM chat_messages WHERE 1=1`+pred+` ORDER BY channel COLLATE NOCASE`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{} // non-nil so the API serializes [] not null
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Characters returns the distinct local character names that have chat rows.
func (s *Store) Characters() ([]string, error) {
	rows, err := s.db.Query(`SELECT DISTINCT character FROM chat_messages WHERE character <> '' ORDER BY character COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// DeletePeer removes a tell conversation (channel = tell) with one peer.
func (s *Store) DeletePeer(character, peer string) error {
	pred, args := charPred("character", character)
	_, err := s.db.Exec(`DELETE FROM chat_messages WHERE channel='tell' AND peer = ? COLLATE NOCASE`+pred,
		append([]any{peer}, args...)...)
	return err
}

// Clear wipes chat rows, optionally scoped to a character and/or channel
// (empty = no scope on that dimension). Returns rows deleted.
func (s *Store) Clear(character, channel string) (int, error) {
	q := `DELETE FROM chat_messages WHERE 1=1`
	var args []any
	if cp, ca := charPred("character", character); cp != "" {
		q += cp
		args = append(args, ca...)
	}
	if channel != "" {
		q += ` AND channel = ? COLLATE NOCASE`
		args = append(args, channel)
	}
	res, err := s.db.Exec(q, args...)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// Purge deletes messages older than the cutoff. Returns rows deleted. Used by
// the retention job; a zero/!positive retention window disables purging at the
// caller, so this is only invoked with a real cutoff.
func (s *Store) Purge(before time.Time) (int, error) {
	res, err := s.db.Exec(`DELETE FROM chat_messages WHERE ts < ?`, before.Unix())
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}
