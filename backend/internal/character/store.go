// Package character manages stored character profiles in user.db.
package character

import (
	"database/sql"
	"fmt"
	"strings"

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

// TradeskillEntry is a character's value in one tradeskill, from the quarmy
// "SkillID\tValue" section. Value is the raw server value; the API layer fills
// in Name and Cap and marks Untrained for the 254/255 sentinels.
type TradeskillEntry struct {
	SkillID   int    `json:"skill_id"`
	Value     int    `json:"value"`
	Name      string `json:"name,omitempty"`
	Cap       int    `json:"cap,omitempty"`
	Untrained bool   `json:"untrained"`
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
	// Additive migrations for columns added after initial release. SQLite
	// reports a "duplicate column name" error when the column already
	// exists, which is the expected path for any database that's already
	// been migrated; any other error should surface so we don't silently
	// proceed with a half-migrated schema.
	addColumns := []string{
		`ALTER TABLE characters ADD COLUMN base_str INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE characters ADD COLUMN base_sta INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE characters ADD COLUMN base_cha INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE characters ADD COLUMN base_dex INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE characters ADD COLUMN base_int INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE characters ADD COLUMN base_agi INTEGER NOT NULL DEFAULT 0`,
		`ALTER TABLE characters ADD COLUMN base_wis INTEGER NOT NULL DEFAULT 0`,
	}
	for _, stmt := range addColumns {
		if _, err := s.db.Exec(stmt); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
			return fmt.Errorf("add column: %w", err)
		}
	}

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
	// Tradeskill values from the quarmy "SkillID\tValue" section (Zeal 1.4.3+).
	// One row per skill id; value is the raw server value (254/255 = untrained
	// sentinels, kept as-is and classified at read time).
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS character_tradeskills (
			character_id INTEGER NOT NULL,
			skill_id     INTEGER NOT NULL,
			value        INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (character_id, skill_id),
			FOREIGN KEY (character_id) REFERENCES characters(id) ON DELETE CASCADE
		)
	`); err != nil {
		return err
	}
	// Per-character raid buff preset. Up to MaxRaidBuffSlots ordered entries
	// per character; empty table for a character means "use frontend default."
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS character_raid_buffs (
			character_id INTEGER NOT NULL,
			slot_index   INTEGER NOT NULL,
			spell_id     INTEGER NOT NULL,
			PRIMARY KEY (character_id, slot_index),
			FOREIGN KEY (character_id) REFERENCES characters(id) ON DELETE CASCADE
		)
	`); err != nil {
		return err
	}
	if err := s.migrateTasks(); err != nil {
		return err
	}
	if err := s.migrateWishlist(); err != nil {
		return err
	}
	if err := s.migrateFavoriteRecipes(); err != nil {
		return err
	}
	if err := s.migrateFactionWishlist(); err != nil {
		return err
	}
	if err := s.migrateFactionTally(); err != nil {
		return err
	}
	if err := s.migrateCustomLevelingRecipes(); err != nil {
		return err
	}
	if err := s.migrateUpgradeWeights(); err != nil {
		return err
	}
	if err := s.migrateUpgradeFocus(); err != nil {
		return err
	}
	return nil
}

// MaxRaidBuffSlots is the in-game limit on simultaneous beneficial buffs
// (EQ Classic / Project Quarm: 13 buff slots).
const MaxRaidBuffSlots = 13

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

