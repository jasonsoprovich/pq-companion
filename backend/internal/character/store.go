// Package character manages stored character profiles in user.db.
package character

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// Character represents a player character profile.
type Character struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Class int    `json:"class"` // -1=not set, 0-14=EQ class index
	Level int    `json:"level"` // 1-60
}

// Store persists character profiles in user.db.
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
		return nil, fmt.Errorf("migrate characters: %w", err)
	}
	return s, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS characters (
			id    INTEGER PRIMARY KEY AUTOINCREMENT,
			name  TEXT    NOT NULL UNIQUE COLLATE NOCASE,
			class INTEGER NOT NULL DEFAULT -1,
			level INTEGER NOT NULL DEFAULT 1
		)
	`)
	return err
}

// List returns all stored characters ordered by name.
func (s *Store) List() ([]Character, error) {
	rows, err := s.db.Query(`SELECT id, name, class, level FROM characters ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Character
	for rows.Next() {
		var c Character
		if err := rows.Scan(&c.ID, &c.Name, &c.Class, &c.Level); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Create inserts a new character and returns the created record.
func (s *Store) Create(name string, class, level int) (Character, error) {
	res, err := s.db.Exec(
		`INSERT INTO characters (name, class, level) VALUES (?, ?, ?)`,
		name, class, level,
	)
	if err != nil {
		return Character{}, fmt.Errorf("create character: %w", err)
	}
	id, _ := res.LastInsertId()
	return Character{ID: int(id), Name: name, Class: class, Level: level}, nil
}

// Update replaces name/class/level for the character with the given id.
func (s *Store) Update(id int, name string, class, level int) error {
	_, err := s.db.Exec(
		`UPDATE characters SET name=?, class=?, level=? WHERE id=?`,
		name, class, level, id,
	)
	return err
}

// Delete removes the character with the given id.
func (s *Store) Delete(id int) error {
	_, err := s.db.Exec(`DELETE FROM characters WHERE id=?`, id)
	return err
}

// GetByName returns the character matching name (case-insensitive).
func (s *Store) GetByName(name string) (Character, bool, error) {
	var c Character
	err := s.db.QueryRow(
		`SELECT id, name, class, level FROM characters WHERE name = ? COLLATE NOCASE`,
		name,
	).Scan(&c.ID, &c.Name, &c.Class, &c.Level)
	if err == sql.ErrNoRows {
		return Character{}, false, nil
	}
	if err != nil {
		return Character{}, false, err
	}
	return c, true, nil
}
