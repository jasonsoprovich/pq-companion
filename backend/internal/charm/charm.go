// Package charm models Project Quarm's four charm-capable classes and their
// charm spell lines, plus the small amount of level-scaling math the charm pet
// finder needs to present a zone's charmable NPCs.
//
// Why a curated catalog: the spells_new.classesN columns are unreliable for the
// charm line on Quarm (several high-end charms read 255 or out-of-era levels),
// so class membership and the level at which each charm is learned are pinned
// here. Everything else the UI shows — the maximum charmable NPC level (the
// SPA-22 effect `max`), the resist type, and the animal/undead body restriction
// (the spell's targettype) — is read live from quarm.db, which is authoritative
// for those fields.
//
// The package is intentionally pure: no DB access, no I/O (mirrors how
// internal/resist and internal/eqstat stay free of a db dependency). The api
// layer resolves catalog entries against quarm.db and feeds the scaling helpers.
package charm

import "math"

// Class indices match the spells_new.classesN ordering and internal/eqstat.
const (
	classDruid       = 5
	classBard        = 7
	classNecromancer = 10
	classEnchanter   = 13
)

// seCharm is the SPA (spell effect) id for the Charm effect. Its per-slot `max`
// value is the maximum NPC level the charm can affect.
const seCharm = 22

// BodyRestriction is the body-type gate a charm line imposes, derived from the
// charm spell's targettype.
type BodyRestriction int

const (
	// RestrictNone is the enchanter/bard charm line — any body type is fair game.
	RestrictNone BodyRestriction = iota
	// RestrictAnimal is the druid charm line — animals only.
	RestrictAnimal
	// RestrictUndead is the necromancer charm line — undead only.
	RestrictUndead
)

// EQMacEmu (spdat.h) target-type ids seen on the charm line. Single-target
// charm (5) carries no body gate; the animal/undead target types do.
const (
	ttAnimal = 9
	ttUndead = 10
)

// RestrictionForTargetType maps a charm spell's targettype to its body gate.
func RestrictionForTargetType(tt int) BodyRestriction {
	switch tt {
	case ttAnimal:
		return RestrictAnimal
	case ttUndead:
		return RestrictUndead
	default:
		return RestrictNone
	}
}

// String renders a restriction as the lowercase token the API/UI use ("",
// "animal", "undead").
func (r BodyRestriction) String() string {
	switch r {
	case RestrictAnimal:
		return "animal"
	case RestrictUndead:
		return "undead"
	default:
		return ""
	}
}

// npc_types.bodytype ids (EQMacEmu bodytypes.h) a charm line can target.
const (
	bodyUndead         = 3
	bodySummonedUndead = 8
	bodyAnimal         = 21
)

// BodyTypeAllowed reports whether an NPC body type can be charmed under a
// restriction. Animal charms hit Animal; undead charms hit Undead (including the
// Summoned Undead variant); unrestricted charms hit anything.
func BodyTypeAllowed(r BodyRestriction, bodytype int) bool {
	switch r {
	case RestrictAnimal:
		return bodytype == bodyAnimal
	case RestrictUndead:
		return bodytype == bodyUndead || bodytype == bodySummonedUndead
	default:
		return true
	}
}

// dmgPerLevel is the per-level growth of an NPC's top-end melee hit across its
// spawn level range (minimum damage is level-independent). Empirically 2/level
// on Quarm — verified against live spawns whose Max Hit climbs by 2 per level.
const dmgPerLevel = 2

// ScaledHP returns an NPC's hitpoints at a given spawn level. Quarm scales an
// NPC's stored HP linearly from its base level, so a level-L spawn of an NPC
// whose base row is (baseHP at baseLevel) has round(baseHP*L/baseLevel) HP.
// Verified against live range spawns (e.g. base 13200@52 → 14215@56,
// 10400@49 → 11249@53).
func ScaledHP(baseHP, baseLevel, level int) int {
	if baseLevel <= 0 {
		return baseHP
	}
	return int(math.Round(float64(baseHP) * float64(level) / float64(baseLevel)))
}

// ScaledMaxHit returns an NPC's maximum melee hit at a given spawn level. The
// top end climbs dmgPerLevel per level above the base; the minimum is constant.
func ScaledMaxHit(baseMaxDmg, baseLevel, level int) int {
	return baseMaxDmg + dmgPerLevel*(level-baseLevel)
}

// DPS returns sustained single-target melee damage per second from an NPC's
// damage spread and attack delay. attackDelay is in tenths of a second (a delay
// of 19 is 1.9s). Double attack, dual wield and haste are intentionally not
// modelled — most charm targets are single-hit warriors and this keeps the
// figure a stable, comparable baseline.
func DPS(minDmg, maxDmg, attackDelay int) float64 {
	if attackDelay <= 0 {
		return 0
	}
	avg := float64(minDmg+maxDmg) / 2
	return avg / (float64(attackDelay) / 10)
}

// CatalogEntry pins a charm spell to a class and the level at which that class
// learns it. The spell's id, maximum charmable NPC level, resist data and body
// restriction are resolved from quarm.db at request time by name.
type CatalogEntry struct {
	Name     string
	ReqLevel int
}

// catalog is the per-class charm spell line. Req levels are the Quarm-era values
// for each class. Dire Charm is intentionally absent: it has no row in quarm.db
// (the spell isn't in this era's spell table), so it can't be resolved, scored,
// or land-checked.
var catalog = map[int][]CatalogEntry{
	classEnchanter: {
		{"Charm", 12},
		{"Beguile", 29},
		{"Cajoling Whispers", 39},
		{"Allure", 49},
		{"Boltran's Agacerie", 53},
		{"Beckon", 57},
		{"Dictate", 58},
		{"Command of Druzzil", 64},
	},
	classNecromancer: {
		{"Dominate Undead", 18},
		{"Beguile Undead", 39},
		{"Cajole Undead", 49},
		{"Thrall of Bones", 53},
		{"Enslave Death", 55},
		{"Word of Terris", 60},
	},
	classDruid: {
		{"Befriend Animal", 14},
		{"Charm Animals", 29},
		{"Tunare's Request", 34},
		{"Beguile Animals", 39},
		{"Allure of the Wild", 49},
		{"Call of Karana", 53},
		{"Command of Tunare", 60},
	},
	classBard: {
		{"Solon's Song of the Sirens", 27},
		{"Solon's Bewitching Bravura", 51},
		{"Call of the Banshee", 57},
	},
}

// Classes returns the charm-capable class indices, highest-tier line last.
func Classes() []int {
	return []int{classEnchanter, classNecromancer, classDruid, classBard}
}

// IsCharmClass reports whether a class index has a charm line.
func IsCharmClass(classIdx int) bool {
	_, ok := catalog[classIdx]
	return ok
}

// SpellsForClass returns the curated charm catalog for a class, or nil when the
// class can't charm.
func SpellsForClass(classIdx int) []CatalogEntry {
	return catalog[classIdx]
}

// CharmEffectSPA is exported so the api layer reads the same SPA id when pulling
// the charm cap off a resolved spell row.
const CharmEffectSPA = seCharm