// Delete removes the character with the given id along with all of its child
// rows. user.db never enables the foreign_keys pragma (no DSN sets it), so the
// schema's ON DELETE CASCADE clauses are inert — deleting a character would
// otherwise orphan its AAs, raid buffs, tasks/subtasks, wishlist, slot layout,
// and upgrade weights/focus forever (AUTOINCREMENT ids, so silent bloat). We
// delete the children explicitly in one transaction. Subtasks hang off
// character_tasks rather than characters directly, so they're cleared first
// via the parent task ids, before character_tasks itself is removed.
func (s *Store) Delete(id int) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("delete character: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.Exec(
		`DELETE FROM character_task_subtasks
		   WHERE task_id IN (SELECT id FROM character_tasks WHERE character_id = ?)`,
		id,
	); err != nil {
		return fmt.Errorf("delete character subtasks: %w", err)
	}

	// Table names are compile-time constants, not user input — safe to
	// interpolate (a table identifier can't be a bound parameter).
	for _, tbl := range []string{
		"character_tasks",
		"character_aas",
		"character_tradeskills",
		"character_raid_buffs",
		"character_upgrade_weights",
		"character_upgrade_focus",
		"character_wishlist",
		"character_wishlist_slot_layout",
	} {
		if _, err := tx.Exec(`DELETE FROM `+tbl+` WHERE character_id = ?`, id); err != nil {
			return fmt.Errorf("delete %s: %w", tbl, err)
		}
	}

	if _, err := tx.Exec(`DELETE FROM characters WHERE id = ?`, id); err != nil {
		return fmt.Errorf("delete character: %w", err)
	}
	return tx.Commit()
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

// ReplaceTradeskills replaces all stored tradeskill values for the character
// with the given id. Values are stored raw (including 254/255 sentinels).
func (s *Store) ReplaceTradeskills(characterID int, skills []TradeskillEntry) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.Exec(`DELETE FROM character_tradeskills WHERE character_id=?`, characterID); err != nil {
		return err
	}
	for _, ts := range skills {
		if _, err := tx.Exec(
			`INSERT INTO character_tradeskills (character_id, skill_id, value) VALUES (?, ?, ?)`,
			characterID, ts.SkillID, ts.Value,
		); err != nil {
			return fmt.Errorf("insert tradeskill %d: %w", ts.SkillID, err)
		}
	}
	return tx.Commit()
}

// ListTradeskills returns the raw stored tradeskill values for a character,
// ordered by skill id. Name/Cap/Untrained are left for the API layer to fill.
func (s *Store) ListTradeskills(characterID int) ([]TradeskillEntry, error) {
	rows, err := s.db.Query(
		`SELECT skill_id, value FROM character_tradeskills WHERE character_id=? ORDER BY skill_id`,
		characterID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []TradeskillEntry
	for rows.Next() {
		var ts TradeskillEntry
		if err := rows.Scan(&ts.SkillID, &ts.Value); err != nil {
			return nil, err
		}
		out = append(out, ts)
	}
	return out, rows.Err()
}

// ListRaidBuffs returns the saved raid-buff spell IDs for a character in
// slot order. Returns an empty slice (not an error) when nothing is saved —
// the frontend treats that as "use the default preset."
func (s *Store) ListRaidBuffs(characterID int) ([]int, error) {
	rows, err := s.db.Query(
		`SELECT spell_id FROM character_raid_buffs WHERE character_id=? ORDER BY slot_index`,
		characterID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]int, 0, MaxRaidBuffSlots)
	for rows.Next() {
		var id int
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// ReplaceRaidBuffs atomically replaces the saved raid-buff preset for a
// character. Caller must ensure len(spellIDs) <= MaxRaidBuffSlots; the
// method enforces this with an error rather than silently truncating.
func (s *Store) ReplaceRaidBuffs(characterID int, spellIDs []int) error {
	if len(spellIDs) > MaxRaidBuffSlots {
		return fmt.Errorf("too many raid buffs: %d (max %d)", len(spellIDs), MaxRaidBuffSlots)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err := tx.Exec(`DELETE FROM character_raid_buffs WHERE character_id=?`, characterID); err != nil {
		return err
	}
	for i, id := range spellIDs {
		if _, err := tx.Exec(
			`INSERT INTO character_raid_buffs (character_id, slot_index, spell_id) VALUES (?, ?, ?)`,
			characterID, i, id,
		); err != nil {
			return fmt.Errorf("insert raid buff slot %d: %w", i, err)
		}
	}
	return tx.Commit()
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
