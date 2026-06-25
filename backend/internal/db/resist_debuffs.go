package db

import (
	"fmt"
	"sort"
	"strings"
)

// ResistDebuff is a spell that lowers a target's resists, with its per-resist
// reduction (negative = lowers that resist). Powers the resist calculator's
// debuff picker — these come from any class since group debuffs (Tashanian,
// Malosini, …) are often cast by other players.
type ResistDebuff struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	MR   int    `json:"mr"`
	CR   int    `json:"cr"`
	FR   int    `json:"fr"`
	DR   int    `json:"dr"`
	PR   int    `json:"pr"`
}

// spaResistAll is SPA 111 (Resist All) — lowers every resist by its base.
const spaResistAll = 111

// ResistDebuffSpells returns every detrimental spell that lowers at least one
// resist, with its per-resist deltas. Deduplicated by name (the dump ships
// several rows per spell), keeping the strongest variant, and sorted by name.
func (db *DB) ResistDebuffSpells() ([]ResistDebuff, error) {
	// Candidate filter: detrimental, non-discipline, with any resist SPA in a
	// slot. We compute exact deltas in Go (covers SPA 111 / Resist All, which
	// ComputeBuffStatDelta doesn't).
	var slotConds []string
	for i := 1; i <= 12; i++ {
		slotConds = append(slotConds, fmt.Sprintf("s.effectid%d IN (46,47,48,49,50,%d)", i, spaResistAll))
	}
	q := fmt.Sprintf(
		"SELECT %s FROM spells_new s WHERE COALESCE(s.goodEffect,0) = 0 AND s.IsDiscipline = 0 AND (%s)",
		spellColumns, strings.Join(slotConds, " OR "),
	)
	rows, err := db.Query(q)
	if err != nil {
		return nil, fmt.Errorf("query resist debuffs: %w", err)
	}
	defer rows.Close()

	// Keep the strongest variant per name (largest total reduction).
	best := map[string]ResistDebuff{}
	for rows.Next() {
		sp, err := scanSpell(rows)
		if err != nil {
			return nil, fmt.Errorf("scan resist debuff: %w", err)
		}
		d := resistDeltaFor(sp)
		if d.MR >= 0 && d.CR >= 0 && d.FR >= 0 && d.DR >= 0 && d.PR >= 0 {
			continue // not actually a resist debuff (no negative delta)
		}
		d.ID, d.Name = sp.ID, sp.Name
		if cur, ok := best[sp.Name]; !ok || debuffMagnitude(d) > debuffMagnitude(cur) {
			best[sp.Name] = d
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]ResistDebuff, 0, len(best))
	for _, d := range best {
		out = append(out, d)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// resistDeltaFor sums a spell's resist-affecting effects into per-resist
// deltas. SPA 46–50 map to one resist each; SPA 111 lowers all five.
func resistDeltaFor(sp *Spell) ResistDebuff {
	var d ResistDebuff
	for i := 0; i < 12; i++ {
		val := sp.EffectBaseValues[i]
		switch sp.EffectIDs[i] {
		case spaBuffFireRes:
			d.FR += val
		case spaBuffColdRes:
			d.CR += val
		case spaBuffPoisonRes:
			d.PR += val
		case spaBuffDiseRes:
			d.DR += val
		case spaBuffMagicRes:
			d.MR += val
		case spaResistAll:
			d.MR += val
			d.CR += val
			d.FR += val
			d.DR += val
			d.PR += val
		}
	}
	return d
}

// debuffMagnitude is the total resist reduction (positive number), used to
// pick the strongest variant of a same-named spell.
func debuffMagnitude(d ResistDebuff) int {
	sum := 0
	for _, v := range []int{d.MR, d.CR, d.FR, d.DR, d.PR} {
		if v < 0 {
			sum -= v
		}
	}
	return sum
}
