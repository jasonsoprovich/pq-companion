package db

import (
	"strconv"
	"strings"
)

// SpecialAbility names keyed by their numeric code.
// Source: CLAUDE.md and EQEmu documentation.
var specialAbilityNames = map[int]string{
	1:  "Summon",
	2:  "Enrage",
	3:  "Rampage",
	4:  "Flurry",
	5:  "Triple Attack",
	6:  "Dual Wield",
	12: "Immune to Melee",
	13: "Immune to Magic",
	17: "Uncharmable",
	18: "Unmezzable",
	19: "Unfearable",
	20: "Immune to Slow",
	24: "No Target",
	26: "See Through Invis",
	28: "See Through Invis vs Undead",
}

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
