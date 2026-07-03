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

// hostTag is the Project Quarm server tag Zeal appends to "format 1"
// (/outputfile format 1) export filenames — the same token that appears in the
// log filename eqlog_<Char>_pq.proj.txt. Format 0 omits it entirely.
const hostTag = "_pq.proj"

// FindInventoryFile, FindSpellbookFile and FindQuarmyFile return the path to the
// most-recently-modified Zeal export of that type for character, checking BOTH
// the legacy format-0 name (<Char>-<Type>.txt) and the format-1 name
// (<Char>-<Type>_pq.proj.txt). They return "" when neither exists.
//
// /outputfile format is a Zeal toggle (issue #133): supporting both means the
// user never has to pick one, and when both files are present the newer wins.
func FindInventoryFile(eqPath, character string) string {
	return findExport(eqPath, character, "Inventory")
}

func FindSpellbookFile(eqPath, character string) string {
	return findExport(eqPath, character, "Spellbook")
}

func FindQuarmyFile(eqPath, character string) string {
	return findExport(eqPath, character, "Quarmy")
}

func findExport(eqPath, character, typ string) string {
	return newestExisting(
		filepath.Join(eqPath, fmt.Sprintf("%s-%s.txt", character, typ)),
		filepath.Join(eqPath, fmt.Sprintf("%s-%s%s.txt", character, typ, hostTag)),
	)
}

// newestExisting returns the path with the latest mod time among those that
// exist on disk, or "" if none of them do.
func newestExisting(paths ...string) string {
	best := ""
	var bestMod time.Time
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		if best == "" || info.ModTime().After(bestMod) {
			best, bestMod = p, info.ModTime()
		}
	}
	return best
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

// inventorySlotAliases maps the numbered equipment-slot names emitted by the
// "_pq.proj" (format 1) Zeal export to the canonical names used by the plain
// (format 0) export and the rest of the app. EQ's three doubled slots come out
// as Ear1/Ear2, Wrist1/Wrist2, Finger1/Finger2 in format 1, but as repeated
// Ear/Wrist/Fingers rows in format 0. Normalizing at parse time means every
// downstream consumer (equip-focus resolution, the character gear panel, the
// inventory tracker) sees one slot vocabulary regardless of export format.
//
// Note "Finger" (singular, format 1) → "Fingers" (plural, the canonical name in
// equipSlots). Bag slots like "General1" are intentionally NOT in this map — a
// blanket digit strip would wrongly collapse them to "General".
//
// Fixes #137: format-1 users saw empty Ear / Ring (Fingers) / Wrist slots on
// the character inventory & equipment screens.
var inventorySlotAliases = map[string]string{
	"Ear1": "Ear", "Ear2": "Ear",
	"Wrist1": "Wrist", "Wrist2": "Wrist",
	"Finger1": "Fingers", "Finger2": "Fingers",
}

// canonicalSlot maps a raw export slot name to the app's canonical name,
// passing through anything not in inventorySlotAliases unchanged.
func canonicalSlot(loc string) string {
	if canon, ok := inventorySlotAliases[loc]; ok {
		return canon
	}
	return loc
}

// parseInventoryLine parses one tab-delimited inventory row.
// Expected: Location\tName\tID\tCount\tSlots (format-1 exports use a
// "Count/Charges" header but the same column positions).
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
		Location: canonicalSlot(strings.TrimSpace(parts[0])),
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
		Names:      []string{},
	}

	seen := make(map[int]bool)
	seenName := make(map[string]bool)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}

		id, name, ok := parseSpellbookLine(line)
		if !ok || id <= 0 {
			continue
		}
		if !seen[id] {
			seen[id] = true
			sb.SpellIDs = append(sb.SpellIDs, id)
		}
		if name != "" {
			key := strings.ToLower(name)
			if !seenName[key] {
				seenName[key] = true
				sb.Names = append(sb.Names, name)
			}
		}
	}

	return sb, scanner.Err()
}

