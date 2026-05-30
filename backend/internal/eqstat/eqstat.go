// Package eqstat reimplements Project Quarm's (EQMacEmu fork) server-side
// player stat formulas: max HP, max mana, displayed AC, resistances, and the
// attribute/resist caps.
//
// These are the classic-EverQuest-era (Al'Kabor / Mac client) formulas, which
// differ from modern EQEmu master. They are ported verbatim from
// EQMacEmu/Server (zone/client_mods.cpp, zone/attack.cpp, zone/bonuses.cpp).
// All arithmetic uses Go int division to match the C++ int32 truncation —
// using floats would round differently and drift from the client.
//
// The package is intentionally pure: no DB access, no I/O. Callers (the API
// layer) gather the inputs — base attributes from the Quarmy export, item and
// buff contributions, and AA-derived bonuses from quarm.db — and feed them in.
package eqstat

// EQ class indices, 0-indexed to match character.Character.Class (the store
// subtracts one from the 1-indexed Quarmy/EQ class id).
const (
	Warrior = iota
	Cleric
	Paladin
	Ranger
	ShadowKnight
	Druid
	Monk
	Bard
	Rogue
	Shaman
	Necromancer
	Wizard
	Magician
	Enchanter
	Beastlord
)

// EQ race ids (as stored on the character row — the raw Quarmy value, not
// 0-indexed). Iksar and Vah Shir use the high client ids.
const (
	RaceHuman     = 1
	RaceBarbarian = 2
	RaceErudite   = 3
	RaceWoodElf   = 4
	RaceHighElf   = 5
	RaceDarkElf   = 6
	RaceHalfElf   = 7
	RaceDwarf     = 8
	RaceTroll     = 9
	RaceOgre      = 10
	RaceHalfling  = 11
	RaceGnome     = 12
	RaceIksar     = 128
	RaceVahShir   = 130
)

// HP and mana both clamp to this signed-16-bit ceiling server-side.
const vitalCap = 32767

// Attributes is the seven primary stats.
type Attributes struct {
	STR, STA, AGI, DEX, WIS, INT, CHA int
}

// Resists is the five resist values, in the display order used across the app
// (magic, cold, fire, disease, poison).
type Resists struct {
	MR, CR, FR, DR, PR int
}

// ── HP ──────────────────────────────────────────────────────────────────────

// classLevelFactor is EQMacEmu's per-class HP-per-level multiplier
// (GetClassLevelFactor). CalcBaseHP divides this by 10. There is no player
// base_data row in this fork — the factor is hardcoded by class/level band.
func classLevelFactor(class, level int) int {
	switch class {
	case Warrior:
		switch {
		case level < 20:
			return 220
		case level < 30:
			return 230
		case level < 40:
			return 250
		case level < 53:
			return 270
		case level < 57:
			return 280
		case level < 60:
			return 290
		case level < 70:
			return 300
		default:
			return 311
		}
	case Cleric, Druid, Shaman:
		if level < 70 {
			return 150
		}
		return 157
	case Paladin, ShadowKnight:
		switch {
		case level < 35:
			return 210
		case level < 45:
			return 220
		case level < 51:
			return 230
		case level < 56:
			return 240
		case level < 60:
			return 250
		case level < 68:
			return 260
		default:
			return 270
		}
	case Monk, Bard, Rogue, Beastlord:
		switch {
		case level < 51:
			return 180
		case level < 58:
			return 190
		case level < 70:
			return 200
		default:
			return 210
		}
	case Ranger:
		switch {
		case level < 58:
			return 200
		case level < 70:
			return 210
		default:
			return 220
		}
	case Magician, Wizard, Necromancer, Enchanter:
		if level < 70 {
			return 120
		}
		return 127
	default:
		// NPC-style fallback (unused for real player classes).
		switch {
		case level < 35:
			return 210
		case level < 45:
			return 220
		case level < 51:
			return 230
		case level < 56:
			return 240
		case level < 60:
			return 250
		default:
			return 260
		}
	}
}

// BaseHP is CalcBaseHP: level × (factor/10), scaled by STA. STA over 255
// counts half. This is the STA-driven HP pool before item/buff/AA additions.
func BaseHP(class, level, sta int) int {
	lm := classLevelFactor(class, level) / 10
	levelHP := level * lm
	staFactor := sta
	if staFactor > 255 {
		staFactor = (staFactor-255)/2 + 255
	}
	return levelHP*staFactor/300 + levelHP
}

