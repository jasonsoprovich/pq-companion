// Package character manages stored character profiles in user.db.
package character

import (
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite"
)

// Character represents a player character profile.
type Character struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	Class   int    `json:"class"`    // -1=not set, 0-14=EQ class index
	Race    int    `json:"race"`     // -1=not set, EQ race id (1=Human, 2=Barbarian, …)
	Level   int    `json:"level"`    // 1-60
	BaseSTR int    `json:"base_str"` // base stats from quarmy.txt; 0 = not yet imported
	BaseSTA int    `json:"base_sta"`
	BaseCHA int    `json:"base_cha"`
	BaseDEX int    `json:"base_dex"`
	BaseINT int    `json:"base_int"`
	BaseAGI int    `json:"base_agi"`
	BaseWIS int    `json:"base_wis"`
}

// AAEntry is a purchased AA ability with its current rank.
type AAEntry struct {
	AAID int    `json:"aa_id"`
	Rank int    `json:"rank"`
	Name string `json:"name,omitempty"`
}

// Store persists character profiles in user.db.
type Store struct {
	db *sql.DB
}

// OpenStore opens (or creates) user.db at path and runs schema migrations.
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
		return nil, fmt.Errorf("migrate characters: %w", err)
	}
	return s, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate() error {
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS characters (
			id    INTEGER PRIMARY KEY AUTOINCREMENT,
			name  TEXT    NOT NULL UNIQUE COLLATE NOCASE,
			class INTEGER NOT NULL DEFAULT -1,
			race  INTEGER NOT NULL DEFAULT -1,
			level INTEGER NOT NULL DEFAULT 1
		)
	`); err != nil {
		return err
	}
	// Additive migrations for columns added after initial release.
	_, _ = s.db.Exec(`ALTER TABLE characters ADD COLUMN race    INTEGER NOT NULL DEFAULT -1`)
	_, _ = s.db.Exec(`ALTER TABLE characters ADD COLUMN base_str INTEGER NOT NULL DEFAULT 0`)
	_, _ = s.db.Exec(`ALTER TABLE characters ADD COLUMN base_sta INTEGER NOT NULL DEFAULT 0`)
	_, _ = s.db.Exec(`ALTER TABLE characters ADD COLUMN base_cha INTEGER NOT NULL DEFAULT 0`)
	_, _ = s.db.Exec(`ALTER TABLE characters ADD COLUMN base_dex INTEGER NOT NULL DEFAULT 0`)
	_, _ = s.db.Exec(`ALTER TABLE characters ADD COLUMN base_int INTEGER NOT NULL DEFAULT 0`)
	_, _ = s.db.Exec(`ALTER TABLE characters ADD COLUMN base_agi INTEGER NOT NULL DEFAULT 0`)
	_, _ = s.db.Exec(`ALTER TABLE characters ADD COLUMN base_wis INTEGER NOT NULL DEFAULT 0`)

	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS character_aas (
			character_id INTEGER NOT NULL,
			aa_id        INTEGER NOT NULL,
			rank         INTEGER NOT NULL DEFAULT 1,
			PRIMARY KEY (character_id, aa_id),
			FOREIGN KEY (character_id) REFERENCES characters(id) ON DELETE CASCADE
		)
	`); err != nil {
		return err
	}
	if err := s.migrateTasks(); err != nil {
		return err
	}
	return nil
}

