// Package enums centralizes all interpretation of raw numeric codes from
// the Project Quarm SQL dump (skill IDs, item types, special abilities,
// tradeskills, etc.). Each enum file in this package cites its
// authoritative upstream source so that future dump refreshes can be
// audited against a known canonical mapping.
//
// Precedence when interpreting a code:
//
//  1. Quarm dump wins on values that exist in it — overrides are clearly
//     marked in their source file.
//  2. EQMacEmu (github.com/EQMacEmu/Server) is the primary citation; its
//     era matches Project Quarm's frozen timeline (classic through
//     Shadows of Luclin, with Quarm-added PoP content).
//  3. Modern EQEmu (github.com/EQEmu/Server) is the fallback for codes
//     present in newer schemas but undocumented in the Mac fork.
//
// Per-file convention: each enum file declares its label map and exposes
// a Label-style accessor. A `// Source:` comment names the upstream file
// or, when no canonical exists, marks the entry as "Quarm-specific —
// derived empirically."
package enums

// Catalog is the public snapshot of all enums in this package, served to
// the frontend via /api/enums so that UI labels stay in sync with the
// backend's source of truth.
type Catalog struct {
	SpecialAbilities map[int]SpecialAbilityMeta `json:"special_abilities"`
	Tradeskills      map[int]string             `json:"tradeskills"`
	ItemTypes        map[int]string             `json:"item_types"`
	NPCClasses       map[int]string             `json:"npc_classes"`
	NPCRaces         map[int]string             `json:"npc_races"`
	ItemSlotBits     map[int]string             `json:"item_slot_bits"`
	ItemClassBits    map[int]string             `json:"item_class_bits"`
	ItemRaceBits     map[int]string             `json:"item_race_bits"`
	BaneBodies       map[int]string             `json:"bane_bodies"`
	BaneRaces        map[int]string             `json:"bane_races"`
}

// Snapshot returns the current Catalog for serialization.
func Snapshot() Catalog {
	return Catalog{
		SpecialAbilities: specialAbilities,
		Tradeskills:      tradeskills,
		ItemTypes:        itemTypes,
		NPCClasses:       npcClasses,
		NPCRaces:         npcRaces,
		ItemSlotBits:     itemSlotBits,
		ItemClassBits:    itemClassBits,
		ItemRaceBits:     itemRaceBits,
		BaneBodies:       baneBodyTypes,
		BaneRaces:        baneRaces,
	}
}
