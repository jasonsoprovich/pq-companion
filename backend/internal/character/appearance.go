package character

import (
	"database/sql"
	"fmt"
)

// Appearance is how a character is drawn in the dressing room and the 3D
// views. The game logs neither gender nor face, so the user picks them.
// Keyed by name, like the hidden list, so it works for any character.
type Appearance struct {
	Name string `json:"name"`
	// Gender is 0 male, 1 female.
	Gender int `json:"gender"`
	// Luclin draws the Luclin-era model rather than the original.
	Luclin bool `json:"luclin"`
	// Hair is the Luclin model's hairstyle: 0-2, or -1 for none.
	Hair int `json:"hair"`
}

func (s *Store) migrateAppearance() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS character_appearance (
			name   TEXT PRIMARY KEY COLLATE NOCASE,
			gender INTEGER NOT NULL DEFAULT 0,
			luclin INTEGER NOT NULL DEFAULT 0,
			hair   INTEGER NOT NULL DEFAULT 0
		)
	`)
	if err != nil {
		return err
	}
	// Tables made before hairstyles.
	var n int
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM pragma_table_info('character_appearance') WHERE name = 'hair'`,
	).Scan(&n); err != nil {
		return err
	}
	if n == 0 {
		_, err = s.db.Exec(`ALTER TABLE character_appearance ADD COLUMN hair INTEGER NOT NULL DEFAULT 0`)
	}
	return err
}

// GetAppearance returns a character's appearance; ok is false when none
// has been saved (the zero value, a male original model, is the default).
func (s *Store) GetAppearance(name string) (Appearance, bool, error) {
	a := Appearance{Name: name}
	var luclin int
	err := s.db.QueryRow(
		`SELECT gender, luclin, hair FROM character_appearance WHERE name = ?`, name,
	).Scan(&a.Gender, &luclin, &a.Hair)
	if err == sql.ErrNoRows {
		return a, false, nil
	}
	if err != nil {
		return a, false, fmt.Errorf("get appearance: %w", err)
	}
	a.Luclin = luclin != 0
	return a, true, nil
}

// SetAppearance saves a character's appearance.
func (s *Store) SetAppearance(a Appearance) error {
	if a.Name == "" {
		return fmt.Errorf("set appearance: no name")
	}
	if a.Gender != 1 {
		a.Gender = 0
	}
	if a.Hair < -1 || a.Hair > 9 {
		a.Hair = 0
	}
	luclin := 0
	if a.Luclin {
		luclin = 1
	}
	_, err := s.db.Exec(`
		INSERT INTO character_appearance (name, gender, luclin, hair) VALUES (?, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET gender = excluded.gender, luclin = excluded.luclin,
			hair = excluded.hair`,
		a.Name, a.Gender, luclin, a.Hair)
	if err != nil {
		return fmt.Errorf("set appearance: %w", err)
	}
	return nil
}
