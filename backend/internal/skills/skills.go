// Package skills implements the per-character Skill Tracker: it consumes
// EventSkillUp log events, persists each character's current skill values to
// user.db, and resolves skill display names to the EQMac skill_id used by the
// quarm.db skill_caps table so the API can report value/cap.
//
// The skill_id numbering is the EQMacEmu SkillType enum (common/skills.h),
// which Project Quarm's skill_caps table follows verbatim — verified against
// the DB by class availability (e.g. Backstab=8 is Rogue-only, Hand to Hand=28
// is Monk/Beastlord, Swimming=50 is uniform across all classes). This differs
// from modern EQEmu master numbering; see the project_eqmac_skill_enum memory.
package skills

import "strings"

// Unknown is the sentinel skill_id stored for a skill whose display name we
// can't map to the EQMac enum. Such skills are still tracked (value recorded)
// but the API reports no cap for them — see the plan's skip criteria.
const Unknown = -1

// skillNameToID maps the in-game skill display name (as it appears in
// "You have become better at <Skill>! (<rank>)") to its EQMac skill_id.
// Keyed lowercase; resolve via SkillID. A few historical/alias spellings are
// included so cap resolution still works if the client wording varies.
var skillNameToID = map[string]int{
	"1h blunt":               0,
	"1h slashing":            1,
	"2h blunt":               2,
	"2h slashing":            3,
	"abjuration":             4,
	"alteration":             5,
	"apply poison":           6,
	"archery":                7,
	"backstab":               8,
	"bind wound":             9,
	"bash":                   10,
	"block":                  11,
	"brass instruments":      12,
	"channeling":             13,
	"conjuration":            14,
	"defense":                15,
	"disarm":                 16,
	"disarm traps":           17,
	"divination":             18,
	"dodge":                  19,
	"double attack":          20,
	"dragon punch":           21,
	"tail rake":              21, // Iksar monk equivalent of Dragon Punch
	"dual wield":             22,
	"eagle strike":           23,
	"evocation":              24,
	"feign death":            25,
	"flying kick":            26,
	"forage":                 27,
	"hand to hand":           28,
	"hide":                   29,
	"kick":                   30,
	"meditate":               31,
	"mend":                   32,
	"offense":                33,
	"parry":                  34,
	"pick lock":              35,
	"1h piercing":            36,
	"piercing":               36, // classic display spelling
	"riposte":                37,
	"round kick":             38,
	"safe fall":              39,
	"sense heading":          40,
	"singing":                41,
	"sneak":                  42,
	"specialize abjuration":  43,
	"specialize alteration":  44,
	"specialize conjuration": 45,
	"specialize divination":  46,
	"specialize evocation":   47,
	"pick pockets":           48,
	"stringed instruments":   49,
	"swimming":               50,
	"throwing":               51,
	"tiger claw":             52,
	"tracking":               53,
	"wind instruments":       54,
	"fishing":                55,
	"make poison":            56,
	"tinkering":              57,
	"research":               58,
	"alchemy":                59,
	"baking":                 60,
	"tailoring":              61,
	"sense traps":            62,
	"blacksmithing":          63,
	"fletching":              64,
	"brewing":                65,
	"alcohol tolerance":      66,
	"begging":                67,
	"jewelry making":         68,
	"pottery":                69,
	"percussion instruments": 70,
	"intimidation":           71,
	"berserking":             72,
	"taunt":                  73,
}

// SkillID resolves a skill display name to its EQMac skill_id. Returns
// (Unknown, false) when the name isn't recognised — callers should still
// record the skill by name, just without a cap.
func SkillID(name string) (int, bool) {
	id, ok := skillNameToID[strings.ToLower(strings.TrimSpace(name))]
	if !ok {
		return Unknown, false
	}
	return id, true
}

// specializeSkillIDs is the set of caster magic-school specialization skills
// (EQMac 43–47). On Project Quarm only one of these may exceed 50 (the
// character's chosen primary specialization); the rest lock at 50. We can't
// know the chosen school from skill_caps alone, but we can infer it from the
// observed values — see the API cap adjustment.
var specializeSkillIDs = map[int]bool{43: true, 44: true, 45: true, 46: true, 47: true}

// IsSpecialize reports whether skillID is one of the five Specialize <school>
// skills subject to the single-primary rule.
func IsSpecialize(skillID int) bool { return specializeSkillIDs[skillID] }
