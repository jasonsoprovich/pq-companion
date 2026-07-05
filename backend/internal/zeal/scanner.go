package zeal

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Project Quarm exposes 10 shared bank slots. Zeal exports 30 (modern-EQ
// inventory layout), so drop the empties that the server can never populate.
const maxSharedBankSlot = 10

// sharedBankLocRe captures the slot number from "SharedBank<N>" or
// "SharedBank<N>-Slot<X>" / "SharedBank<N>:Slot<X>".
var sharedBankLocRe = regexp.MustCompile(`^SharedBank(\d+)(?:[:\-]Slot\d+)?$`)

func sharedBankSlotInRange(location string) bool {
	m := sharedBankLocRe.FindStringSubmatch(location)
	if m == nil {
		return false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return false
	}
	return n >= 1 && n <= maxSharedBankSlot
}

// inventoryFileRe matches a Zeal inventory export in either /outputfile format
// — "<CharName>-Inventory.txt" (format 0) or "<CharName>-Inventory_pq.proj.txt"
// (format 1, #133) — and captures the character name.
var inventoryFileRe = regexp.MustCompile(`(?i)^(.+?)-Inventory(?:_pq\.proj)?\.txt$`)

// spellsetFileRe matches "<CharName>_spellsets.ini" and captures the character name.
var spellsetFileRe = regexp.MustCompile(`(?i)^(.+?)_spellsets\.ini$`)

// bandolierFileRe matches "<CharName>_bandolier.ini" and captures the character name.
var bandolierFileRe = regexp.MustCompile(`(?i)^(.+?)_bandolier\.ini$`)

// macroFileRe matches "<CharName>_pq.proj.ini" and captures the character name.
// EQ character names never contain an underscore, so the capture is restricted
// to an underscore-free name. That deliberately excludes the client's OTHER
// per-character config files that share the _pq.proj.ini suffix — window
// layouts (UI_<Char>_pq.proj.ini) and bazaar state (BZR_<Char>_pq.proj.ini) —
// which would otherwise be scanned as bogus "characters" in the macro editor.
var macroFileRe = regexp.MustCompile(`(?i)^([^_]+)_pq\.proj\.ini$`)

// ScanAllInventories discovers and parses every Zeal inventory export in eqPath.
//
// It returns:
//   - chars: one *Inventory per discovered file, with SharedBank entries removed
//   - sharedBank: the SharedBank entries from the most-recently-modified export file
func ScanAllInventories(eqPath string) ([]*Inventory, []InventoryEntry, error) {
	// Glob broadly and let inventoryFileRe filter, so both /outputfile formats
	// (<Char>-Inventory.txt and <Char>-Inventory_pq.proj.txt) are picked up.
	matches, err := filepath.Glob(filepath.Join(eqPath, "*-Inventory*.txt"))
	if err != nil {
		return nil, nil, err
	}

	type parsed struct {
		inv   *Inventory
		sbEnt []InventoryEntry
	}

	// Keep one parsed result per character — the most recently exported — so a
	// user who has both naming formats on disk doesn't get duplicate characters
	// in the all-inventories view (#133).
	byChar := make(map[string]parsed)
	for _, path := range matches {
		base := filepath.Base(path)
		m := inventoryFileRe.FindStringSubmatch(base)
		if m == nil {
			continue
		}
		character := m[1]
		inv, parseErr := ParseInventory(path, character)
		if parseErr != nil {
			continue
		}

		// Split SharedBank entries from character-specific entries. Zeal exports
		// modern-EQ shared bank slots 1–30, but Project Quarm only uses 1–10;
		// drop the rest so the UI doesn't render 20 empty containers.
		var charEnt, sbEnt []InventoryEntry
		for _, e := range inv.Entries {
			if strings.HasPrefix(e.Location, "SharedBank") {
				if sharedBankSlotInRange(e.Location) {
					sbEnt = append(sbEnt, e)
				}
			} else {
				charEnt = append(charEnt, e)
			}
		}
		inv.Entries = charEnt

		if existing, ok := byChar[character]; ok && !inv.ExportedAt.After(existing.inv.ExportedAt) {
			continue // an equal-or-newer export for this character already won
		}
		byChar[character] = parsed{inv, sbEnt}
	}

	chars := make([]*Inventory, 0, len(byChar))
	var sharedBank []InventoryEntry
	var newestTime time.Time

	for _, r := range byChar {
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

// ScanAllBandoliers discovers and parses every <CharName>_bandolier.ini file in
// eqPath. Returns one *BandolierFile per character whose file parses successfully.
func ScanAllBandoliers(eqPath string) ([]*BandolierFile, error) {
	pattern := filepath.Join(eqPath, "*_bandolier.ini")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	out := make([]*BandolierFile, 0, len(matches))
	for _, path := range matches {
		base := filepath.Base(path)
		m := bandolierFileRe.FindStringSubmatch(base)
		if m == nil {
			continue
		}
		bf, err := ParseBandolier(path, m[1])
		if err != nil {
			continue
		}
		out = append(out, bf)
	}
	return out, nil
}

// ScanAllMacros discovers and parses the [Socials] section of every
// <CharName>_pq.proj.ini file in eqPath. Returns one *MacroFile per character
// whose file parses successfully.
func ScanAllMacros(eqPath string) ([]*MacroFile, error) {
	pattern := filepath.Join(eqPath, "*_pq.proj.ini")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return nil, err
	}

	out := make([]*MacroFile, 0, len(matches))
	for _, path := range matches {
		base := filepath.Base(path)
		m := macroFileRe.FindStringSubmatch(base)
		if m == nil {
			continue
		}
		mf, err := ParseMacros(path, m[1])
		if err != nil {
			continue
		}
		out = append(out, mf)
	}
	return out, nil
}
