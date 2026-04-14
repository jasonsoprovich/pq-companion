package zeal

import (
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// inventoryFileRe matches "<CharName>_pq.proj-Inventory.txt" and captures the character name.
var inventoryFileRe = regexp.MustCompile(`(?i)^(.+?)_pq\.proj-Inventory\.txt$`)

// ScanAllInventories discovers and parses every Zeal inventory export in eqPath.
//
// It returns:
//   - chars: one *Inventory per discovered file, with SharedBank entries removed
//   - sharedBank: the SharedBank entries from the most-recently-modified export file
func ScanAllInventories(eqPath string) ([]*Inventory, []InventoryEntry, error) {
	pattern := filepath.Join(eqPath, "*_pq.proj-Inventory.txt")
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
