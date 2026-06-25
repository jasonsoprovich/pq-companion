// Package resist ports Project Quarm's spell resist check so the app can
// estimate the odds a spell lands on a targeted NPC.
//
// The math is a faithful port of Mob::CheckResistSpell from the EQMacEmu fork
// (zone/spells.cpp), restricted to the only scenario the calculator needs: a
// player (client) casting on an NPC, on the initial cast (not a buff-tick
// recast). Branches that only apply when the target is a client, when the
// caster is an NPC, or to summoned pets are intentionally omitted — they can
// never fire for a player→NPC cast.
//
// Rather than rolling once, ComputeChances iterates every value of the
// uniform roll (0..200) and buckets the outcomes, yielding the exact outcome
// distribution (full-resist / partial / full-damage) instead of a sample.
//
// IMPORTANT: this era's resist math is community-reverse-engineered, not
// officially documented. Results are a worst-case, best-known approximation —
// surface that caveat in any UI. The formula is era-sensitive (see Era); the
// defaults track Project Quarm's current pre-Planes-of-Power state.
package resist

import "fmt"

// EQMac spell-effect (SPA) ids used to classify spells. Mirrors common/spdat.h
// in the EQMacEmu fork.
const (
	seCurrentHP              = 0   // damage/heal DoT (repeats per tick in a buff)
	seMovementSpeed          = 3   // SE_MovementSpeed (snare when negative)
	seCHA                    = 10  // used as a slot spacer
	seAttackSpeed            = 11  // SE_AttackSpeed (slow when base < 100)
	seStun                   = 21  // SE_Stun
	seCharm                  = 22  // SE_Charm
	seFear                   = 23  // SE_Fear
	seChangeFrenzyRad        = 30  // SE_ChangeFrenzyRad (Pacify)
	seMez                    = 31  // SE_Mez
	seCurrentHPOnce          = 79  // instant damage/heal (no DoT tail)
	seHarmony                = 86  // SE_Harmony
	seRoot                   = 99  // SE_Root
	seStackingCommandBlock   = 148 // SE_StackingCommand_Block
	seStackingCommandOverwrt = 149 // SE_StackingCommand_Overwrite
	seBlank                  = 254 // SE_Blank
)

// fearLevelCap: EQMacEmu hardcaps fear on NPCs above level 52 regardless of
// the spell's own level limit (Mob::IsImmuneToSpell).
const fearLevelCap = 52

// EQMac resist-type ids (spells_new.resisttype).
const (
	resistNone    = 0
	resistMagic   = 1
	resistFire    = 2
	resistCold    = 3
	resistPoison  = 4
	resistDisease = 5
)

// EQMac target-type id we care about for rain-spell classification.
const stAETarget = 8 // ST_AETarget

// classEnchanter is the 0-based class index (matches eqstat.Enchanter and the
// spells_new.classesN ordering). The Charisma resist modifier for charm/mez
// only applies to Enchanters.
const classEnchanter = 13

// resistFalloff is RuleI(Spells, ResistFalloff) — the level at and above which
// the level-difference adjustment stops scaling. Default 67 on Quarm.
const resistFalloff = 67

// effectCount is the number of spell effect slots.
const effectCount = 12

// Era captures the expansion-era flags the resist formula branches on. On
// Project Quarm today the server is pre-Planes-of-Power and Luclin is live, so
// the zero value of PoPEnabled (false) with LuclinEnabled=true is the current
// state. These derive from internal/era at the call site.
type Era struct {
	// PoPEnabled is Preferences.PoPEnabled. When false the classic resist
	// system is active (EnableClassicResistSystem defaults on); PoP also
	// flips lull spells to a fixed target resist of 15.
	PoPEnabled bool
	// LuclinEnabled gates the harsh "six-level rule" that pins level_mod to
	// 1000 (effectively unresistable-to-land) for NPCs far above the caster.
	// That rule only applies in the Classic→Velious window, i.e. before
	// Luclin. Quarm is currently in the Luclin era, so this is true and the
	// rule is OFF.
	LuclinEnabled bool
}

// Spell holds the spells_new fields the resist check reads. The api layer maps
// db.Spell into this so the resist package stays free of a db dependency
// (mirrors how eqstat stays pure).
type Spell struct {
	ResistType      int
	ResistDiff      int
	NoPartialResist bool
	TargetType      int
	BuffDuration    int
	AEDuration      int
	// GoodEffect is spells_new.goodEffect (1 = beneficial). Detrimental
	// spells (0) are the only ones that can be rain spells.
	GoodEffect    int
	EffectIDs     [effectCount]int
	EffectBase    [effectCount]int
	EffectFormula [effectCount]int
	// EffectMax is the per-slot `max` value. For charm/mez/fear effects this
	// holds the maximum NPC level the spell can affect (0 = no limit).
	EffectMax [effectCount]int
}

