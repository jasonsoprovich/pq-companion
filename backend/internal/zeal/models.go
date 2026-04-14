// Package zeal handles Zeal export file parsing and watching.
// Zeal (https://github.com/iamclint/Zeal) is an EverQuest client plugin that
// exports inventory and spellbook data to text files on character logout.
package zeal

import "time"

// InventoryEntry is one row from a Zeal inventory export file.
// The file is tab-delimited with columns: Location, Name, ID, Count, Slots.
type InventoryEntry struct {
	Location string `json:"location"` // e.g. "Head", "General1", "General1:Slot1"
	Name     string `json:"name"`
	ID       int    `json:"id"`
	Count    int    `json:"count"`
	Slots    int    `json:"slots"` // bag capacity; 0 for non-containers
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
