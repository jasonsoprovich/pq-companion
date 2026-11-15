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
	// Face is 0-7.
	Face int `json:"face"`
	// HairColor is a palette index (0-23), or -1 for the race's default.
	HairColor int `json:"hair_color"`
	// Beard is the Luclin beard style, or -1 for none.
	Beard int `json:"beard"`
	// BeardColor is a palette index, or -1 to follow the hair.
	BeardColor int `json:"beard_color"`
	// Eyes is the Luclin eye colour, 1-10, or 0 for the model's own.
	Eyes int `json:"eyes"`
}

// appearanceColumns are the columns added after the table was made, with
// their definitions.
var appearanceColumns = []struct{ name, def string }{
	{"hair", "INTEGER NOT NULL DEFAULT 0"},
	{"face", "INTEGER NOT NULL DEFAULT 0"},
	{"hair_color", "INTEGER NOT NULL DEFAULT -1"},
	{"beard", "INTEGER NOT NULL DEFAULT -1"},
	{"beard_color", "INTEGER NOT NULL DEFAULT -1"},
	{"eyes", "INTEGER NOT NULL DEFAULT 0"},
}

func (s *Store) migrateAppearance() error {
	_, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS character_appearance (
			name   TEXT PRIMARY KEY COLLATE NOCASE,
			gender INTEGER NOT NULL DEFAULT 0,
			luclin INTEGER NOT NULL DEFAULT 0
		)
	`)
	if err != nil {
		return err
	}
	for _, c := range appearanceColumns {
		var n int
		if err := s.db.QueryRow(
			`SELECT COUNT(*) FROM pragma_table_info('character_appearance') WHERE name = ?`, c.name,
		).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			// Names and definitions are the constants above.
			if _, err := s.db.Exec(`ALTER TABLE character_appearance ADD COLUMN ` + c.name + ` ` + c.def); err != nil {
				return fmt.Errorf("add appearance column %s: %w", c.name, err)
			}
		}
	}
	return nil
}

// DefaultAppearance is a character nobody has set an appearance for.
func DefaultAppearance(name string) Appearance {
	return Appearance{Name: name, HairColor: -1, Beard: -1, BeardColor: -1}
}

// GetAppearance returns a character's appearance; ok is false when none
// has been saved (DefaultAppearance, a male original model).
func (s *Store) GetAppearance(name string) (Appearance, bool, error) {
	a := DefaultAppearance(name)
	var luclin int
	err := s.db.QueryRow(`
		SELECT gender, luclin, hair, face, hair_color, beard, beard_color, eyes
		FROM character_appearance WHERE name = ?`, name,
	).Scan(&a.Gender, &luclin, &a.Hair, &a.Face, &a.HairColor, &a.Beard, &a.BeardColor, &a.Eyes)
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
	clamp := func(v *int, lo, hi, def int) {
		if *v < lo || *v > hi {
			*v = def
		}
	}
	clamp(&a.Hair, -1, 9, 0)
	clamp(&a.Face, 0, 9, 0)
	clamp(&a.HairColor, -1, 23, -1)
	clamp(&a.Beard, -1, 9, -1)
	clamp(&a.BeardColor, -1, 23, -1)
	clamp(&a.Eyes, 0, 10, 0)
	luclin := 0
	if a.Luclin {
		luclin = 1
	}
	_, err := s.db.Exec(`
		INSERT INTO character_appearance
			(name, gender, luclin, hair, face, hair_color, beard, beard_color, eyes)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(name) DO UPDATE SET gender = excluded.gender, luclin = excluded.luclin,
			hair = excluded.hair, face = excluded.face, hair_color = excluded.hair_color,
			beard = excluded.beard, beard_color = excluded.beard_color, eyes = excluded.eyes`,
		a.Name, a.Gender, luclin, a.Hair, a.Face, a.HairColor, a.Beard, a.BeardColor, a.Eyes)
	if err != nil {
		return fmt.Errorf("set appearance: %w", err)
	}
	return nil
}