// Immunities flags the NPC special abilities that make a whole spell category
// fail outright, independent of the resist roll (Mob::IsImmuneToSpell). Parsed
// from npc_types.special_abilities at the call site.
type Immunities struct {
	Magic bool // SpecialAbility::MagicImmunity (20) — fully resists everything
	Mez   bool // MesmerizeImmunity (13)
	Charm bool // CharmImmunity (14)
	Fear  bool // FearImmunity (17)
	Snare bool // SnareImmunity (16) — root/movement
	Slow  bool // SlowImmunity (12)
	Stun  bool // StunImmunity (15)
}

// Input describes one calculator query: a spell, the caster, and the targeted
// NPC. TargetResist is the NPC's resist value for the spell's resist type
// (MR/CR/FR/DR/PR), already read off npc_types. TargetLevel should be the top
// end of the NPC's level range (worst case).
type Input struct {
	Spell            Spell
	CasterLevel      int
	CasterClass      int // 0-based class index (eqstat ordering)
	CasterCHA        int
	TargetLevel      int
	TargetResist     int
	TargetImmunities Immunities
	Era              Era
}

// Result is the outcome distribution for an Input. All probabilities are in
// [0,1]. For binary spells Partial is always 0 and FullDamage == LandChance.
type Result struct {
	// Unresistable is true for resist-type-none spells (always land).
	Unresistable bool
	// Binary is true when the spell cannot partial (mez/root/charm/snare):
	// every cast is either a full land or a full resist.
	Binary bool
	// CannotAffect is true when the target is immune to this spell category or
	// is above the spell's level cap — it can never land, regardless of the
	// resist roll. Reason explains why (shown in the UI).
	CannotAffect bool
	Reason       string

	// LandChance is the probability the spell has any effect at all
	// (1 - FullResist). For binary spells this equals FullDamage.
	LandChance float64
	// AvgCastsToLand is 1/LandChance (0 when LandChance is 0). Most useful
	// for binary spells, as the design notes call out.
	AvgCastsToLand float64

	// FullResist / Partial / FullDamage partition every cast (sum ~= 1).
	FullResist float64
	Partial    float64
	FullDamage float64

	// ExpectedEffectiveness is the mean damage/effect multiplier across all
	// casts, in [0,1] (1 = full damage). For partial-capable damage spells
	// this is the headline "average fraction of damage that lands".
	ExpectedEffectiveness float64
	// PartialMin/PartialMax bound the effectiveness of the partial outcomes
	// (in [0,1]); both 0 when there are no partials.
	PartialMin float64
	PartialMax float64

	// ResistChance is the computed pre-roll resist_chance, surfaced for
	// transparency/debugging (the roll is uniform on 0..200).
	ResistChance int
}

func isEffectInSpell(s Spell, spa int) bool {
	for i := 0; i < effectCount; i++ {
		if s.EffectIDs[i] == spa {
			return true
		}
	}
	return false
}

func isBlankSlot(s Spell, i int) bool {
	e := s.EffectIDs[i]
	if e == seBlank ||
		(e == seCHA && s.EffectBase[i] == 0 && s.EffectFormula[i] == 100) ||
		e == seStackingCommandBlock || e == seStackingCommandOverwrt {
		return true
	}
	return false
}

func isFearSpell(s Spell) bool  { return isEffectInSpell(s, seFear) }
func isCharmSpell(s Spell) bool { return isEffectInSpell(s, seCharm) }
func isMezSpell(s Spell) bool   { return isEffectInSpell(s, seMez) }
func isHarmonySpell(s Spell) bool {
	return isEffectInSpell(s, seHarmony) || isEffectInSpell(s, seChangeFrenzyRad)
}

// isDirectDamageSpell mirrors IsDirectDamageSpell: an instant (no buff
// duration) spell with a negative HP effect.
func isDirectDamageSpell(s Spell) bool {
	if s.BuffDuration > 0 {
		return false
	}
	for i := 0; i < effectCount; i++ {
		e := s.EffectIDs[i]
		if (e == seCurrentHPOnce || e == seCurrentHP) && s.EffectBase[i] < 0 {
			return true
		}
	}
	return false
}

// isRainSpell mirrors IsRainSpell: a detrimental AE-target spell with an AE
// duration in the rain window.
func isRainSpell(s Spell) bool {
	return s.GoodEffect == 0 && s.TargetType == stAETarget &&
		s.AEDuration > 2000 && s.AEDuration < 360000
}

