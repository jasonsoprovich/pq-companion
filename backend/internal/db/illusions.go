package db

import (
	"fmt"
	"strings"
)

// Illusion is a race a spell can turn its target into (SPA 58, Illusion),
// with the spells that do.
type Illusion struct {
	Race int `json:"race"`
	// Name is the race's name, or the first spell's where the race has
	// none ("Unknown 143" is Treeform's tree).
	Name   string   `json:"name"`
	Spells []string `json:"spells"`
}

// spaIllusion is the Illusion spell effect; its base value is the race
// (-1 toggles gender, which isn't a race to show).
const spaIllusion = 58

// Illusions returns the races illusion spells turn their target into, by
// name. Every effect slot is searched: some spells illusion in a later
// slot.
func (db *DB) Illusions() ([]Illusion, error) {
	var parts []string
	for i := 1; i <= 12; i++ {
		parts = append(parts, fmt.Sprintf(
			"SELECT id, name, effect_base_value%d AS race FROM spells_new WHERE effectid%d = %d AND effect_base_value%d > 0",
			i, i, spaIllusion, i))
	}
	rows, err := db.Query(`
		SELECT s.race, COALESCE(r.name, ''), s.name
		FROM (` + strings.Join(parts, " UNION ") + `) s
		LEFT JOIN races r ON r.id = s.race
		WHERE s.name != ''
		ORDER BY s.race, s.name LIKE 'Illusion:%' DESC, s.name`)
	if err != nil {
		return nil, fmt.Errorf("illusions: %w", err)
	}
	defer rows.Close()
	var out []Illusion
	for rows.Next() {
		var (
			race           int
			raceName, name string
		)
		if err := rows.Scan(&race, &raceName, &name); err != nil {
			return nil, fmt.Errorf("scan illusion: %w", err)
		}
		if len(out) == 0 || out[len(out)-1].Race != race {
			label := raceName
			if label == "" || strings.HasPrefix(label, "Unknown") {
				label = strings.TrimPrefix(name, "Illusion: ")
			}
			out = append(out, Illusion{Race: race, Name: label})
		}
		il := &out[len(out)-1]
		if len(il.Spells) == 0 || il.Spells[len(il.Spells)-1] != name {
			il.Spells = append(il.Spells, name)
		}
	}
	return out, rows.Err()
}
