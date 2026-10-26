package character

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// MockLoadout is a saved what-if gear/AA set for the Spell Modifiers "Test
// Resolution" panel. Both maps are overrides on top of the character's real
// Quarmy export — anything absent keeps the actually worn item / trained rank.
//
// Gear is keyed by the physical worn-slot key the upgrade finder uses ("ear1",
// "head", "finger2", …); an item id of 0 means "slot left empty". AAs is
// keyed by altadv_vars.eqmacid; a rank of 0 means "untrained".
type MockLoadout struct {
	ID          int            `json:"id"`
	CharacterID int            `json:"character_id"`
	Name        string         `json:"name"`
	Gear        map[string]int `json:"gear"`
	AAs         map[int]int    `json:"aas"`
	CreatedAt   int64          `json:"created_at"`
	UpdatedAt   int64          `json:"updated_at"`
}

// ErrMockLoadoutNameTaken is returned when a character already has a loadout
// with the requested name.
var ErrMockLoadoutNameTaken = fmt.Errorf("a loadout with that name already exists")

func (s *Store) migrateMockLoadouts() error {
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS character_mock_loadouts (
			id           INTEGER PRIMARY KEY AUTOINCREMENT,
			character_id INTEGER NOT NULL,
			name         TEXT    NOT NULL,
			gear_json    TEXT    NOT NULL DEFAULT '{}',
			aas_json     TEXT    NOT NULL DEFAULT '{}',
			created_at   INTEGER NOT NULL,
			updated_at   INTEGER NOT NULL,
			UNIQUE (character_id, name),
			FOREIGN KEY (character_id) REFERENCES characters(id) ON DELETE CASCADE
		)
	`); err != nil {
		return err
	}
	_, err := s.db.Exec(
		`CREATE INDEX IF NOT EXISTS idx_character_mock_loadouts_char
		 ON character_mock_loadouts(character_id)`,
	)
	return err
}

// ListMockLoadouts returns a character's saved loadouts ordered by name.
func (s *Store) ListMockLoadouts(characterID int) ([]MockLoadout, error) {
	rows, err := s.db.Query(
		`SELECT id, character_id, name, gear_json, aas_json, created_at, updated_at
		 FROM character_mock_loadouts
		 WHERE character_id = ?
		 ORDER BY name COLLATE NOCASE`,
		characterID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MockLoadout{}
	for rows.Next() {
		l, err := scanMockLoadout(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// GetMockLoadout returns one loadout; ok is false when it doesn't exist or
// belongs to another character.
func (s *Store) GetMockLoadout(characterID, id int) (MockLoadout, bool, error) {
	row := s.db.QueryRow(
		`SELECT id, character_id, name, gear_json, aas_json, created_at, updated_at
		 FROM character_mock_loadouts
		 WHERE character_id = ? AND id = ?`,
		characterID, id,
	)
	l, err := scanMockLoadout(row)
	if err == sql.ErrNoRows {
		return MockLoadout{}, false, nil
	}
	if err != nil {
		return MockLoadout{}, false, err
	}
	return l, true, nil
}

// CreateMockLoadout saves a new loadout for the character.
func (s *Store) CreateMockLoadout(characterID int, name string, gear map[string]int, aas map[int]int) (MockLoadout, error) {
	gearJSON, aasJSON, err := encodeMockMaps(gear, aas)
	if err != nil {
		return MockLoadout{}, err
	}
	now := time.Now().Unix()
	res, err := s.db.Exec(
		`INSERT INTO character_mock_loadouts
		 (character_id, name, gear_json, aas_json, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		characterID, name, gearJSON, aasJSON, now, now,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return MockLoadout{}, ErrMockLoadoutNameTaken
		}
		return MockLoadout{}, fmt.Errorf("create mock loadout: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return MockLoadout{}, err
	}
	l, _, err := s.GetMockLoadout(characterID, int(id))
	return l, err
}

// UpdateMockLoadout replaces a loadout's name and contents. ok is false when
// the loadout doesn't exist for this character.
func (s *Store) UpdateMockLoadout(characterID, id int, name string, gear map[string]int, aas map[int]int) (MockLoadout, bool, error) {
	gearJSON, aasJSON, err := encodeMockMaps(gear, aas)
	if err != nil {
		return MockLoadout{}, false, err
	}
	res, err := s.db.Exec(
		`UPDATE character_mock_loadouts
		 SET name = ?, gear_json = ?, aas_json = ?, updated_at = ?
		 WHERE character_id = ? AND id = ?`,
		name, gearJSON, aasJSON, time.Now().Unix(), characterID, id,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return MockLoadout{}, false, ErrMockLoadoutNameTaken
		}
		return MockLoadout{}, false, fmt.Errorf("update mock loadout: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return MockLoadout{}, false, nil
	}
	return s.GetMockLoadout(characterID, id)
}

// DeleteMockLoadout removes a loadout. Deleting a missing one is a no-op.
func (s *Store) DeleteMockLoadout(characterID, id int) error {
	_, err := s.db.Exec(
		`DELETE FROM character_mock_loadouts WHERE character_id = ? AND id = ?`,
		characterID, id,
	)
	return err
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanMockLoadout(r rowScanner) (MockLoadout, error) {
	var l MockLoadout
	var gearJSON, aasJSON string
	if err := r.Scan(&l.ID, &l.CharacterID, &l.Name, &gearJSON, &aasJSON, &l.CreatedAt, &l.UpdatedAt); err != nil {
		return MockLoadout{}, err
	}
	l.Gear = map[string]int{}
	l.AAs = map[int]int{}
	if err := json.Unmarshal([]byte(gearJSON), &l.Gear); err != nil {
		return MockLoadout{}, fmt.Errorf("decode loadout %d gear: %w", l.ID, err)
	}
	if err := json.Unmarshal([]byte(aasJSON), &l.AAs); err != nil {
		return MockLoadout{}, fmt.Errorf("decode loadout %d aas: %w", l.ID, err)
	}
	return l, nil
}

func encodeMockMaps(gear map[string]int, aas map[int]int) (string, string, error) {
	if gear == nil {
		gear = map[string]int{}
	}
	if aas == nil {
		aas = map[int]int{}
	}
	g, err := json.Marshal(gear)
	if err != nil {
		return "", "", err
	}
	a, err := json.Marshal(aas)
	if err != nil {
		return "", "", err
	}
	return string(g), string(a), nil
}

func isUniqueViolation(err error) bool {
	return strings.Contains(err.Error(), "UNIQUE constraint failed")
}