// isPartialCapableSpell mirrors IsPartialCapableSpell: a spell partials only
// when no_partial_resist is unset AND its first non-blank effect is a negative
// HP effect (a damage spell or damage DoT).
func isPartialCapableSpell(s Spell) bool {
	if s.NoPartialResist {
		return false
	}
	for i := 0; i < effectCount; i++ {
		if isBlankSlot(s, i) {
			continue
		}
		e := s.EffectIDs[i]
		if (e == seCurrentHPOnce || e == seCurrentHP) && s.EffectBase[i] < 0 {
			return true
		}
		return false
	}
	return false
}

// effectMax returns the `max` value of the first slot holding the given SPA
// (and whether such a slot exists). For charm/mez/fear this is the spell's
// maximum affectable NPC level.
func effectMax(s Spell, spa int) (int, bool) {
	for i := 0; i < effectCount; i++ {
		if s.EffectIDs[i] == spa {
			return s.EffectMax[i], true
		}
	}
	return 0, false
}

func isStunSpell(s Spell) bool { return isEffectInSpell(s, seStun) }

// isSlowSpell mirrors IsSlowSpell: an attack-speed effect with base < 100
// (100 = no change, <100 = slow).
func isSlowSpell(s Spell) bool {
	for i := 0; i < effectCount; i++ {
		if s.EffectIDs[i] == seAttackSpeed && s.EffectBase[i] < 100 {
			return true
		}
	}
	return false
}

// isRootOrSnareSpell: snare immunity covers both root and movement-speed
// (snare) effects.
func isRootOrSnareSpell(s Spell) bool {
	return isEffectInSpell(s, seRoot) || isEffectInSpell(s, seMovementSpeed)
}

// immunityCheck mirrors Mob::IsImmuneToSpell for a client→NPC cast: it returns
// a reason string when the spell can never land on this target (special-ability
// immunity or over the spell's level cap), or "" when the spell is allowed to
// proceed to the resist roll.
func immunityCheck(in Input) string {
	s := in.Spell
	im := in.TargetImmunities
	lvl := in.TargetLevel

	// Magic immunity fully resists every spell.
	if im.Magic {
		return "Target is immune to magic — no spell can land."
	}

	if isMezSpell(s) {
		if im.Mez {
			return "Target is immune to mesmerize."
		}
		// NPCs above the spell's max level can't be mezzed.
		if max, ok := effectMax(s, seMez); ok && lvl > max {
			return fmt.Sprintf("Target level %d is above this mez spell's cap of %d.", lvl, max)
		}
	}

	if isCharmSpell(s) {
		if im.Charm {
			return "Target is immune to charm."
		}
		if max, ok := effectMax(s, seCharm); ok && max != 0 && lvl > max {
			return fmt.Sprintf("Target level %d is above this charm spell's cap of %d.", lvl, max)
		}
	}

	if isFearSpell(s) {
		if im.Fear {
			return "Target is immune to fear."
		}
		if lvl > fearLevelCap {
			return fmt.Sprintf("NPCs above level %d cannot be feared.", fearLevelCap)
		}
		if max, ok := effectMax(s, seFear); ok && max != 0 && lvl > max {
			return fmt.Sprintf("Target level %d is above this fear spell's cap of %d.", lvl, max)
		}
	}

	if isRootOrSnareSpell(s) && im.Snare {
		return "Target is immune to snare/root."
	}
	if isSlowSpell(s) && im.Slow {
		return "Target is immune to slow."
	}
	if isStunSpell(s) && im.Stun {
		return "Target is immune to stun."
	}

	return ""
}

// resistChanceFor computes the pre-roll resist_chance for a player→NPC initial
// cast. This is the roll-independent part of CheckResistSpell.
func resistChanceFor(in Input) int {
	s := in.Spell
	resistModifier := s.ResistDiff

	target := in.TargetResist
	casterLevel := in.CasterLevel
	targetLevel := in.TargetLevel

	// Level-difference adjustment (target is an NPC throughout).
	levelDiff := targetLevel - casterLevel
	tempLevelDiff := levelDiff
	if targetLevel >= resistFalloff {
		a := (resistFalloff - 1) - casterLevel
		if a > 0 {
			tempLevelDiff = a
		} else {
			tempLevelDiff = 0
		}
	}
	if tempLevelDiff < -9 {
		tempLevelDiff = -9
	}

	levelMod := tempLevelDiff * tempLevelDiff / 2
	if tempLevelDiff < 0 {
		levelMod = -levelMod
	}

	// Crude approximation of Sony's resist bonus for NPCs above the caster.
	if casterLevel < 50 {
		bumpLevel := casterLevel + 4 + casterLevel/6
		if targetLevel >= bumpLevel {
			levelMod += 70 + casterLevel*6
		}
	} else if casterLevel < 64 {
		if levelDiff >= 13 {
			levelMod = casterLevel * 5
		}
	} else {
		if levelDiff >= 16 {
			levelMod = casterLevel * 5
		}
	}

	// Extra level penalty for direct-damage spells.
	if isDirectDamageSpell(s) {
		var t int
		if targetLevel >= resistFalloff {
			t = (resistFalloff - 1) - casterLevel
			if t < 0 {
				t = 0
			}
		} else {
			t = targetLevel - casterLevel
		}
		if t > 0 && targetLevel >= 17 {
			levelMod += 2 * t
		}
	}

	// Enchanter charisma reduces charm/mez resist on the initial cast.
	if in.CasterClass == classEnchanter && (isCharmSpell(s) || isMezSpell(s)) {
		if in.CasterCHA > 75 {
			resistModifier -= (in.CasterCHA - 75) / 8
		}
	}

	// PoP-era lull/harmony spells ignore real resists and use a flat 15.
	if in.Era.PoPEnabled && isHarmonySpell(s) {
		target = 15
	}

	// The "six-level rule" only applies before Luclin.
	sixLevelRule := !in.Era.LuclinEnabled
	if sixLevelRule {
		bound := casterLevel + 7
		if alt := int(float64(casterLevel) * 1.25); alt > bound {
			bound = alt
		}
		if targetLevel >= bound {
			levelMod = 1000
		}
	}

	resistChance := target + levelMod
	resistChance += resistModifier
	return resistChance
}

