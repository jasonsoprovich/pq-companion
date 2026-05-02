// Package zeal handles Zeal export file parsing and watching.
// Zeal (https://github.com/iamclint/Zeal) is an EverQuest client plugin that
// exports inventory and spellbook data to text files on character logout.
package zeal

import "time"

// InventoryEntry is one row from a Zeal inventory export file.
// The file is tab-delimited with columns: Location, Name, ID, Count, Slots.
// Icon is not in the export — it's joined in by the API layer from the items DB.
type InventoryEntry struct {
	Location string `json:"location"` // e.g. "Head", "General1", "General1:Slot1"
	Name     string `json:"name"`
	ID       int    `json:"id"`
	Count    int    `json:"count"`
	Slots    int    `json:"slots"` // bag capacity; 0 for non-containers
	Icon     int    `json:"icon,omitempty"`
}

// Inventory is the full parsed state of a character's inventory export.
type Inventory struct {
	Character  string           `json:"character"`
	ExportedAt time.Time        `json:"exported_at"`
	Entries    []InventoryEntry `json:"entries"`
}

// SpellbookEntry represents one known spell from a Zeal spellbook export.
type SpellbookEntry struct {
	SpellID int `json:"spell_id"`
}

// Spellbook is the full parsed state of a character's spellbook export.
type Spellbook struct {
	Character  string           `json:"character"`
	ExportedAt time.Time        `json:"exported_at"`
	SpellIDs   []int            `json:"spell_ids"`
}

// State is the combined in-memory snapshot held by the Watcher.
type State struct {
	Inventory *Inventory `json:"inventory"`
	Spellbook *Spellbook `json:"spellbook"`
}

// AllInventoriesResponse is returned by GET /api/zeal/all-inventories.
// Configured is false when the EQ path has not been set in config.
// SharedBank contains deduplicated entries from the most-recently-modified export file.
type AllInventoriesResponse struct {
	Configured bool             `json:"configured"`
	Characters []*Inventory     `json:"characters"`
	SharedBank []InventoryEntry `json:"shared_bank"`
}

// CharStats holds base (unmodified) character stats from the quarmy.txt header.
type CharStats struct {
	BaseSTR int `json:"base_str"`
	BaseSTA int `json:"base_sta"`
	BaseCHA int `json:"base_cha"`
	BaseDEX int `json:"base_dex"`
	BaseINT int `json:"base_int"`
	BaseAGI int `json:"base_agi"`
	BaseWIS int `json:"base_wis"`
}

// AAEntry is one purchased AA ability and its rank from the quarmy.txt AA section.
type AAEntry struct {
	ID   int `json:"id"`
	Rank int `json:"rank"`
}

// QuarmyData is the parsed contents of a <CharName>-Quarmy.txt file.
// It contains character identity (level/class/race), stats, inventory, and AAs.
//
// Class and Race are the raw EQ 1-indexed IDs as written by Zeal:
//   Class: 1=Warrior … 14=Enchanter, 15=Beastlord
//   Race:  1=Human, 2=Barbarian, 3=Erudite, 4=Wood Elf, …
// Callers persisting to user.db must convert Class to the app's 0-indexed scheme.
type QuarmyData struct {
	Character  string           `json:"character"`
	ExportedAt time.Time        `json:"exported_at"`
	Level      int              `json:"level"`
	Class      int              `json:"class"`
	Race       int              `json:"race"`
	Stats      CharStats        `json:"stats"`
	Inventory  []InventoryEntry `json:"inventory"`
	AAs        []AAEntry        `json:"aas"`
}
