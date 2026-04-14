package backup

import (
	"database/sql"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Store persists backup metadata in user.db.
type Store struct {
	db *sql.DB
}

// OpenStore opens (or creates) the user.db at path and applies schema migrations.
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
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS backups (
			id         TEXT    NOT NULL PRIMARY KEY,
			name       TEXT    NOT NULL,
			notes      TEXT    NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL,
			size_bytes INTEGER NOT NULL DEFAULT 0,
			file_count INTEGER NOT NULL DEFAULT 0
		)
	`)
	return err
}

// Insert persists a new backup record.
func (s *Store) Insert(b *Backup) error {
	_, err := s.db.Exec(
		`INSERT INTO backups (id, name, notes, created_at, size_bytes, file_count)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		b.ID, b.Name, b.Notes, b.CreatedAt.Unix(), b.SizeBytes, b.FileCount,
	)
	if err != nil {
		return fmt.Errorf("insert backup: %w", err)
	}
	return nil
}

// List returns all backups ordered newest-first.
func (s *Store) List() ([]*Backup, error) {
	rows, err := s.db.Query(
		`SELECT id, name, notes, created_at, size_bytes, file_count
		 FROM backups ORDER BY created_at DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list backups: %w", err)
	}
	defer rows.Close()

	var backups []*Backup
	for rows.Next() {
		b, err := scanBackup(rows)
		if err != nil {
			return nil, err
		}
		backups = append(backups, b)
	}
	return backups, rows.Err()
}

// Get returns the backup with the given ID, or sql.ErrNoRows if not found.
func (s *Store) Get(id string) (*Backup, error) {
	row := s.db.QueryRow(
		`SELECT id, name, notes, created_at, size_bytes, file_count
		 FROM backups WHERE id = ?`, id,
	)
	b, err := scanBackup(row)
	if err != nil {
		return nil, fmt.Errorf("get backup %s: %w", id, err)
	}
	return b, nil
}

// Delete removes the backup record with the given ID.
func (s *Store) Delete(id string) error {
	_, err := s.db.Exec(`DELETE FROM backups WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete backup %s: %w", id, err)
	}
	return nil
}

type scanner interface {
	Scan(...any) error
}

func scanBackup(row scanner) (*Backup, error) {
	var b Backup
	var unixSec int64
	if err := row.Scan(&b.ID, &b.Name, &b.Notes, &unixSec, &b.SizeBytes, &b.FileCount); err != nil {
		return nil, err
	}
	b.CreatedAt = time.Unix(unixSec, 0).UTC()
	return &b, nil
}
