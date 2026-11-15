package db

import (
	"fmt"
	"sort"
	"strings"
)

// Illusion is one look illusion spells give their target (SPA 58,
// Illusion): a race, in a texture set and, for some, a gender. Spells that
// give the same look are listed together.
type Illusion struct {
	Race int `json:"race"`
	// Texture is the race's texture set (an elemental's earth, fire, water
	// or air), and Gender the gender it's shown as: -1 for the target's
	// own (the player races), else 0 male, 1 female or 2 neutral.
	Texture int `json:"texture"`
	Gender  int `json:"gender"`
	// Name is the race's name, or where the race has several looks (or no
	// name, "Unknown 143" being Treeform's tree), a spell's (see lookSpell).
	Name   string   `json:"name"`
	Spells []string `json:"spells"`
}

// spaIllusion is the Illusion spell effect; its base value is the race
// (-1 toggles gender, which isn't a race to show).
const spaIllusion = 58

// illusionTextures is the texture the server gives each illusion that
// isn't its race's first, by spell id: the spell data has no texture, so
// EQMacEmu picks it by spell (Mob::ApplyIllusion, zone/mob.cpp).
var illusionTextures = map[int]int{
	// Fire elemental, and the Howler and Hunter wolves.
	598: 1, 2795: 1, 2796: 1, 2797: 1, 3856: 1, 1562: 1, 1563: 1,
	// Water elemental, and the Great Wolf.
	599: 2, 2798: 2, 2799: 2, 2800: 2, 3857: 2, 427: 2,
	// Air elemental, and the Greater Wolf.
	597: 3, 2789: 3, 2790: 3, 2791: 3, 3854: 3, 426: 3,
}

// illusionGenders is the gender an illusion forces, by spell id (as
// above): Scaled Wolf is a female wolf.
var illusionGenders = map[int]int{3586: 1}

// playableRaces are the races an illusion keeps its target's gender on;
// on any other the server shows the neutral model (Mob::GetDefaultGender;
// none of its male- or female-only races are illusions).
var playableRaces = map[int]bool{
	1: true, 2: true, 3: true, 4: true, 5: true, 6: true, 7: true, 8: true,
	9: true, 10: true, 11: true, 12: true, 128: true, 130: true, 330: true,
}

// postPoPIllusions are player spells in the data that came after Planes of
// Power and can't be had: Illusion: Froglok (Legacy of Ykesha; its scroll
// drops nowhere and no merchant sells it).
var postPoPIllusions = map[int]bool{3063: true}

// raceNames renames races whose name in the data isn't what players call
// the look.
var raceNames = map[string]string{
	"Were Wolf":         "Werewolf",
	"Old Froglok Ghoul": "Froglok",
}

// genderNeutral is the neutral gender.
const genderNeutral = 2

// Illusions returns the looks illusion spells give their target, by race
// then texture. Every effect slot is searched: some spells illusion in a
// later slot. Only spells a player can have are counted: one a class casts
// (an AA's spell is a class's at level 254) or an item clicks, procs or
// wears; the rest are NPC spells (Polymorphic Mayhem's goblin, Stonetouch).
func (db *DB) Illusions() ([]Illusion, error) {
	var parts []string
	for i := 1; i <= 12; i++ {
		parts = append(parts, fmt.Sprintf(
			"SELECT id, name, effect_base_value%d AS race, "+
				"MIN(classes1, classes2, classes3, classes4, classes5, classes6, classes7, classes8, "+
				"classes9, classes10, classes11, classes12, classes13, classes14, classes15) AS lvl "+
				"FROM spells_new WHERE effectid%d = %d AND effect_base_value%d > 0",
			i, i, spaIllusion, i))
	}
	rows, err := db.Query(`
		SELECT s.id, s.race, COALESCE(r.name, ''), s.name
		FROM (` + strings.Join(parts, " UNION ") + `) s
		LEFT JOIN races r ON r.id = s.race
		WHERE s.name != ''
		  AND (s.lvl < 255 OR s.id IN (
		      SELECT clickeffect FROM items UNION SELECT proceffect FROM items
		      UNION SELECT worneffect FROM items))
		ORDER BY s.race, s.name LIKE 'Illusion:%' DESC, s.name`)
	if err != nil {
		return nil, fmt.Errorf("illusions: %w", err)
	}
	defer rows.Close()
	type key struct{ race, texture, gender int }
	var (
		out      []Illusion
		index    = map[key]int{}
		looks    = map[int]int{} // looks per race
		raceName = map[int]string{}
	)
	for rows.Next() {
		var (
			id, race     int
			rname, spell string
		)
		if err := rows.Scan(&id, &race, &rname, &spell); err != nil {
			return nil, fmt.Errorf("scan illusion: %w", err)
		}
		if postPoPIllusions[id] {
			continue
		}
		gender, ok := illusionGenders[id]
		switch {
		case ok:
		case playableRaces[race]:
			gender = -1
		default:
			gender = genderNeutral
		}
		k := key{race, illusionTextures[id], gender}
		i, seen := index[k]
		if !seen {
			i = len(out)
			index[k] = i
			looks[race]++
			raceName[race] = rname
			out = append(out, Illusion{Race: race, Texture: k.texture, Gender: gender})
		}
		il := &out[i]
		if len(il.Spells) == 0 || il.Spells[len(il.Spells)-1] != spell {
			il.Spells = append(il.Spells, spell)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		il := &out[i]
		name := raceName[il.Race]
		if name == "" || strings.HasPrefix(name, "Unknown") || looks[il.Race] > 1 {
			name = strings.TrimPrefix(lookSpell(il.Spells, name), "Illusion: ")
		}
		if n, ok := raceNames[name]; ok {
			name = n
		}
		il.Name = name
	}
	sort.SliceStable(out, func(a, b int) bool { return out[a].Name < out[b].Name })
	return out, nil
}

// lookSpell picks the spell to name a look by: an "Illusion:" spell (they
// sort first), else the shortest naming the race ("Wolf Form" over
// "Guardian of the Forest" and "Share Wolf Form"), else the first.
func lookSpell(spells []string, race string) string {
	if strings.HasPrefix(spells[0], "Illusion: ") || race == "" {
		return spells[0]
	}
	best := ""
	for _, s := range spells {
		if strings.Contains(strings.ToLower(s), strings.ToLower(race)) && (best == "" || len(s) < len(best)) {
			best = s
		}
	}
	if best == "" {
		return spells[0]
	}
	return best
}
