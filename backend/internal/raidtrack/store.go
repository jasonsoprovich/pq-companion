// Package raidtrack records raid positions for Raid Replay. While Zeal sends
// raid frames, every raider in the observer's zone is sampled every few
// seconds and written to user.db along with the kills and deaths the log
// shows, so a raid night can be scrubbed back on the zone map.
package raidtrack

import (
	"bytes"
	"compress/gzip"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"time"

	_ "modernc.org/sqlite"
)

// Session is one continuous recording: one character in one zone.
type Session struct {
	ID        int64  `json:"id"`
	Character string `json:"character"`
	Zone      string `json:"zone"`
	// ZoneName is the zone's long name, filled in by the API.
	ZoneName  string    `json:"zone_name,omitempty"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at"`
	Frames    int       `json:"frames"`
	// PeakMembers is the most raiders seen in the zone in one frame.
	PeakMembers int `json:"peak_members"`
	Kills       int `json:"kills"`
	Deaths      int `json:"deaths"`
}

// MemberInfo is one raider in a session's roster. Frames refer to raiders by
// Index.
type MemberInfo struct {
	Index int    `json:"index"`
	Name  string `json:"name"`
	Class int    `json:"class"` // Zeal 1-indexed class id, 0 unknown
	Level int    `json:"level"`
}

// Frame is one sample. Each row of M is
// [index, x, y, z, heading, group, hp%] with x/y in the spawn2 convention
// (zealpipe.Location.GameX/GameY), group 0 for ungrouped and hp% -1 when
// unknown.
type Frame struct {
	T int64   `json:"t"` // unix ms
	M [][]int `json:"m"`
}

// Event kinds.
const (
	EventKill  = "kill"  // an NPC died
	EventDeath = "death" // a raider died
)

// Event is a kill or death during a session.
type Event struct {
	T     int64  `json:"t"` // unix ms
	Kind  string `json:"kind"`
	Name  string `json:"name"`            // what died
	Other string `json:"other,omitempty"` // the killer, when the log names one
}

// Recording is a whole session for playback.
type Recording struct {
	Session Session      `json:"session"`
	Members []MemberInfo `json:"members"`
	Frames  []Frame      `json:"frames"`
	Events  []Event      `json:"events"`
}

// Store wraps the user.db connection.
type Store struct {
	db *sql.DB
}

// OpenStore opens user.db at path and applies the raid replay migration.
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
		`CREATE TABLE IF NOT EXISTS raid_track_sessions (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			character    TEXT    NOT NULL DEFAULT '',
			zone         TEXT    NOT NULL DEFAULT '',
			started_at   INTEGER NOT NULL,
			ended_at     INTEGER NOT NULL,
			frames       INTEGER NOT NULL DEFAULT 0,
			peak_members INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS raid_track_sessions_started ON raid_track_sessions(started_at)`,
		`CREATE INDEX IF NOT EXISTS raid_track_sessions_ended ON raid_track_sessions(ended_at)`,
		`CREATE TABLE IF NOT EXISTS raid_track_members (
			session_id INTEGER NOT NULL,
			idx        INTEGER NOT NULL,
			name       TEXT    NOT NULL,
			class      INTEGER NOT NULL DEFAULT 0,
			level      INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (session_id, idx)
		)`,
		// One row per minute of frames, gzipped JSON: raiders barely move
		// between samples, so a minute compresses far better than a frame.
		`CREATE TABLE IF NOT EXISTS raid_track_chunks (
			session_id INTEGER NOT NULL,
			start_ms   INTEGER NOT NULL,
			frames     INTEGER NOT NULL,
			data       BLOB    NOT NULL,
			PRIMARY KEY (session_id, start_ms)
		)`,
		`CREATE TABLE IF NOT EXISTS raid_track_events (
			id         INTEGER PRIMARY KEY AUTOINCREMENT,
			session_id INTEGER NOT NULL,
			at_ms      INTEGER NOT NULL,
			kind       TEXT    NOT NULL,
			name       TEXT    NOT NULL,
			other      TEXT    NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS raid_track_events_session ON raid_track_events(session_id, at_ms)`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) createSession(character, zone string, at time.Time) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO raid_track_sessions (character, zone, started_at, ended_at) VALUES (?, ?, ?, ?)`,
		character, zone, at.UnixMilli(), at.UnixMilli())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) addMember(session int64, m MemberInfo) error {
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO raid_track_members (session_id, idx, name, class, level) VALUES (?, ?, ?, ?, ?)`,
		session, m.Index, m.Name, m.Class, m.Level)
	return err
}

// writeChunk stores a minute of frames and moves the session's end and
// counters forward.
func (s *Store) writeChunk(session int64, frames []Frame, total, peak int) error {
	if len(frames) == 0 {
		return nil
	}
	data, err := encodeFrames(frames)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.Exec(
		`INSERT OR REPLACE INTO raid_track_chunks (session_id, start_ms, frames, data) VALUES (?, ?, ?, ?)`,
		session, frames[0].T, len(frames), data); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`UPDATE raid_track_sessions SET ended_at = ?, frames = ?, peak_members = ? WHERE id = ?`,
		frames[len(frames)-1].T, total, peak, session); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) addEvent(session int64, ev Event) error {
	_, err := s.db.Exec(
		`INSERT INTO raid_track_events (session_id, at_ms, kind, name, other) VALUES (?, ?, ?, ?, ?)`,
		session, ev.T, ev.Kind, ev.Name, ev.Other)
	return err
}

