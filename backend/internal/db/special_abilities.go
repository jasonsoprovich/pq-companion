package db

import (
	"strconv"
	"strings"
)

// SpecialAbility names keyed by their numeric code.
//
// Canonical mapping from Project Quarm's EQMacEmu fork's
// `SpecialAbility` namespace in `common/emu_constants.h` — these are the
// codes the server itself uses when reading the `npc_types.special_abilities`
// column, and they differ from modern EQEmu master numbering.
var specialAbilityNames = map[int]string{
	1:  "Summon",
	2:  "Enrage",
	3:  "Rampage",
	4:  "Area Rampage",
	5:  "Flurry",
	6:  "Triple Attack",
	7:  "Dual Wield",
	8:  "Disallow Equip",
	9:  "Bane Attack",
	10: "Magical Attack",
	11: "Ranged Attack",
	12: "Immune to Slow",
	13: "Immune to Mez",
	14: "Immune to Charm",
	15: "Immune to Stun",
	16: "Immune to Snare",
	17: "Immune to Fear",
	18: "Immune to Dispel",
	19: "Immune to Melee",
	20: "Immune to Magic",
	21: "Immune to Fleeing",
	22: "Immune to Non-Bane Melee",
	23: "Immune to Non-Magical Melee",
	24: "Immune to Aggro",
	25: "Immune to Being Aggro'd",
	26: "Immune to Ranged Spells",
	27: "Immune to Feign Death",
	28: "Immune to Taunt",
	29: "Tunnel Vision",
	30: "Won't Heal/Buff Allies",
	31: "Immune to Pacify",
	32: "Leashed",
	33: "Tethered",
	34: "Permaroot Flee",
	35: "Immune to Harm from Client",
	36: "Always Flees",
	37: "Flee Percent",
	38: "Allows Beneficial Spells",
	39: "Melee Disabled",
	40: "Chase Distance",
	41: "Allowed to Tank",
	42: "Proximity Aggro",
	43: "Always Calls for Help",
	44: "Use Warrior Skills",
	45: "Always Flee on Low Con",
	46: "No Loitering",
	47: "Block Handin on Bad Faction",
	48: "PC Deathblow Corpse",
	49: "Corpse Camper",
	50: "Reverse Slow",
	51: "Immune to Haste",
	52: "Immune to Disarm",
	53: "Immune to Riposte",
	54: "Proximity Aggro 2",
}

// Synthetic ability codes used by the overlay layer for flags that are
// stored on dedicated NPC columns rather than encoded in the
// `special_abilities` string. They sit well above SpecialAbility::Max (55)
// so they can never collide with a real Quarm code.
const (
	SyntheticSeeInvis       = 1001
	SyntheticSeeInvisUndead = 1002
)

// SpecialAbility represents a single parsed ability from the special_abilities field.
type SpecialAbility struct {
	Code  int    `json:"code"`
	Value int    `json:"value"`
	Name  string `json:"name,omitempty"`
}

// ParseSpecialAbilities parses the caret-delimited special_abilities string
// from npc_types into a slice of SpecialAbility.
// Format: "code,value^code,value^..." (e.g. "1,1^18,1^19,1")
// Empty or null input returns nil.
func ParseSpecialAbilities(raw string) []SpecialAbility {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	parts := strings.Split(raw, "^")
	result := make([]SpecialAbility, 0, len(parts))

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, ",", 2)
		if len(kv) != 2 {
			continue
		}
		code, err := strconv.Atoi(strings.TrimSpace(kv[0]))
		if err != nil {
			continue
		}
		val, err := strconv.Atoi(strings.TrimSpace(kv[1]))
		if err != nil {
			continue
		}
		sa := SpecialAbility{
			Code:  code,
			Value: val,
			Name:  specialAbilityNames[code],
		}
		result = append(result, sa)
	}

	return result
}

// HasSpecialAbility returns true if the raw abilities string contains the given code.
func HasSpecialAbility(raw string, code int) bool {
	for _, sa := range ParseSpecialAbilities(raw) {
		if sa.Code == code {
			return sa.Value != 0
		}
	}
	return false
}