// effectivenessForRoll returns the spell effectiveness (0..100) for a given
// uniform roll, mirroring the tail of CheckResistSpell for a player→NPC cast.
func effectivenessForRoll(in Input, resistChance, roll int) int {
	if roll > resistChance {
		return 100
	}
	if !isPartialCapableSpell(in.Spell) || resistChance == 0 {
		return 0
	}

	partialModifier := (150 * (resistChance - roll)) / resistChance

	casterLevel := in.CasterLevel
	targetLevel := in.TargetLevel
	useClassicResists := !in.Era.PoPEnabled
	if targetLevel > casterLevel && targetLevel >= 17 && (casterLevel <= 50 || useClassicResists) {
		partialModifier += 5
	}
	if targetLevel >= 30 && (casterLevel <= 50 || useClassicResists) {
		partialModifier += casterLevel - 25
	}
	if targetLevel < 15 {
		partialModifier -= 5
	}

	if partialModifier <= 0 {
		return 100
	}
	if partialModifier >= 100 {
		return 0
	}
	return 100 - partialModifier
}

// ComputeChances returns the exact outcome distribution for an Input by
// enumerating every uniform roll (0..200), the same range zone uses.
func ComputeChances(in Input) Result {
	// Immunity / level-cap gate runs before the resist roll (and before the
	// unresistable shortcut — magic immunity stops even unresistable spells).
	if reason := immunityCheck(in); reason != "" {
		return Result{
			Binary:       !isPartialCapableSpell(in.Spell),
			CannotAffect: true,
			Reason:       reason,
		}
	}

	// Unresistable spells always land fully.
	if in.Spell.ResistType == resistNone {
		return Result{
			Unresistable:          true,
			LandChance:            1,
			AvgCastsToLand:        1,
			FullDamage:            1,
			ExpectedEffectiveness: 1,
		}
	}

	binary := !isPartialCapableSpell(in.Spell)
	resistChance := resistChanceFor(in)

	const rolls = 201 // 0..200 inclusive
	var full, partial, fullDmg int
	var effSum int
	partialMin, partialMax := 101, -1
	for roll := 0; roll < rolls; roll++ {
		eff := effectivenessForRoll(in, resistChance, roll)
		effSum += eff
		switch {
		case eff == 0:
			full++
		case eff == 100:
			fullDmg++
		default:
			partial++
			if eff < partialMin {
				partialMin = eff
			}
			if eff > partialMax {
				partialMax = eff
			}
		}
	}

	res := Result{
		Binary:                binary,
		ResistChance:          resistChance,
		FullResist:            float64(full) / rolls,
		Partial:               float64(partial) / rolls,
		FullDamage:            float64(fullDmg) / rolls,
		ExpectedEffectiveness: float64(effSum) / (rolls * 100),
	}
	res.LandChance = 1 - res.FullResist

	// Rain spells carry a flat 20% innate full-resist on NPCs (the low-HP
	// execute branch needs live HP, which a planner can't know — assume full
	// HP, so only the 20% applies).
	if isRainSpell(in.Spell) {
		const innate = 0.20
		res.FullResist = res.FullResist*(1-innate) + innate
		res.Partial *= 1 - innate
		res.FullDamage *= 1 - innate
		res.ExpectedEffectiveness *= 1 - innate
		res.LandChance = 1 - res.FullResist
	}

	if res.LandChance > 0 {
		res.AvgCastsToLand = 1 / res.LandChance
	}
	if partial > 0 {
		res.PartialMin = float64(partialMin) / 100
		res.PartialMax = float64(partialMax) / 100
	}
	return res
}