// MaxHP is CalcMaxHP. The AA HP percent (Natural Durability / Physical
// Enhancement / Planar Durability) applies to (baseHP + itemHP) only; flat AA
// HP, the constant +5, and buff HP are added afterward. Clamped to 32767.
func MaxHP(class, level, sta, itemHP, buffHP, aaFlatHP int, aaHPPct float64) int {
	val := BaseHP(class, level, sta) + itemHP
	if aaHPPct > 0 {
		val += int(float64(val) * aaHPPct / 100.0)
	}
	val += aaFlatHP + 5
	val += buffHP
	if val > vitalCap {
		val = vitalCap
	}
	if val < 0 {
		val = 0
	}
	return val
}

// ── Mana ────────────────────────────────────────────────────────────────────

// Caster-class letter from GetCasterClass: 'I' = INT casters, 'W' = WIS
// casters, 'N' = no mana (pure melee).
const (
	casterINT  = 'I'
	casterWIS  = 'W'
	casterNone = 'N'
)

// CasterType returns the mana-casting class letter for the class.
func CasterType(class int) byte {
	switch class {
	case Necromancer, Wizard, Magician, Enchanter, ShadowKnight:
		return casterINT
	case Cleric, Druid, Shaman, Paladin, Ranger, Beastlord:
		return casterWIS
	default: // Warrior, Monk, Rogue, Bard
		return casterNone
	}
}

// BaseMana is CalcBaseMana: the level + prime-stat driven mana pool. The prime
// stat is WIS for 'W' casters, INT for 'I' casters. Returns 0 for non-casters.
func BaseMana(class, level, prime int) int {
	ct := CasterType(class)
	if ct == casterNone {
		return 0
	}
	levelFactor := 15 * level
	statFactor := prime
	if statFactor > 200 {
		statFactor = (statFactor-200)/2 + 200
	}
	if statFactor > 100 {
		statFactor += 3 * (statFactor - 100) / 2
	}
	return statFactor*levelFactor/200 + levelFactor
}

// MaxMana is CalcMaxMana. Hybrids (Ranger/Paladin/Beastlord) have no mana
// before level 9. flatMana is the summed item + buff mana-pool contribution.
func MaxMana(class, level, wis, intel, flatMana int) int {
	ct := CasterType(class)
	if ct == casterNone {
		return 0
	}
	if (class == Ranger || class == Paladin || class == Beastlord) && level < 9 {
		return 0
	}
	prime := intel
	if ct == casterWIS {
		prime = wis
	}
	m := BaseMana(class, level, prime) + flatMana
	if m < 0 {
		m = 0
	}
	if m > vitalCap {
		m = vitalCap
	}
	return m
}

// ── Resists ──────────────────────────────────────────────────────────────────

// BaseResists returns the innate per-race resist floor (the racial base before
// any class/level, item, buff, or AA additions).
func BaseResists(race int) Resists {
	switch race {
	case RaceBarbarian:
		return Resists{MR: 25, CR: 35, FR: 25, DR: 15, PR: 15}
	case RaceErudite:
		return Resists{MR: 30, CR: 25, FR: 25, DR: 10, PR: 15}
	case RaceDwarf:
		return Resists{MR: 30, CR: 25, FR: 25, DR: 15, PR: 20}
	case RaceTroll:
		return Resists{MR: 25, CR: 25, FR: 5, DR: 15, PR: 15}
	case RaceHalfling:
		return Resists{MR: 25, CR: 25, FR: 25, DR: 20, PR: 20}
	case RaceIksar:
		return Resists{MR: 25, CR: 15, FR: 30, DR: 15, PR: 15}
	case RaceHuman, RaceWoodElf, RaceHighElf, RaceDarkElf, RaceHalfElf,
		RaceOgre, RaceGnome, RaceVahShir:
		return Resists{MR: 25, CR: 25, FR: 25, DR: 15, PR: 15}
	default:
		// NPC-style fallback.
		return Resists{MR: 20, CR: 25, FR: 20, DR: 15, PR: 15}
	}
}

