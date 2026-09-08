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

// catalog is the per-class charm spell line, by spell name. The catalog only
// pins class membership (the spells_new.classesN columns can't reliably tell
// enchanter from bard on shared target types, and don't name the line); the
// required level, era availability, maximum charmable NPC level, resist data
// and body restriction are all resolved live from quarm.db at request time.
//
// Era gating is data-driven: a charm's required level is read from the spell's
// classesN column, which on Quarm encodes era — Planes-of-Power charms sit above
// the level 60 cap (Beckon 62, Command of Druzzil 64, Word of Terris 65, Command
// of Tunare 63, Call of the Banshee 64), so they fall out automatically while the
// pop_enabled flag is off and reappear when it's on (cap 65).
//
// Dire Charm is included via its three per-class effect spells. It is an AA
// (altadv_vars skill_id 145, aa_expansion 3 = Luclin, level 59), not a trained
// spell — the server (EQMacEmu zone/aa.cpp) maps the AA to a class-specific
// spell id: Necromancer -> 2759 "Undead Pact", Druid -> 2760 "Servant of
// Nature", Enchanter -> 2761 "Dominating Gaze". Those quarm.db rows carry the
// real charm data (SPA 22, max level 46, magic resist) and encode the body
// gate the same way every other charm line does — targettype 10 (undead) for
// the necro spell, 9 (animal) for the druid spell, 5 (any) for the enchanter
// spell — so RestrictionForTargetType handles them with no special case. Only
// the required level needs pinning (see grantedLevels): their spells_new
// classesN columns read 254/255/bogus because they're AA-granted.
var catalog = map[int][]string{
	classEnchanter: {
		"Charm", "Beguile", "Cajoling Whispers", "Allure",
		"Boltran's Agacerie", "Beckon", "Dictate", "Command of Druzzil",
		"Dominating Gaze", // Dire Charm (Enchanter)
	},
	classNecromancer: {
		"Dominate Undead", "Beguile Undead", "Cajole Undead",
		"Thrall of Bones", "Enslave Death", "Word of Terris",
		"Undead Pact", // Dire Charm (Necromancer)
	},
	classDruid: {
		"Befriend Animal", "Charm Animals", "Tunare's Request", "Beguile Animals",
		"Allure of the Wild", "Call of Karana", "Command of Tunare",
		"Servant of Nature", // Dire Charm (Druid)
	},
	classBard: {
		"Solon's Song of the Sirens", "Solon's Bewitching Bravura", "Call of the Banshee",
	},
}

// direCharmDBNames are the three quarm.db spell names the Dire Charm AA casts,
// one per class. The UI shows them all as "Dire Charm" (see DisplayName).
var direCharmDBNames = map[string]bool{
	"Undead Pact":       true, // Necromancer
	"Servant of Nature": true, // Druid
	"Dominating Gaze":   true, // Enchanter
}

// grantedLevels overrides the required level for charm spells a class gets from
// something other than a trainer, whose spells_new classesN column therefore
// reads 254/255 (or a bogus value) and can't be trusted:
//   - Boltran's Agacerie: a Luclin spell-research enchanter charm (level 53).
//   - The three Dire Charm effect spells: AA-granted at level 59 (altadv_vars
//     skill_id 145, class_type 59, aa_expansion 3 = Luclin).
var grantedLevels = map[string]int{
	"Boltran's Agacerie": 53,
	"Undead Pact":        59,
	"Servant of Nature":  59,
	"Dominating Gaze":    59,
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

// SpellsForClass returns the charm spell names for a class, or nil when the
// class can't charm.
func SpellsForClass(classIdx int) []string {
	return catalog[classIdx]
}

// GrantedLevel returns the required level for a charm spell a class gets from
// research or an AA rather than a trainer (so its spells_new classesN column is
// unreliable), and whether such an override exists. Callers should prefer this
// over the classesN value whenever it returns ok.
func GrantedLevel(name string) (int, bool) {
	lvl, ok := grantedLevels[name]
	return lvl, ok
}

// DisplayName maps a catalog spell name to what the UI should show. The three
// per-class Dire Charm effect spells ("Undead Pact", "Servant of Nature",
// "Dominating Gaze") all display as "Dire Charm"; every other name is returned
// unchanged.
func DisplayName(name string) string {
	if direCharmDBNames[name] {
		return "Dire Charm"
	}
	return name
}

// CharmEffectSPA is exported so the api layer reads the same SPA id when pulling
// the charm cap off a resolved spell row.
const CharmEffectSPA = seCharm
