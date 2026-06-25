package trader

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Store persists trader inventory snapshots to user.db. History is append-only:
// each captured export becomes one immutable row, and sale sessions are derived
// on read by diffing consecutive rows.
type Store struct {
	db *sql.DB
}

// OpenStore opens user.db at path and applies the trader migration.
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
		`CREATE TABLE IF NOT EXISTS trader_snapshots (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			character   TEXT    NOT NULL COLLATE NOCASE,
			taken_at    INTEGER NOT NULL,
			on_person   INTEGER NOT NULL DEFAULT 0,
			bank        INTEGER NOT NULL DEFAULT 0,
			satchel     TEXT    NOT NULL DEFAULT '[]',
			fingerprint TEXT    NOT NULL DEFAULT '',
			source_path TEXT    NOT NULL DEFAULT '',
			created_at  INTEGER NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS trader_snap_char_time
			ON trader_snapshots(character, taken_at)`,
	}
	for _, q := range stmts {
		if _, err := s.db.Exec(q); err != nil {
			return err
		}
	}
	return nil
}

// scanSnapshot builds a *Snapshot from a row scanner.
func scanSnapshot(scan func(dest ...any) error) (*Snapshot, error) {
	var (
		character  string
		takenAt    int64
		onPerson   int64
		bank       int64
		satchelRaw string
		source     string
	)
	if err := scan(&character, &takenAt, &onPerson, &bank, &satchelRaw, &source); err != nil {
		return nil, err
	}
	snap := &Snapshot{
		Character:      character,
		TakenAt:        time.Unix(takenAt, 0),
		OnPersonCopper: onPerson,
		BankCopper:     bank,
		SourcePath:     source,
		Satchel:        []SatchelItem{},
	}
	if satchelRaw != "" {
		if err := json.Unmarshal([]byte(satchelRaw), &snap.Satchel); err != nil {
			return nil, fmt.Errorf("decode satchel: %w", err)
		}
	}
	return snap, nil
}

const snapshotCols = `character, taken_at, on_person, bank, satchel, source_path`

// LatestSnapshot returns the most recent stored snapshot for a character.
func (s *Store) LatestSnapshot(character string) (*Snapshot, bool, error) {
	row := s.db.QueryRow(
		`SELECT `+snapshotCols+` FROM trader_snapshots
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

// ListSnapshots returns all snapshots for a character oldest-first.
func (s *Store) ListSnapshots(character string) ([]*Snapshot, error) {
	rows, err := s.db.Query(
		`SELECT `+snapshotCols+` FROM trader_snapshots
		 WHERE character = ? COLLATE NOCASE
		 ORDER BY taken_at ASC, id ASC`, character)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []*Snapshot
	for rows.Next() {
		snap, err := scanSnapshot(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, snap)
	}
	return out, rows.Err()
}

// AppendSnapshot stores a snapshot row and returns its new ID.
func (s *Store) AppendSnapshot(snap *Snapshot) (int64, error) {
	satchel, err := json.Marshal(snap.Satchel)
	if err != nil {
		return 0, fmt.Errorf("encode satchel: %w", err)
	}
	res, err := s.db.Exec(
		`INSERT INTO trader_snapshots
			(character, taken_at, on_person, bank, satchel, fingerprint, source_path, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		snap.Character, snap.TakenAt.Unix(), snap.OnPersonCopper, snap.BankCopper,
		string(satchel), snap.Fingerprint(), snap.SourcePath, time.Now().Unix())
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// SnapshotCount returns how many snapshots are stored for a character.
func (s *Store) SnapshotCount(character string) (int, error) {
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(*) FROM trader_snapshots WHERE character = ? COLLATE NOCASE`,
		character).Scan(&n)
	return n, err
}

// CharactersWithSnapshots returns the distinct characters that have at least
// one stored snapshot, sorted.
func (s *Store) CharactersWithSnapshots() ([]string, error) {
	rows, err := s.db.Query(
		`SELECT DISTINCT character FROM trader_snapshots ORDER BY character COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}