// classLevelResistBonus returns the class/level resist bonuses layered on top
// of the racial base. The "+ (level-49)" tail kicks in above level 49 for the
// classes that scale their innate resist with level.
func classLevelResistBonus(class, level int) Resists {
	var r Resists
	overOld := 0 // level past 49, used by the scaling classes
	if level > 49 {
		overOld = level - 49
	}
	over50 := 0 // monk disease/poison scale from 50
	if level > 50 {
		over50 = level - 50
	}
	switch class {
	case Warrior:
		r.MR += level / 2
	case Ranger:
		r.FR += 4 + overOld
		r.CR += 4 + overOld
	case Monk:
		r.FR += 8 + overOld
		r.DR += over50
		r.PR += over50
	case Paladin:
		r.DR += 8 + overOld
	case ShadowKnight:
		r.DR += 4 + overOld
		r.PR += 4 + overOld
	case Beastlord:
		r.DR += 4 + overOld
		r.CR += 4 + overOld
	case Rogue:
		r.PR += 8 + overOld
	}
	return r
}

// ResistCap is the per-resist hard cap before AA cap modifiers.
const ResistCap = 500

// Resistance computes one layer's total for all five resists:
// racial base + class/level bonus + summed item/buff/AA contributions, floored
// at 1 and capped at ResistCap (+ any AA cap modifier the caller folds in).
func Resistance(class, level, race int, add Resists, capMod Resists) Resists {
	base := BaseResists(race)
	cl := classLevelResistBonus(class, level)
	out := Resists{
		MR: base.MR + cl.MR + add.MR,
		CR: base.CR + cl.CR + add.CR,
		FR: base.FR + cl.FR + add.FR,
		DR: base.DR + cl.DR + add.DR,
		PR: base.PR + cl.PR + add.PR,
	}
	out.MR = clampResist(out.MR, capMod.MR)
	out.CR = clampResist(out.CR, capMod.CR)
	out.FR = clampResist(out.FR, capMod.FR)
	out.DR = clampResist(out.DR, capMod.DR)
	out.PR = clampResist(out.PR, capMod.PR)
	return out
}

func clampResist(v, capMod int) int {
	if v < 1 {
		v = 1
	}
	if max := ResistCap + capMod; v > max {
		v = max
	}
	return v
}

// ── Attribute cap ────────────────────────────────────────────────────────────

// MaxStat is the attribute cap: 255 through level 60, then +5 per level above
// 60, plus any AA stat-cap modifier (SE_RaiseStatCap).
func MaxStat(level, capMod int) int {
	base := 255
	if level > 60 {
		base += (level - 60) * 5
	}
	return base + capMod
}

// CapAttribute clamps a single attribute to MaxStat.
func CapAttribute(v, level, capMod int) int {
	if m := MaxStat(level, capMod); v > m {
		return m
	}
	return v
}

// ── Haste ────────────────────────────────────────────────────────────────────

// MeleeHasteCap is the level-based ceiling on combined worn + spell melee haste
// (v1 + v2) before overhaste (v3). Overhaste stacks on top of this cap.
func MeleeHasteCap(level int) int {
	switch {
	case level <= 0:
		return 100 // unknown level — assume max
	case level <= 30:
		return 50
	case level <= 50:
		return 74
	case level <= 54:
		return 84
	case level <= 59:
		return 94
	default:
		return 100
	}
}

// ── AC ───────────────────────────────────────────────────────────────────────

// isACCaster reports whether the class uses the pure-caster AC path: raw item
// AC (no ×4/3), defense skill ÷2 (instead of ÷3), spell AC ÷3 (instead of ÷4).
// This set is the four INT casters — note ShadowKnight is NOT included even
// though it casts INT spells.
func isACCaster(class int) bool {
	switch class {
	case Wizard, Magician, Necromancer, Enchanter:
		return true
	}
	return false
}

