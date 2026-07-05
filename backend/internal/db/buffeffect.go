package db

// BuffStatDelta is the structured stat contribution from a single buff spell's
// 12 effect slots. Field names align with the frontend StatDelta and the
// character stats statBlock for easy summing.
//
// Haste is the raw melee-haste percent (SPA 11 / 119, max within this spell).
// The caller decides which stacking tier the contribution belongs to (v1 for
// worn items, v2 for cast buffs, v3 for overhaste like Warsong of the Vah
// Shir) based on how the spell is being applied — that distinction can't be
// inferred from the spell row alone.
//
// SpellHaste is SPA 127 raw percent (cast-time reduction). Per Project Quarm
// rules the total spell-haste sum is hard-capped at 50%, but capping happens
// at the caller, not here.
type BuffStatDelta struct {
	HP   int `json:"hp"`
	Mana int `json:"mana"`
	AC   int `json:"ac"`
	STR  int `json:"str"`
	STA  int `json:"sta"`
	AGI  int `json:"agi"`
	DEX  int `json:"dex"`
	WIS  int `json:"wis"`
	INT  int `json:"int"`
	CHA  int `json:"cha"`
	PR   int `json:"pr"`
	MR   int `json:"mr"`
	DR   int `json:"dr"`
	FR   int `json:"fr"`
	CR   int `json:"cr"`

	Attack     int `json:"attack"`
	Haste      int `json:"haste"`       // SPA 11/119, raw % (max within spell)
	SpellHaste int `json:"spell_haste"` // SPA 127, raw %
	Regen      int `json:"regen"`       // SPA 0 on a buff (HP/tick)
	ManaRegen  int `json:"mana_regen"`  // SPA 15 on a buff (mana/tick)
	DmgShield  int `json:"dmg_shield"`  // SPA 59 magnitude
}

// EQEmu SPA codes used here. Mirrors the constants in api/characters.go
// (parseWornEffect) but kept separate so the helper can live in the db
// package without an import cycle.
const (
	spaBuffHitpoints = 0 // base/tick = HP regen when buffduration>0
	spaBuffAC        = 1
	spaBuffATK       = 2
	spaBuffSTR       = 4
	spaBuffDEX       = 5
	spaBuffAGI       = 6
	spaBuffSTA       = 7
	spaBuffINT       = 8
	spaBuffWIS       = 9
	spaBuffCHA       = 10
	spaBuffHasteV1   = 11 // melee haste v1 (worn-context primarily)
	spaBuffMana      = 15 // base/tick = mana regen when buffduration>0
	spaBuffFireRes   = 46
	spaBuffColdRes   = 47
	spaBuffPoisonRes = 48
	spaBuffDiseRes   = 49
	spaBuffMagicRes  = 50
	spaBuffDmgShield = 59
	spaBuffMaxHP     = 69
	spaBuffManaPool  = 97
	spaBuffHasteV2   = 119 // melee haste v2 (spell/song/proc primarily)
	spaBuffSpellHst  = 127 // spell-cast haste
)

// ComputeBuffStatDelta walks the 12 effect slots of a buff spell and returns
// the aggregated stat contributions in BuffStatDelta form. Level-scaling
// effects are evaluated at the server level cap via ApplyLevelFormula — i.e.
// the best-case "at cap" value, matching how pqdi.cc, quarmy, and this app's
// own spell database page render scaled buffs.
//
// Haste is reported as the largest single-slot SPA 11/119 contribution within
// this spell. The caller is responsible for stacking rules across spells
// (max within a tier, sum across tiers, level-based caps).
func ComputeBuffStatDelta(spell *Spell) BuffStatDelta {
	var d BuffStatDelta
	if spell == nil {
		return d
	}
	hasBuffDuration := spell.BuffDuration > 0

	for i := 0; i < 12; i++ {
		spa := spell.EffectIDs[i]
		base := spell.EffectBaseValues[i]
		if spa == 254 || spa == 255 {
			continue
		}
		if spa == 0 && base == 0 {
			continue
		}

		// Level-scaling formulas (101–105, 109, 110, 119, 121) grow the effect
		// with caster level; we evaluate at the server cap so the value matches
		// what a level-60 main receives — the same best-case the spell database
		// page and quarmy show. Static formulas (0/100) return base unchanged.
		// e.g. Khura's Focusing HP is SPA 69 base 250 formula 104 → 250+60*3=430,
		// not the raw 250. Formula 100 buffs (Aego AC/HP) are untouched.
		val := ApplyLevelFormula(spell.EffectFormulas[i], base, spell.EffectMaxValues[i], ServerLevelCap)

		switch spa {
		case spaBuffHitpoints:
			// Buff slot 0 with positive base + buff duration = HP regen/tick.
			// Negative or instant-only landings are not buff stats.
			if val > 0 && hasBuffDuration {
				d.Regen += val
			}
		case spaBuffAC:
			d.AC += val
		case spaBuffATK:
			d.Attack += val
		case spaBuffSTR:
			d.STR += val
		case spaBuffDEX:
			d.DEX += val
		case spaBuffAGI:
			d.AGI += val
		case spaBuffSTA:
			d.STA += val
		case spaBuffINT:
			d.INT += val
		case spaBuffWIS:
			d.WIS += val
		case spaBuffCHA:
			d.CHA += val
		case spaBuffHasteV1:
			// SPA 11 uses the "+100" encoding (base 122 → +22% haste).
			if val > 100 {
				h := val - 100
				if h > d.Haste {
					d.Haste = h
				}
			}
		case spaBuffHasteV2:
			// SPA 119 uses raw % (base 25 → +25% haste). No +100 shift.
			// Warsong of the Vah Shir, Battlecry, Primal Guard etc. live
			// here. Within a single spell only the largest haste slot
			// wins; the caller buckets v1/v2/v3 by source.
			if val > 0 && val > d.Haste {
				d.Haste = val
			}
		case spaBuffMana:
			if val > 0 && hasBuffDuration {
				d.ManaRegen += val
			}
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
		case spaBuffDmgShield:
			// DS base is conventionally negative on items/spells (damage
			// dealt to the attacker). Surface as positive magnitude.
			if val < 0 {
				d.DmgShield += -val
			} else {
				d.DmgShield += val
			}
		case spaBuffMaxHP:
			d.HP += val
		case spaBuffManaPool:
			d.Mana += val
		case spaBuffSpellHst:
			// SPA 127 base is the raw % (no +100 encoding).
			d.SpellHaste += val
		}
	}
	return d
}
