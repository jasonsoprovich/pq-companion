package db

// ── Haste effect math ────────────────────────────────────────────────────────
//
// EQEmu encodes melee/attack-speed haste in the "+100" form: spell effect
// base value 122 means +22% haste. The actual per-item haste percentage for
// a worn slot depends on the spell's effect formula:
//
//   formula 100 (static): effective_value = base
//   formula 102 (linear): effective_value = min(base + level, max), where
//                          level is the item's wornlevel column.
//
// All currently observed Project Quarm worn-haste spells use formula 100 or
// 102. Other formulas are not modelled here — they fall back to `base`.
//
// Reported haste % is effective_value - 100 (clamped to [0, max - 100]).
//
// SPA 11  = Melee Haste v1 (worn equipment haste)
// SPA 119 = Melee Haste v2 (spell/song/proc haste; can appear on items too)

const (
	spaMeleeHasteV1 = 11
	spaMeleeHasteV2 = 119
)

// ComputeEffectValue applies the spell effect formula to a level input.
// Used for worn item effects where `level` = item.WornLevel.
//
// Only formulas 100 (static) and 102 (linear) are modelled. Anything else
// returns `base` unchanged — matches how pqdi renders untyped scaling.
func ComputeEffectValue(formula, base, max, level int) int {
	switch formula {
	case 102:
		v := base + level
		if max > 0 && v > max {
			v = max
		}
		return v
	default:
		return base
	}
}

// ComputeWornHastePct returns the effective haste percentage for an item
// whose worneffect points at `spell`, given the item's wornlevel. Returns 0
// when the spell has no SPA 11/119 slot. Walks all 12 effect slots and
// returns the largest contribution found (in practice only one slot is the
// haste effect, but defensive against multi-slot templates).
func ComputeWornHastePct(spell *Spell, wornLevel int) int {
	if spell == nil {
		return 0
	}
	best := 0
	for i := 0; i < 12; i++ {
		spa := spell.EffectIDs[i]
		if spa != spaMeleeHasteV1 && spa != spaMeleeHasteV2 {
			continue
		}
		base := spell.EffectBaseValues[i]
		if base == 0 {
			continue
		}
		v := ComputeEffectValue(spell.EffectFormulas[i], base, spell.EffectMaxValues[i], wornLevel)
		pct := v - 100
		if pct > best {
			best = pct
		}
	}
	return best
}
