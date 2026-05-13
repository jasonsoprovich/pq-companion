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
// Zeal writes: <eq_path>/<CharName>-Inventory.txt
func InventoryPath(eqPath, character string) string {
	return filepath.Join(eqPath, fmt.Sprintf("%s-Inventory.txt", character))
}

// SpellbookPath returns the expected Zeal spellbook export path for a character.
// Zeal writes: <eq_path>/<CharName>-Spellbook.txt
func SpellbookPath(eqPath, character string) string {
	return filepath.Join(eqPath, fmt.Sprintf("%s-Spellbook.txt", character))
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

// SpellsetPath returns the expected Zeal spellsets export path for a character.
// Zeal writes: <eq_path>/<CharName>_spellsets.ini
func SpellsetPath(eqPath, character string) string {
	return filepath.Join(eqPath, fmt.Sprintf("%s_spellsets.ini", character))
}

// ParseSpellsets reads and parses a Zeal spellsets INI file.
//
// Format:
//
//	[set name]
//	0=<spell_id>
//	...
//	7=<spell_id>
//
// Spell IDs of -1 indicate an empty gem. Slots not present in a section default to -1.
// Sections are returned in the order they appear in the file (matches the in-game dropdown).
func ParseSpellsets(path, character string) (*SpellsetFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}

	out := &SpellsetFile{
		Character:  character,
		ExportedAt: info.ModTime(),
		Spellsets:  []Spellset{},
	}

	var cur *Spellset
	flush := func() {
		if cur == nil {
			return
		}
		out.Spellsets = append(out.Spellsets, *cur)
		cur = nil
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			flush()
			name := strings.TrimSpace(line[1 : len(line)-1])
			ids := make([]int, SpellsetSlotCount)
			for i := range ids {
				ids[i] = -1
			}
			cur = &Spellset{Name: name, SpellIDs: ids}
			continue
		}

		if cur == nil {
			continue
		}

		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		slot, err := strconv.Atoi(strings.TrimSpace(line[:eq]))
		if err != nil || slot < 0 || slot >= SpellsetSlotCount {
			continue
		}
		id, err := strconv.Atoi(strings.TrimSpace(line[eq+1:]))
		if err != nil {
			continue
		}
		cur.SpellIDs[slot] = id
	}
	flush()

	return out, scanner.Err()
}

// QuarmyPath returns the expected Zeal quarmy export path for a character.
// Zeal writes: <eq_path>/<CharName>-Quarmy.txt
func QuarmyPath(eqPath, character string) string {
	return filepath.Join(eqPath, fmt.Sprintf("%s-Quarmy.txt", character))
}

// ParseQuarmy reads and parses a Zeal quarmy export file.
// The file has three sections separated by header rows:
//  1. Character stats header + one data row (BaseSTR … BaseWIS)
//  2. Inventory section (identical format to -Inventory.txt)
//  3. AA section: "AAIndex\tRank" header followed by id\trank rows
//
// Returns a non-nil QuarmyData even if individual sections are missing.
func ParseQuarmy(path, character string) (*QuarmyData, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}

	data := &QuarmyData{
		Character:  character,
		ExportedAt: info.ModTime(),
		Inventory:  []InventoryEntry{},
		AAs:        []AAEntry{},
	}

	type section int
	const (
		secStats     section = iota // lines 1-2: char header + data
		secInventory                // lines 3+: inventory
		secAA                       // after "AAIndex" header
	)

	cur := secStats
	statsHeaderSeen := false

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Detect section transitions by first field.
		firstField := strings.ToLower(strings.SplitN(line, "\t", 2)[0])

		switch {
		case firstField == "aaindex":
			cur = secAA
			continue
		case firstField == "location":
			cur = secInventory
			continue
		case firstField == "checksum":
			continue
		}

		switch cur {
		case secStats:
			if !statsHeaderSeen && firstField == "character" {
				statsHeaderSeen = true
				continue // skip header row
			}
			// Data row: Character\tName\tLastName\tLevel\tClass\tRace\tGender\tDeity\tGuild\tGuildRank\tBaseSTR\tBaseSTA\tBaseCHA\tBaseDEX\tBaseINT\tBaseAGI\tBaseWIS
			parts := strings.Split(line, "\t")
			if len(parts) >= 17 {
				data.Level = parseInt(parts[3])
				data.Class = parseInt(parts[4])
				data.Race = parseInt(parts[5])
				data.Stats = CharStats{
					BaseSTR: parseInt(parts[10]),
					BaseSTA: parseInt(parts[11]),
					BaseCHA: parseInt(parts[12]),
					BaseDEX: parseInt(parts[13]),
					BaseINT: parseInt(parts[14]),
					BaseAGI: parseInt(parts[15]),
					BaseWIS: parseInt(parts[16]),
				}
			}
			cur = secInventory // next section starts after stats row

		case secInventory:
			entry, ok := parseInventoryLine(line)
			if ok {
				data.Inventory = append(data.Inventory, entry)
			}

		case secAA:
			parts := strings.SplitN(line, "\t", 2)
			if len(parts) == 2 {
				id, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
				rank, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
				if err1 == nil && err2 == nil && rank > 0 {
					data.AAs = append(data.AAs, AAEntry{ID: id, Rank: rank})
				}
			}
		}
	}

	return data, scanner.Err()
}

// parseInt converts a string to int, returning 0 on error.
func parseInt(s string) int {
	n, _ := strconv.Atoi(strings.TrimSpace(s))
	return n
}

// ModTime returns the modification time of the file at path, or zero if not found.
func ModTime(path string) time.Time {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}
