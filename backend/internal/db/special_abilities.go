package db

import (
	"strconv"
	"strings"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db/enums"
)

// SpecialAbility represents a single parsed ability from the
// npc_types.special_abilities column. Name is filled from the canonical
// enums.SpecialAbilityName mapping at parse time.
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
		result = append(result, SpecialAbility{
			Code:  code,
			Value: val,
			Name:  enums.SpecialAbilityName(code),
		})
	}

	return result
}
