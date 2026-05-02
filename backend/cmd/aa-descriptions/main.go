// aa-descriptions reads an EverQuest/TAKP eqstr_en.txt string table and emits
// a JSON map of altadv_vars.eqmacid → description for every AA whose name is
// present in the strings file.
//
// The Quarm SQLite dump (quarm.db) carries no AA description text — that data
// lives client-side in eqstr_en.txt (TAKP) / dbstr_us.txt (retail). Each AA
// name in the strings file is followed on the next line by its description,
// regardless of which numeric range the IDs land in. We match by exact name.
//
// Run once after pulling a new strings file from a TAKP client. The output
// JSON is checked into the repo and embedded into the Go binary at build
// time — see internal/db/aa_descriptions.go.
//
// Usage:
//
//	go run ./cmd/aa-descriptions \
//	    -strings /path/to/eqstr_en.txt \
//	    -db backend/data/quarm.db \
//	    -out backend/internal/db/aa_descriptions.json
package main

import (
	"bufio"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"

	_ "modernc.org/sqlite"
)

type stringsEntry struct {
	id   int
	text string
}

func main() {
	stringsPath := flag.String("strings", "", "path to eqstr_en.txt")
	dbPath := flag.String("db", "backend/data/quarm.db", "path to quarm.db")
	outPath := flag.String("out", "backend/internal/db/aa_descriptions.json", "output JSON path")
	flag.Parse()

	if *stringsPath == "" {
		log.Fatal("-strings is required")
	}

	entries, err := readStrings(*stringsPath)
	if err != nil {
		log.Fatalf("read strings: %v", err)
	}
	log.Printf("loaded %d strings", len(entries))

	aas, err := readAAs(*dbPath)
	if err != nil {
		log.Fatalf("read aas: %v", err)
	}
	log.Printf("loaded %d AA rows from altadv_vars", len(aas))

	// Index strings by exact text. An AA name can collide with unrelated
	// system strings, so we record every match and pick the best candidate
	// per name — the one whose immediate next entry looks like a real
	// description (longest, with sentence-ish content).
	byText := make(map[string][]int, len(entries))
	byNormText := make(map[string][]int, len(entries))
	for i, e := range entries {
		byText[e.text] = append(byText[e.text], i)
		byNormText[normalizeName(e.text)] = append(byNormText[normalizeName(e.text)], i)
	}

	// stringIDByID overrides the matched description for AAs whose entry in
	// eqstr_en.txt is misaligned (the line after the name is a different
	// AA's name or description). Detected by inspection: only "Advanced
	// Innate Strength" and "Advanced Innate Wisdom" suffer this in the
	// current TAKP strings file. Both real descriptions exist in the file —
	// they're just sitting at non-adjacent IDs.
	stringIDByID := map[int]int{
		129: 5550, // Advanced Innate Strength
		134: 5563, // Advanced Innate Wisdom
	}
	stringByID := make(map[int]string, len(entries))
	for _, e := range entries {
		stringByID[e.id] = e.text
	}

	descriptions := make(map[int]string, len(aas))
	missing := make([]string, 0)
	seen := make(map[string]bool)

	for _, aa := range aas {
		if seen[aa.name] {
			// Duplicate names in altadv_vars (legacy + current eqmacid) all
			// share the same description; reuse what we already resolved.
			if d, ok := descriptions[lookupByName(aa.name, aas, descriptions)]; ok {
				descriptions[aa.eqmacid] = d
			}
			continue
		}
		seen[aa.name] = true

		// Hard override first — these are known misalignments in the
		// strings file's data layout.
		if sid, ok := stringIDByID[aa.eqmacid]; ok {
			if t, ok := stringByID[sid]; ok {
				descriptions[aa.eqmacid] = t
				continue
			}
		}

		idxs := byText[aa.name]
		desc := pickDescription(idxs, entries)
		// Fall back to a normalized lookup for names that differ only by
		// punctuation/whitespace/extra qualifiers — e.g. DB has "Ayonaes
		// Tutelage" but strings file has "Ayonae's Tutelage", "Lifeburn" vs
		// "Life Burn", "Fletching Mastery" vs "Fletching/Bowyer Mastery".
		if desc == "" {
			desc = pickDescription(byNormText[normalizeName(aa.name)], entries)
		}
		if desc == "" {
			missing = append(missing, aa.name)
			continue
		}
		descriptions[aa.eqmacid] = desc
	}

	// Backfill duplicate-name AAs whose first encounter happened after the
	// canonical entry was recorded (loop order).
	for _, aa := range aas {
		if _, ok := descriptions[aa.eqmacid]; ok {
			continue
		}
		if d, ok := descriptions[lookupByName(aa.name, aas, descriptions)]; ok {
			descriptions[aa.eqmacid] = d
		}
	}

	if err := writeJSON(*outPath, descriptions); err != nil {
		log.Fatalf("write json: %v", err)
	}

	log.Printf("wrote %d descriptions to %s", len(descriptions), *outPath)
	if len(missing) > 0 {
		sort.Strings(missing)
		log.Printf("no description match for %d AA name(s):", len(missing))
		for _, n := range missing {
			fmt.Fprintf(os.Stderr, "  - %s\n", n)
		}
	}
}