// parseSpellbookLine extracts a spell ID (and, when present, the spell name)
// from a spellbook export line. Handles every /outputfile variant:
//
//	"1234"                       → id only
//	"3\t1234"                    → slot\tspell_id
//	"1234\tSome Spell Name"      → spell_id\tname
//	"26\t1359\t8\tEnchant Clay"  → index\tspell_id\tlevel\tname (modern format)
//
// The id keeps the legacy heuristic (prefer the 2nd field, which is the spell
// id in the slot/index-led formats, else the 1st). The name is the trailing
// field when it is non-numeric, so name-based matching keeps working even if
// the exported spell id ever diverges from the bundled quarm.db id.
func parseSpellbookLine(line string) (id int, name string, ok bool) {
	parts := strings.Split(line, "\t")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}

	if len(parts) == 1 {
		n, err := strconv.Atoi(parts[0])
		return n, "", err == nil
	}

	// Prefer the 2nd field (slot\tspell_id / index\tspell_id\t...), then the 1st
	// (spell_id\tname).
	if n, err := strconv.Atoi(parts[1]); err == nil {
		id = n
	} else if n, err := strconv.Atoi(parts[0]); err == nil {
		id = n
	} else {
		return 0, "", false
	}

	// The name is the last field when it isn't itself a number (rules out the
	// slot\tspell_id form, whose trailing field is the id).
	if last := parts[len(parts)-1]; last != "" {
		if _, err := strconv.Atoi(last); err != nil {
			name = last
		}
	}

	return id, name, true
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

// WriteSpellsets serializes a SpellsetFile back to its INI format and writes it
// to path atomically (temp file + rename). Each spellset becomes a [section]
// followed by keys 0..7 = <spell_id>; missing slots are written as -1.
func WriteSpellsets(path string, sf *SpellsetFile) error {
	if sf == nil {
		return fmt.Errorf("nil spellset file")
	}
	var buf strings.Builder
	for _, s := range sf.Spellsets {
		buf.WriteByte('[')
		buf.WriteString(s.Name)
		buf.WriteString("]\n")
		for i := 0; i < SpellsetSlotCount; i++ {
			id := -1
			if i < len(s.SpellIDs) {
				id = s.SpellIDs[i]
			}
			fmt.Fprintf(&buf, "%d=%d\n", i, id)
		}
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".spellsets-*.ini")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }
	if _, err := tmp.WriteString(buf.String()); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		cleanup()
		return fmt.Errorf("rename: %w", err)
	}
	return nil
}

// BandolierPath returns the expected Zeal bandolier path for a character.
// Zeal writes: <eq_path>/<CharName>_bandolier.ini
func BandolierPath(eqPath, character string) string {
	return filepath.Join(eqPath, fmt.Sprintf("%s_bandolier.ini", character))
}

// ParseBandolier reads and parses a Zeal bandolier INI file.
//
// Format:
//
//	[set name]
//	0=<item_id>   ; Primary
//	1=<item_id>   ; Secondary
//	2=<item_id>   ; Range
//	3=<item_id>   ; Ammo
//
// Item IDs of 0 indicate an empty slot. Slots not present in a section default
// to 0. Sections are returned in the order they appear in the file (matches the
// in-game /bandolier list order).
func ParseBandolier(path, character string) (*BandolierFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return nil, err
	}

	out := &BandolierFile{
		Character:  character,
		ExportedAt: info.ModTime(),
		Sets:       []BandolierSet{},
	}

	var cur *BandolierSet
	flush := func() {
		if cur == nil {
			return
		}
		out.Sets = append(out.Sets, *cur)
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
			cur = &BandolierSet{Name: name, ItemIDs: make([]int, BandolierSlotCount)}
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
		if err != nil || slot < 0 || slot >= BandolierSlotCount {
			continue
		}
		id, err := strconv.Atoi(strings.TrimSpace(line[eq+1:]))
		if err != nil {
			continue
		}
		cur.ItemIDs[slot] = id
	}
	flush()

	return out, scanner.Err()
}

// WriteBandolier serializes a BandolierFile back to its INI format and writes it
// to path atomically (temp file + rename). Each set becomes a [section] followed
// by keys 0..3 = <item_id>; missing slots are written as 0.
func WriteBandolier(path string, bf *BandolierFile) error {
	if bf == nil {
		return fmt.Errorf("nil bandolier file")
	}
	var buf strings.Builder
	for _, s := range bf.Sets {
		buf.WriteByte('[')
		buf.WriteString(s.Name)
		buf.WriteString("]\n")
		for i := 0; i < BandolierSlotCount; i++ {
			id := 0
			if i < len(s.ItemIDs) {
				id = s.ItemIDs[i]
			}
			fmt.Fprintf(&buf, "%d=%d\n", i, id)
		}
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".bandolier-*.ini")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpPath) }
	if _, err := tmp.WriteString(buf.String()); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		cleanup()
		return fmt.Errorf("rename: %w", err)
	}
	return nil
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