// avoidance is the avoidance half of displayed AC (GetAvoidance with the
// Combat Agility AA ignored, as CalcAC does). defenseSkill is the character's
// Defense skill value (we assume the class/level cap), agi the total AGI.
func avoidance(defenseSkill, agi, level int) int {
	defenseAvoidance := 0
	if defenseSkill > 0 {
		defenseAvoidance = defenseSkill * 400 / 225
	}

	agiAvoidance := 0
	switch {
	case agi < 40:
		agiAvoidance = 25 * (agi - 40) / 40
	case agi >= 40 && agi < 60:
		agiAvoidance = 0
	case agi >= 60 && agi <= 74:
		agiAvoidance = 2 * (28 - (200-agi)/5) / 3
	default: // agi >= 75
		bonusAdj := 80
		switch {
		case level < 7:
			bonusAdj = 35
		case level < 20:
			bonusAdj = 55
		case level < 40:
			bonusAdj = 70
		}
		if agi < 200 {
			agiAvoidance = 2 * (bonusAdj - (200-agi)/5) / 3
		} else {
			agiAvoidance = 2 * bonusAdj / 3
		}
	}

	computed := defenseAvoidance + agiAvoidance
	if computed < 1 {
		computed = 1
	}
	return computed
}

// mitigation is the mitigation half of displayed AC (GetMitigation with
// ignoreCap = true — the value the client shows, which skips the anti-twink
// and class/level softcaps entirely). itemAC is the summed worn-item AC,
// spellAC the summed buff/song AC, defenseSkill the Defense skill value, agi
// the total AGI, weight the monk's carried weight (in stone; only consulted
// for Monk).
func mitigation(class, level, race, itemAC, spellAC, agi, defenseSkill, weight int) int {
	acSum := itemAC
	if !isACCaster(class) {
		acSum = 4 * acSum / 3
	}

	// Class- and race-specific innate AC. The non-pure-cap (displayed) path
	// keeps these; the softcap math is what ignoreCap skips.
	acSum += classRaceACBonus(class, level, race, agi, weight)

	if defenseSkill > 0 {
		if isACCaster(class) {
			acSum += defenseSkill / 2
		} else {
			acSum += defenseSkill / 3
		}
	}

	spellDivisor := 4
	if isACCaster(class) {
		spellDivisor = 3
	}
	acSum += spellAC / spellDivisor

	if agi > 70 {
		acSum += agi / 20
	}

	if acSum < 0 {
		acSum = 0
	}
	return acSum
}

// classRaceACBonus is the inlined class/race innate AC from GetMitigation:
// Iksar skin AC, plus the Monk (weight-based), Rogue, and Beastlord AGI
// bonuses. The Monk weight tiers and the Rogue/Beastlord AGI tiers are the
// less-documented corners of the formula; they are implemented best-effort
// for level ≤ 60 and may be refined. The caster/tank classes have no innate
// block here and are exact.
func classRaceACBonus(class, level, race, agi, weight int) int {
	bonus := 0

	// Iksar innate skin AC (applies regardless of class).
	if race == RaceIksar {
		switch {
		case level < 10:
			bonus += 10
		case level > 35:
			bonus += 35
		default:
			bonus += level
		}
	}

	switch class {
	case Monk:
		// Under the weight softcap a monk gets (level+5) × 4/3; the bonus
		// scales down as carried weight climbs toward the hardcap. We model
		// the common "light monk" case (under softcap). Quarm's exact per-
		// level weight tiers are not published; refine if a monk reports drift.
		acBonus := level + 5
		bonus += acBonus * 4 / 3
		_ = weight
	case Rogue:
		if level >= 30 && agi > 75 {
			b := agiTierBonus(agi)
			if b > 12 {
				b = 12
			}
			bonus += b
		}
	case Beastlord:
		if level > 10 {
			b := agiTierBonus(agi)
			if b > 16 {
				b = 16
			}
			bonus += b
		}
	}
	return bonus
}

// agiTierBonus is the small AGI-tiered AC bonus shared by the Rogue and
// Beastlord innate-AC blocks (capped by the caller). Tiers at AGI 80/85/90/100.
func agiTierBonus(agi int) int {
	switch {
	case agi >= 100:
		return 12
	case agi >= 90:
		return 9
	case agi >= 85:
		return 6
	case agi >= 80:
		return 3
	default:
		return 0
	}
}

// DisplayedAC reproduces the client inventory-window AC:
// (avoidance + mitigation) × 1000 / 847, integer-truncated. This is the number
// the EQ client shows — it ignores the in-combat AC softcap.
func DisplayedAC(class, level, race, itemAC, spellAC, agi, defenseSkill, weight int) int {
	av := avoidance(defenseSkill, agi, level)
	mit := mitigation(class, level, race, itemAC, spellAC, agi, defenseSkill, weight)
	return (av + mit) * 1000 / 847
}