func readStrings(path string) ([]stringsEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := make([]stringsEntry, 0, 8192)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	lineNum := 0
	for sc.Scan() {
		lineNum++
		line := sc.Text()
		// First two lines are header (`EQST0002`) + count (`0 5808`).
		if lineNum <= 2 {
			continue
		}
		// Each entry is `<id> <text>`. The text can contain spaces; we only
		// split on the first space.
		sp := strings.IndexByte(line, ' ')
		if sp < 0 {
			continue
		}
		id, err := strconv.Atoi(line[:sp])
		if err != nil {
			continue
		}
		out = append(out, stringsEntry{id: id, text: line[sp+1:]})
	}
	return out, sc.Err()
}

type aaRow struct {
	skillID int
	eqmacid int
	name    string
}

func readAAs(path string) ([]aaRow, error) {
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	// Match the filter ListAvailableAAs uses minus the class bitmask, so we
	// cover every AA that any class could see. Order by eqmacid for a
	// deterministic JSON output.
	rows, err := conn.Query(`
		SELECT skill_id, eqmacid, name
		FROM altadv_vars
		WHERE name != 'NOT USED'
		  AND cost > 0
		  AND eqmacid > 0
		  AND class_type != 0
		ORDER BY eqmacid
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []aaRow
	for rows.Next() {
		var r aaRow
		if err := rows.Scan(&r.skillID, &r.eqmacid, &r.name); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// pickDescription chooses the best description from candidate name-line
// indexes. The next line after the name is the description; we prefer the
// longest one because non-AA collisions tend to be short tooltips while AA
// description text is paragraph-length.
func pickDescription(idxs []int, entries []stringsEntry) string {
	best := ""
	for _, i := range idxs {
		if i+1 >= len(entries) {
			continue
		}
		next := entries[i+1].text
		// Reject the obvious case where the "next" line is itself another
		// AA name (one or two short words, no sentence punctuation).
		if !looksLikeDescription(next) {
			continue
		}
		if len(next) > len(best) {
			best = next
		}
	}
	return best
}

// normalizeName lower-cases and strips apostrophes, slashes, and any
// "/secondary" qualifier so naming variants between altadv_vars and
// eqstr_en.txt collapse to the same key.
func normalizeName(s string) string {
	s = strings.ToLower(s)
	// Drop alternate names after a slash: "Fletching/Bowyer Mastery" → "Fletching Mastery".
	if i := strings.IndexByte(s, '/'); i >= 0 {
		// Replace `/<word>` with empty. The chunk runs to the next space.
		end := strings.IndexByte(s[i:], ' ')
		if end < 0 {
			s = s[:i]
		} else {
			s = s[:i] + s[i+end:]
		}
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch r {
		case '\'', '`', '’', ' ', '\t', '-':
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func looksLikeDescription(s string) bool {
	if len(s) < 20 {
		return false
	}
	// Real descriptions are sentences — they contain at least one period or
	// have multiple words. A short proper-noun line like "Dire Charm" fails
	// both checks.
	return strings.ContainsAny(s, ".!?") || strings.Count(s, " ") >= 4
}

// lookupByName returns the eqmacid of an already-resolved AA matching the
// given name, or 0 if none yet recorded.
func lookupByName(name string, aas []aaRow, descs map[int]string) int {
	for _, aa := range aas {
		if aa.name == name {
			if _, ok := descs[aa.eqmacid]; ok {
				return aa.eqmacid
			}
		}
	}
	return 0
}

func writeJSON(path string, descs map[int]string) error {
	// Stringify keys so the JSON is portable and diff-friendly. Sort by
	// numeric eqmacid for stable output across runs.
	keys := make([]int, 0, len(descs))
	for k := range descs {
		keys = append(keys, k)
	}
	sort.Ints(keys)

	out := make(map[string]string, len(descs))
	for _, k := range keys {
		out[strconv.Itoa(k)] = descs[k]
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
