package zeal

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// InventoryPath returns the expected Zeal inventory export path for a character.
// Zeal writes: <eq_path>/<CharName>_pq.proj-Inventory.txt
func InventoryPath(eqPath, character string) string {
	return filepath.Join(eqPath, fmt.Sprintf("%s_pq.proj-Inventory.txt", character))
}

// SpellbookPath returns the expected Zeal spellbook export path for a character.
// Zeal writes: <eq_path>/<CharName>_pq.proj-Spells.txt
func SpellbookPath(eqPath, character string) string {
	return filepath.Join(eqPath, fmt.Sprintf("%s_pq.proj-Spells.txt", character))
}

// ParseInventory reads and parses a Zeal inventory export file.
// Format: tab-delimited with header row: Location\tName\tID\tCount\tSlots
// Returns a non-nil Inventory even if the file is empty (zero entries).
func ParseInventory(path, character string) (*Inventory, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	modTime := info.ModTime()

	inv := &Inventory{
		Character:  character,
		ExportedAt: modTime,
		Entries:    []InventoryEntry{},
	}

	scanner := bufio.NewScanner(f)
	firstLine := true
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Skip the header row.
		if firstLine {
			firstLine = false
			// Detect header by checking if first field is "Location" (case-insensitive).
			parts := strings.SplitN(line, "\t", 2)
			if len(parts) > 0 && strings.EqualFold(strings.TrimSpace(parts[0]), "location") {
				continue
			}
		}

		entry, ok := parseInventoryLine(line)
		if ok {
			inv.Entries = append(inv.Entries, entry)
		}
	}

	return inv, scanner.Err()
}

// parseInventoryLine parses one tab-delimited inventory row.
// Expected: Location\tName\tID\tCount\tSlots
func parseInventoryLine(line string) (InventoryEntry, bool) {
	parts := strings.Split(line, "\t")
	if len(parts) < 4 {
		return InventoryEntry{}, false
	}

	id, err := strconv.Atoi(strings.TrimSpace(parts[2]))
	if err != nil {
		return InventoryEntry{}, false
	}

	count, _ := strconv.Atoi(strings.TrimSpace(parts[3]))
	if count == 0 {
		count = 1
	}

	slots := 0
	if len(parts) >= 5 {
		slots, _ = strconv.Atoi(strings.TrimSpace(parts[4]))
	}

	return InventoryEntry{
		Location: strings.TrimSpace(parts[0]),
		Name:     strings.TrimSpace(parts[1]),
		ID:       id,
		Count:    count,
		Slots:    slots,
	}, true
}

// ParseSpellbook reads and parses a Zeal spellbook export file.
// Zeal writes one spell per line in one of several formats:
//
//	<spell_id>
//	<slot>\t<spell_id>
//	<spell_id>\t<spell_name>
//
// The parser tries each variant, preferring to extract the numeric spell ID.
func ParseSpellbook(path, character string) (*Spellbook, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	modTime := info.ModTime()

	sb := &Spellbook{
		Character:  character,
		ExportedAt: modTime,
		SpellIDs:   []int{},
	}

	seen := make(map[int]bool)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		id, ok := parseSpellbookLine(line)
		if ok && id > 0 && !seen[id] {
			seen[id] = true
			sb.SpellIDs = append(sb.SpellIDs, id)
		}
	}

	return sb, scanner.Err()
}

// parseSpellbookLine extracts a spell ID from a spellbook export line.
// Handles: "1234", "3\t1234", "1234\tSome Spell Name"
func parseSpellbookLine(line string) (int, bool) {
	parts := strings.SplitN(line, "\t", 3)

	// Single token: must be the spell ID.
	if len(parts) == 1 {
		id, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		return id, err == nil
	}

	// Two tokens: either "slot\tspell_id" or "spell_id\tname".
	// Try second field first (slot\tspell_id is common in Zeal).
	if id, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil {
		return id, true
	}
	// Fall back to first field.
	if id, err := strconv.Atoi(strings.TrimSpace(parts[0])); err == nil {
		return id, true
	}

	return 0, false
}

// ModTime returns the modification time of the file at path, or zero if not found.
func ModTime(path string) time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}
