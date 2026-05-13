package zeal

import (
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// inventoryFileRe matches "<CharName>-Inventory.txt" and captures the character name.
var inventoryFileRe = regexp.MustCompile(`(?i)^(.+?)-Inventory\.txt$`)

// spellsetFileRe matches "<CharName>_spellsets.ini" and captures the character name.
var spellsetFileRe = regexp.MustCompile(`(?i)^(.+?)_spellsets\.ini$`)

// ScanAllInventories discovers and parses every Zeal inventory export in eqPath.
//
// It returns:
//   - chars: one *Inventory per discovered file, with SharedBank entries removed
//   - sharedBank: the SharedBank entries from the most-recently-modified export file
func ScanAllInventories(eqPath string) ([]*Inventory, []InventoryEntry, error) {
	pattern := filepath.Join(eqPath, "*-Inventory.txt")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, nil, err
	}

	type parsed struct {
		inv   *Inventory
		sbEnt []InventoryEntry
	}

	results := make([]parsed, 0, len(matches))
	for _, path := range matches {
		base := filepath.Base(path)
		m := inventoryFileRe.FindStringSubmatch(base)
		if m == nil {
			continue
		}
		inv, parseErr := ParseInventory(path, m[1])
		if parseErr != nil {
			continue
		}

		// Split SharedBank entries from character-specific entries.
		var charEnt, sbEnt []InventoryEntry
		for _, e := range inv.Entries {
			if strings.HasPrefix(e.Location, "SharedBank") {
				sbEnt = append(sbEnt, e)
			} else {
				charEnt = append(charEnt, e)
			}
		}
		inv.Entries = charEnt
		results = append(results, parsed{inv, sbEnt})
	}

	chars := make([]*Inventory, 0, len(results))
	var sharedBank []InventoryEntry
	var newestTime time.Time

	for _, r := range results {
		chars = append(chars, r.inv)
		if r.inv.ExportedAt.After(newestTime) {
			newestTime = r.inv.ExportedAt
			sharedBank = r.sbEnt
		}
	}

	return chars, sharedBank, nil
}

// ScanAllSpellsets discovers and parses every <CharName>_spellsets.ini file in eqPath.
// Returns one *SpellsetFile per character whose file parses successfully.
func ScanAllSpellsets(eqPath string) ([]*SpellsetFile, error) {
	pattern := filepath.Join(eqPath, "*_spellsets.ini")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	out := make([]*SpellsetFile, 0, len(matches))
	for _, path := range matches {
		base := filepath.Base(path)
		m := spellsetFileRe.FindStringSubmatch(base)
		if m == nil {
			continue
		}
		sf, err := ParseSpellsets(path, m[1])
		if err != nil {
			continue
		}
		out = append(out, sf)
	}
	return out, nil
}