// ListSessions returns every session, newest first.
func (s *Store) ListSessions() ([]Session, error) {
	rows, err := s.db.Query(`
		SELECT s.id, s.character, s.zone, s.started_at, s.ended_at, s.frames, s.peak_members,
		       (SELECT COUNT(*) FROM raid_track_events e WHERE e.session_id = s.id AND e.kind = ?),
		       (SELECT COUNT(*) FROM raid_track_events e WHERE e.session_id = s.id AND e.kind = ?)
		FROM raid_track_sessions s
		WHERE s.frames > 0
		ORDER BY s.started_at DESC`, EventKill, EventDeath)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Session{}
	for rows.Next() {
		var ss Session
		var start, end int64
		if err := rows.Scan(&ss.ID, &ss.Character, &ss.Zone, &start, &end, &ss.Frames, &ss.PeakMembers,
			&ss.Kills, &ss.Deaths); err != nil {
			return nil, err
		}
		ss.StartedAt, ss.EndedAt = time.UnixMilli(start), time.UnixMilli(end)
		out = append(out, ss)
	}
	return out, rows.Err()
}

// GetRecording loads one session for playback. ok is false when it doesn't
// exist.
func (s *Store) GetRecording(id int64) (rec Recording, ok bool, err error) {
	var start, end int64
	err = s.db.QueryRow(`
		SELECT id, character, zone, started_at, ended_at, frames, peak_members
		FROM raid_track_sessions WHERE id = ?`, id).Scan(
		&rec.Session.ID, &rec.Session.Character, &rec.Session.Zone, &start, &end,
		&rec.Session.Frames, &rec.Session.PeakMembers)
	if err == sql.ErrNoRows {
		return rec, false, nil
	}
	if err != nil {
		return rec, false, err
	}
	rec.Session.StartedAt, rec.Session.EndedAt = time.UnixMilli(start), time.UnixMilli(end)

	rec.Members = []MemberInfo{}
	mrows, err := s.db.Query(`SELECT idx, name, class, level FROM raid_track_members WHERE session_id = ? ORDER BY idx`, id)
	if err != nil {
		return rec, false, err
	}
	for mrows.Next() {
		var m MemberInfo
		if err := mrows.Scan(&m.Index, &m.Name, &m.Class, &m.Level); err != nil {
			mrows.Close()
			return rec, false, err
		}
		rec.Members = append(rec.Members, m)
	}
	mrows.Close()

	rec.Frames = []Frame{}
	crows, err := s.db.Query(`SELECT data FROM raid_track_chunks WHERE session_id = ? ORDER BY start_ms`, id)
	if err != nil {
		return rec, false, err
	}
	for crows.Next() {
		var data []byte
		if err := crows.Scan(&data); err != nil {
			crows.Close()
			return rec, false, err
		}
		frames, err := decodeFrames(data)
		if err != nil {
			crows.Close()
			return rec, false, fmt.Errorf("decode chunk: %w", err)
		}
		rec.Frames = append(rec.Frames, frames...)
	}
	crows.Close()

	rec.Events = []Event{}
	erows, err := s.db.Query(`SELECT at_ms, kind, name, other FROM raid_track_events WHERE session_id = ? ORDER BY at_ms, id`, id)
	if err != nil {
		return rec, false, err
	}
	defer erows.Close()
	kills, deaths := 0, 0
	for erows.Next() {
		var ev Event
		if err := erows.Scan(&ev.T, &ev.Kind, &ev.Name, &ev.Other); err != nil {
			return rec, false, err
		}
		switch ev.Kind {
		case EventKill:
			kills++
		case EventDeath:
			deaths++
		}
		rec.Events = append(rec.Events, ev)
	}
	rec.Session.Kills, rec.Session.Deaths = kills, deaths
	return rec, true, erows.Err()
}

// DeleteSession removes one session and everything recorded for it.
func (s *Store) DeleteSession(id int64) error {
	return s.deleteWhere(`id = ?`, id)
}

// DeleteAll removes every recording.
func (s *Store) DeleteAll() error {
	return s.deleteWhere(`1 = 1`)
}

// PruneOlderThan removes sessions that ended before cutoff and returns how
// many went.
func (s *Store) PruneOlderThan(cutoff time.Time) (int, error) {
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM raid_track_sessions WHERE ended_at < ?`,
		cutoff.UnixMilli()).Scan(&n); err != nil {
		return 0, err
	}
	if n == 0 {
		return 0, nil
	}
	return n, s.deleteWhere(`ended_at < ?`, cutoff.UnixMilli())
}

// deleteWhere removes the sessions matching cond (a fixed SQL condition on
// raid_track_sessions with ? placeholders) and their rows.
func (s *Store) deleteWhere(cond string, args ...any) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	sub := `SELECT id FROM raid_track_sessions WHERE ` + cond
	for _, table := range []string{"raid_track_members", "raid_track_chunks", "raid_track_events"} {
		if _, err := tx.Exec(`DELETE FROM `+table+` WHERE session_id IN (`+sub+`)`, args...); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`DELETE FROM raid_track_sessions WHERE `+cond, args...); err != nil {
		return err
	}
	return tx.Commit()
}

func encodeFrames(frames []Frame) ([]byte, error) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if err := json.NewEncoder(zw).Encode(frames); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decodeFrames(data []byte) ([]Frame, error) {
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	raw, err := io.ReadAll(zr)
	if err != nil {
		return nil, err
	}
	var frames []Frame
	if err := json.Unmarshal(raw, &frames); err != nil {
		return nil, err
	}
	return frames, nil
}
