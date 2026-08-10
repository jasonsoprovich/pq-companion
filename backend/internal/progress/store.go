package progress

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Store wraps the user.db connection for progression tracking.
type Store struct {
	db *sql.DB
}

// OpenStore opens user.db at path and applies the progression migration.
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
		// Append-only journal, sourced from the EQ log (live tail and
		// backfill both write here). The UNIQUE constraint makes replay
		// idempotent via INSERT OR IGNORE: a level/AA/spell/skill event
		// carries a genuinely different value every time it fires for real,
		// so nothing legitimate collides.
		`CREATE TABLE IF NOT EXISTS character_progress_events (
			id        INTEGER PRIMARY KEY AUTOINCREMENT,
			character TEXT    NOT NULL COLLATE NOCASE,
			at        INTEGER NOT NULL,
			kind      TEXT    NOT NULL,
			detail    TEXT    NOT NULL DEFAULT '',
			value     INTEGER NOT NULL DEFAULT 0,
			delta     INTEGER NOT NULL DEFAULT 0,
			UNIQUE (character, at, kind, detail, value)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_progress_char_at ON character_progress_events(character, at)`,
		// Forward-only totals capture. Nothing here is derivable from the
		// log (coin spending isn't logged), so unlike the journal above this
		// table has no backfill path — see the Recorder in snapshot.go.
		`CREATE TABLE IF NOT EXISTS character_progress_snapshots (
			id               INTEGER PRIMARY KEY AUTOINCREMENT,
			character        TEXT    NOT NULL COLLATE NOCASE,
			taken_at         INTEGER NOT NULL,
			level            INTEGER NOT NULL DEFAULT 0,
			aa_ranks         INTEGER NOT NULL DEFAULT 0,
			tradeskill_total INTEGER NOT NULL DEFAULT 0,
			spells_known     INTEGER NOT NULL DEFAULT 0,
			copper           INTEGER NOT NULL DEFAULT 0,
			fingerprint      TEXT    NOT NULL DEFAULT '',
			created_at       INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_progress_snap_char_time ON character_progress_snapshots(character, taken_at)`,
		// One row per (character, calendar day) the character's log had any
		// activity at all — the actual "logged into the game" signal, as
		// opposed to character_progress_events which only fires on
		// level/AA/spell/skill milestones. Backfillable the same way as the
		// journal above.
		`CREATE TABLE IF NOT EXISTS character_active_days (
			character TEXT    NOT NULL COLLATE NOCASE,
			date      TEXT    NOT NULL,
			at        INTEGER NOT NULL,
			PRIMARY KEY (character, date)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_active_days_char_at ON character_active_days(character, at)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

// AppendEvent records a journal event, ignoring it if an identical
// (character, at, kind, detail, value) row already exists. Returns whether a
// new row was actually inserted, so callers (the live consumer and the
// backfill handler) can report accurate counts.
func (s *Store) AppendEvent(ev Event) (bool, error) {
	if ev.Character == "" {
		return false, nil
	}
	res, err := s.db.Exec(
		`INSERT OR IGNORE INTO character_progress_events (character, at, kind, detail, value, delta)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		ev.Character, ev.At.Unix(), string(ev.Kind), ev.Detail, ev.Value, ev.Delta,
	)
	if err != nil {
		return false, fmt.Errorf("insert progress event: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// EventsSince returns every journal event for character at or after since,
// oldest first.
func (s *Store) EventsSince(character string, since time.Time) ([]Event, error) {
	rows, err := s.db.Query(
		`SELECT character, at, kind, detail, value, delta FROM character_progress_events
		 WHERE character = ? COLLATE NOCASE AND at >= ?
		 ORDER BY at ASC, id ASC`,
		character, since.Unix(),
	)
	if err != nil {
		return nil, fmt.Errorf("query progress events: %w", err)
	}
	defer rows.Close()
	return scanEvents(rows)
}

// AllEventsSince returns every journal event across all characters at or
// after since, oldest first — used for the all-characters recap view.
func (s *Store) AllEventsSince(since time.Time) ([]Event, error) {
	rows, err := s.db.Query(
		`SELECT character, at, kind, detail, value, delta FROM character_progress_events
		 WHERE at >= ?
		 ORDER BY character COLLATE NOCASE ASC, at ASC, id ASC`,
		since.Unix(),
	)
	if err != nil {
		return nil, fmt.Errorf("query progress events: %w", err)
	}
	defer rows.Close()
	return scanEvents(rows)
}

func scanEvents(rows *sql.Rows) ([]Event, error) {
	out := []Event{}
	for rows.Next() {
		var (
			ev      Event
			at      int64
			kindRaw string
		)
		if err := rows.Scan(&ev.Character, &at, &kindRaw, &ev.Detail, &ev.Value, &ev.Delta); err != nil {
			return nil, fmt.Errorf("scan progress event: %w", err)
		}
		ev.At = time.Unix(at, 0)
		ev.Kind = Kind(kindRaw)
		out = append(out, ev)
	}
	return out, rows.Err()
}

// MarkActiveDay records that character had log activity on at's calendar
// day (character's local time). Idempotent — a day already on record is
// left untouched (first-seen timestamp within the day is what's stored).
func (s *Store) MarkActiveDay(character string, at time.Time) error {
	if character == "" {
		return nil
	}
	date := at.Local().Format("2006-01-02")
	_, err := s.db.Exec(
		`INSERT OR IGNORE INTO character_active_days (character, date, at) VALUES (?, ?, ?)`,
		character, date, at.Unix(),
	)
	if err != nil {
		return fmt.Errorf("insert active day: %w", err)
	}
	return nil
}

// ActiveDaysSince returns the distinct calendar-day strings (YYYY-MM-DD) on
// which character had log activity at or after since.
func (s *Store) ActiveDaysSince(character string, since time.Time) ([]string, error) {
	rows, err := s.db.Query(
		`SELECT date FROM character_active_days WHERE character = ? COLLATE NOCASE AND at >= ? ORDER BY date ASC`,
		character, since.Unix(),
	)
	if err != nil {
		return nil, fmt.Errorf("query active days: %w", err)
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, fmt.Errorf("scan active day: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

const snapshotCols = `character, taken_at, level, aa_ranks, tradeskill_total, spells_known, copper`

func scanSnapshot(scan func(dest ...any) error) (*Snapshot, error) {
	var (
		character string
		takenAt   int64
	)
	snap := &Snapshot{}
	if err := scan(&character, &takenAt, &snap.Level, &snap.AARanks, &snap.TradeskillTotal, &snap.SpellsKnown, &snap.Copper); err != nil {
		return nil, err
	}
	snap.Character = character
	snap.TakenAt = time.Unix(takenAt, 0)
	return snap, nil
}

// LatestSnapshot returns the most recent stored snapshot for a character.
func (s *Store) LatestSnapshot(character string) (*Snapshot, bool, error) {
	row := s.db.QueryRow(
		`SELECT `+snapshotCols+` FROM character_progress_snapshots
		 WHERE character = ? COLLATE NOCASE
		 ORDER BY taken_at DESC, id DESC LIMIT 1`, character)
	snap, err := scanSnapshot(row.Scan)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return snap, true, nil
}

// SnapshotAtOrBefore returns the latest stored snapshot for character taken
// at or before at — used to establish the "start of window" baseline for
// coin/total deltas in the recap.
func (s *Store) SnapshotAtOrBefore(character string, at time.Time) (*Snapshot, bool, error) {
	row := s.db.QueryRow(
		`SELECT `+snapshotCols+` FROM character_progress_snapshots
		 WHERE character = ? COLLATE NOCASE AND taken_at <= ?
		 ORDER BY taken_at DESC, id DESC LIMIT 1`, character, at.Unix())
	snap, err := scanSnapshot(row.Scan)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return snap, true, nil
}

// AppendSnapshot stores a snapshot row and returns its new ID.
func (s *Store) AppendSnapshot(snap Snapshot) (int64, error) {
	res, err := s.db.Exec(
		`INSERT INTO character_progress_snapshots
			(character, taken_at, level, aa_ranks, tradeskill_total, spells_known, copper, fingerprint, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		snap.Character, snap.TakenAt.Unix(), snap.Level, snap.AARanks, snap.TradeskillTotal,
		snap.SpellsKnown, snap.Copper, snap.Fingerprint(), time.Now().Unix(),
	)
	if err != nil {
		return 0, fmt.Errorf("insert progress snapshot: %w", err)
	}
	return res.LastInsertId()
}

// Fingerprint is a content hash used to skip storing a snapshot that is
// identical to the last one on record for the character (same idiom as
// trader.Snapshot.Fingerprint).
func (snap Snapshot) Fingerprint() string {
	var b strings.Builder
	b.WriteString(strconv.Itoa(snap.Level))
	b.WriteByte('|')
	b.WriteString(strconv.Itoa(snap.AARanks))
	b.WriteByte('|')
	b.WriteString(strconv.Itoa(snap.TradeskillTotal))
	b.WriteByte('|')
	b.WriteString(strconv.Itoa(snap.SpellsKnown))
	b.WriteByte('|')
	b.WriteString(strconv.FormatInt(snap.Copper, 10))
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:])
}
