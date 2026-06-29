package db

import (
	"fmt"
	"sort"
	"strings"
)

// ResistMod is one resist-lowering effect slot. The actual magnitude is
// level-scaled (Base/Max/Formula via CalcSpellEffectValue_formula), so the
// caller computes the value at the debuffer's level rather than trusting Base.
type ResistMod struct {
	Resist  string `json:"resist"` // "mr" | "cr" | "fr" | "dr" | "pr"
	Base    int    `json:"base"`
	Max     int    `json:"max"`
	Formula int    `json:"formula"`
}

// ResistDebuff is a spell that lowers a target's resists. Powers the resist
// calculator's debuff picker — these come from any class since group debuffs
// (Tashanian, Malosini, …) are often cast by other players. The per-resist
// magnitude is level-scaled, so Mods carries the scaling params, not a final
// number.
type ResistDebuff struct {
	ID   int         `json:"id"`
	Name string      `json:"name"`
	Mods []ResistMod `json:"mods"`
	// BardSkill is the song's instrument skill (spells_new.skill: wind/stringed/
	// brass/percussion/singing) when this debuff is a bard song, else 0. The
	// resist calculator scales a bard-song debuff by that instrument's modifier.
	BardSkill int `json:"bard_skill"`
}

// spaResistAll is SPA 111 (Resist All) — lowers every resist by its base.
const spaResistAll = 111

// ResistDebuffSpells returns every detrimental spell that lowers at least one
// resist, with per-resist scaling params. Deduplicated by name (the dump ships
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
		mods := resistModsFor(sp)
		if len(mods) == 0 {
			continue // not actually a resist debuff (no negative slot)
		}
		d := ResistDebuff{ID: sp.ID, Name: sp.Name, Mods: mods, BardSkill: bardSongSkill(sp)}
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

// resistModsFor collects a spell's resist-lowering effect slots as scaling
// params. SPA 46–50 map to one resist each; SPA 111 lowers all five. Only
// slots with a negative base (an actual reduction) are kept.
func resistModsFor(sp *Spell) []ResistMod {
	var mods []ResistMod
	add := func(resist string, i int) {
		if sp.EffectBaseValues[i] >= 0 {
			return
		}
		mods = append(mods, ResistMod{
			Resist:  resist,
			Base:    sp.EffectBaseValues[i],
			Max:     sp.EffectMaxValues[i],
			Formula: sp.EffectFormulas[i],
		})
	}
	for i := 0; i < 12; i++ {
		switch sp.EffectIDs[i] {
		case spaBuffFireRes:
			add("fr", i)
		case spaBuffColdRes:
			add("cr", i)
		case spaBuffPoisonRes:
			add("pr", i)
		case spaBuffDiseRes:
			add("dr", i)
		case spaBuffMagicRes:
			add("mr", i)
		case spaResistAll:
			for _, r := range []string{"mr", "cr", "fr", "dr", "pr"} {
				add(r, i)
			}
		}
	}
	return mods
}

// bardSongSkill returns the song's instrument skill if this spell is a bard
// song that scales off an instrument (or singing), else 0. We require the bard
// class to actually cast it (a handful of non-song spells carry a bard casting
// skill) — ClassLevels is 0-based with 255 meaning "cannot cast".
func bardSongSkill(sp *Spell) int {
	switch sp.Skill {
	case skillPercussionInst, skillBrassInst, skillSinging, skillStringedInst, skillWindInst:
	default:
		return 0
	}
	const bardIdx = 7
	if lvl := sp.ClassLevels[bardIdx]; lvl <= 0 || lvl >= 255 {
		return 0
	}
	return sp.Skill
}

// debuffMagnitude is the total resist reduction by base magnitude, used to
// pick the strongest variant of a same-named spell.
func debuffMagnitude(d ResistDebuff) int {
	sum := 0
	for _, m := range d.Mods {
		if m.Base < 0 {
			sum -= m.Base
		}
	}
	return sum
}