// List returns all stored characters ordered by name.
func (s *Store) List() ([]Character, error) {
	rows, err := s.db.Query(`
		SELECT id, name, class, race, level,
		       base_str, base_sta, base_cha, base_dex, base_int, base_agi, base_wis
		FROM characters ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Character
	for rows.Next() {
		var c Character
		if err := rows.Scan(&c.ID, &c.Name, &c.Class, &c.Race, &c.Level,
			&c.BaseSTR, &c.BaseSTA, &c.BaseCHA, &c.BaseDEX, &c.BaseINT, &c.BaseAGI, &c.BaseWIS); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// Create inserts a new character and returns the created record.
func (s *Store) Create(name string, class, race, level int) (Character, error) {
	res, err := s.db.Exec(
		`INSERT INTO characters (name, class, race, level) VALUES (?, ?, ?, ?)`,
		name, class, race, level,
	)
	if err != nil {
		return Character{}, fmt.Errorf("create character: %w", err)
	}
	id, _ := res.LastInsertId()
	return Character{ID: int(id), Name: name, Class: class, Race: race, Level: level}, nil
}

// Delete removes the character with the given id.
func (s *Store) Delete(id int) error {
	_, err := s.db.Exec(`DELETE FROM characters WHERE id=?`, id)
	return err
}

// Get returns the character with the given id.
func (s *Store) Get(id int) (Character, bool, error) {
	var c Character
	err := s.db.QueryRow(
		`SELECT id, name, class, race, level,
		        base_str, base_sta, base_cha, base_dex, base_int, base_agi, base_wis
		 FROM characters WHERE id = ?`,
		id,
	).Scan(&c.ID, &c.Name, &c.Class, &c.Race, &c.Level,
		&c.BaseSTR, &c.BaseSTA, &c.BaseCHA, &c.BaseDEX, &c.BaseINT, &c.BaseAGI, &c.BaseWIS)
	if err == sql.ErrNoRows {
		return Character{}, false, nil
	}
	if err != nil {
		return Character{}, false, err
	}
	return c, true, nil
}

// GetByName returns the character matching name (case-insensitive).
func (s *Store) GetByName(name string) (Character, bool, error) {
	var c Character
	err := s.db.QueryRow(
		`SELECT id, name, class, race, level,
		        base_str, base_sta, base_cha, base_dex, base_int, base_agi, base_wis
		 FROM characters WHERE name = ? COLLATE NOCASE`,
		name,
	).Scan(&c.ID, &c.Name, &c.Class, &c.Race, &c.Level,
		&c.BaseSTR, &c.BaseSTA, &c.BaseCHA, &c.BaseDEX, &c.BaseINT, &c.BaseAGI, &c.BaseWIS)
	if err == sql.ErrNoRows {
		return Character{}, false, nil
	}
	if err != nil {
		return Character{}, false, err
	}
	return c, true, nil
}

// UpdatePersona saves level/class/race imported from quarmy.txt. Class and race
// must already be in the app's internal scheme (class 0-indexed, race 1-indexed).
func (s *Store) UpdatePersona(id, class, race, level int) error {
	_, err := s.db.Exec(
		`UPDATE characters SET class=?, race=?, level=? WHERE id=?`,
		class, race, level, id,
	)
	return err
}

// UpdateStats saves base stats imported from quarmy.txt for the character with the given id.
func (s *Store) UpdateStats(id, baseStr, baseSta, baseCha, baseDex, baseInt, baseAgi, baseWis int) error {
	_, err := s.db.Exec(
		`UPDATE characters SET base_str=?, base_sta=?, base_cha=?, base_dex=?, base_int=?, base_agi=?, base_wis=? WHERE id=?`,
		baseStr, baseSta, baseCha, baseDex, baseInt, baseAgi, baseWis, id,
	)
	return err
}

// ReplaceAAs replaces all stored AA entries for the character with the given id.
func (s *Store) ReplaceAAs(characterID int, aas []AAEntry) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.Exec(`DELETE FROM character_aas WHERE character_id=?`, characterID); err != nil {
		return err
	}
	for _, aa := range aas {
		if _, err := tx.Exec(
			`INSERT INTO character_aas (character_id, aa_id, rank) VALUES (?, ?, ?)`,
			characterID, aa.AAID, aa.Rank,
		); err != nil {
			return fmt.Errorf("insert aa %d: %w", aa.AAID, err)
		}
	}
	return tx.Commit()
}

// ListAAs returns all stored AA entries for the character with the given id.
func (s *Store) ListAAs(characterID int) ([]AAEntry, error) {
	rows, err := s.db.Query(
		`SELECT aa_id, rank FROM character_aas WHERE character_id=? AND rank > 0 ORDER BY aa_id`,
		characterID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AAEntry
	for rows.Next() {
		var aa AAEntry
		if err := rows.Scan(&aa.AAID, &aa.Rank); err != nil {
			return nil, err
		}
		out = append(out, aa)
	}
	return out, rows.Err()
}

// Names returns the set of stored character names (case-preserved).
func (s *Store) Names() (map[string]struct{}, error) {
	rows, err := s.db.Query(`SELECT name FROM characters`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make(map[string]struct{})
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out[name] = struct{}{}
	}
	return out, rows.Err()
}
