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
// the aggregated stat contributions in BuffStatDelta form. For formula 102
// (linear scale by level), the spell's `max` value is used — i.e. the
// best-case scaled value, matching how pqdi.cc renders scaled buffs.
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

		// In buff context, formula 102 scales by caster level — without a
		// caster-level input we use the spell's max as the best-case value.
		// All currently observed beneficial buff spells use formula 100
		// (static) so this is forward-looking; formula 100 paths use base.
		val := base
		if spell.EffectFormulas[i] == 102 && spell.EffectMaxValues[i] > base {
			val = spell.EffectMaxValues[i]
		}

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
