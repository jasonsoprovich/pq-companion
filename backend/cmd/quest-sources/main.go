// quest-sources parses the Project Quarm quest scripts (Lua/Perl, one file per
// NPC, grouped into per-zone directories) and emits a compact JSON list of the
// item facts each quest exposes: which item IDs an NPC *rewards* and which it
// requires as *turn-ins*, tagged with the NPC's zone.
//
// Why this exists: quarm.db records where items DROP / are SOLD / foraged, but
// EQEmu implements quests in Lua/Perl scripts, NOT database tables — so a
// quest-reward item (e.g. the Sigil Earring of Veracity, 29861) has no DB
// source row and looks "unobtainable." The gear upgrade finder needs the quest
// source both to keep current-era quest gear visible AND to correctly hide
// PoP-era quest rewards (e.g. Jade Hoop of Speed, 32106, rewarded in Plane of
// Knowledge). The item page's Quests tab renders the same facts.
//
// We emit ONLY facts (item id, zone short-name, NPC name, role). We never copy
// the quest dialogue or the script source, so the GPLv3 quest scripts are a
// build-time input, not redistributed. The output JSON is checked in and
// embedded into the binary (see internal/db/quest_sources.go), so it survives
// every quarm.db regeneration from the upstream MySQL dump.
//
// Run after cloning the quest scripts repo, pinned to the commit that matches
// the quarm.db data release:
//
//	git clone https://github.com/SecretsOTheP/quests
//	go run ./cmd/quest-sources \
//	    -quests ./quests \
//	    -out backend/internal/db/quest_sources.json
package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// questEntry is one NPC's quest item activity. Rewards are items the NPC gives
// out (quest rewards + cursor-summoned items); TurnIns are items it requires.
type questEntry struct {
	Zone    string `json:"zone"`
	NPC     string `json:"npc"`
	Rewards []int  `json:"rewards,omitempty"`
	TurnIns []int  `json:"turnins,omitempty"`
}

var (
	// itemid = N  and  itemid1..4 = N  inside a QuestReward table.
	reTableItemID = regexp.MustCompile(`itemid[1-4]?\s*=\s*(\d+)`)
	// items = { a, b, c } reward list.
	reItemsList = regexp.MustCompile(`items\s*=\s*\{([0-9,\s]+)\}`)
	// item1 = N inside a check_turn_in table.
	reTurnInItem = regexp.MustCompile(`item\d*\s*=\s*(\d+)`)
	// A bare integer (used to read a positional QuestReward arg).
	reInt = regexp.MustCompile(`^\d+$`)
)

func main() {
	questsDir := flag.String("quests", "", "path to a clone of the quest scripts repo")
	outPath := flag.String("out", "backend/internal/db/quest_sources.json", "output JSON path")
	flag.Parse()

	if *questsDir == "" {
		log.Fatal("-quests is required (path to a clone of the quest scripts repo)")
	}

	entries, stats := parseTree(*questsDir)
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Zone != entries[j].Zone {
			return entries[i].Zone < entries[j].Zone
		}
		return entries[i].NPC < entries[j].NPC
	})

	buf, err := json.MarshalIndent(entries, "", " ")
	if err != nil {
		log.Fatalf("marshal: %v", err)
	}
	buf = append(buf, '\n')
	if err := os.WriteFile(*outPath, buf, 0o644); err != nil {
		log.Fatalf("write %s: %v", *outPath, err)
	}
	log.Printf("wrote %d quest entries across %d zones (%d reward refs, %d turn-in refs) to %s",
		len(entries), stats.zones, stats.rewards, stats.turnins, *outPath)
}

type parseStats struct{ zones, rewards, turnins int }

func parseTree(root string) ([]questEntry, parseStats) {
	var entries []questEntry
	var stats parseStats
	zoneSeen := map[string]bool{}

	_ = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".lua" && ext != ".pl" {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		parts := strings.Split(rel, string(filepath.Separator))
		if len(parts) < 2 {
			return nil // not <zone>/<npc> — skip global/util scripts
		}
		zone := parts[0]
		// Skip non-zone helper dirs that ship alongside the per-zone scripts.
		switch zone {
		case "global", "lua_modules", "plugins", "bots":
			return nil
		}
		npc := strings.ReplaceAll(strings.TrimSuffix(parts[len(parts)-1], filepath.Ext(path)), "_", " ")

		src, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		rewards, turnins := parseScript(string(src))
		if len(rewards) == 0 && len(turnins) == 0 {
			return nil
		}
		zoneSeen[zone] = true
		stats.rewards += len(rewards)
		stats.turnins += len(turnins)
		entries = append(entries, questEntry{
			Zone: zone, NPC: npc,
			Rewards: rewards, TurnIns: turnins,
		})
		return nil
	})
	stats.zones = len(zoneSeen)
	return entries, stats
}

// parseScript extracts reward and turn-in item IDs from one quest script. It
// uses balanced-paren extraction (not line regexes) so multi-line calls and
// nested parens like math.random(0,5) don't derail the arg parsing.
func parseScript(src string) (rewards, turnins []int) {
	rewardSet := map[int]bool{}
	turnInSet := map[int]bool{}

	for _, args := range callArgs(src, "QuestReward") {
		for _, id := range rewardItemsFromQuestReward(args) {
			rewardSet[id] = true
		}
	}
	// SummonItem / SummonCursorItem hand the player an item directly.
	for _, fn := range []string{"SummonItem", "SummonCursorItem"} {
		for _, args := range callArgs(src, fn) {
			if id := firstInt(args); id > 0 {
				rewardSet[id] = true
			}
		}
	}
	for _, args := range callArgs(src, "check_turn_in") {
		for _, m := range reTurnInItem.FindAllStringSubmatch(args, -1) {
			if id, _ := strconv.Atoi(m[1]); id > 0 {
				turnInSet[id] = true
			}
		}
	}
	return sortedKeys(rewardSet), sortedKeys(turnInSet)
}

// rewardItemsFromQuestReward pulls item IDs out of one QuestReward(...) arg
// string. Two shapes occur:
//
//	table:      QuestReward(e.self, { itemid = N, exp = ... })   // also items={..}, itemid1..4
//	positional: QuestReward(e.self, copper, silver, gold, plat, ITEMID, exp)
//
// A non-integer itemid (a Lua variable like `ranitem`, or `25 * ONE_THOUSAND`)
// can't be resolved statically, so it's skipped.
func rewardItemsFromQuestReward(args string) []int {
	var out []int
	if strings.Contains(args, "{") {
		for _, m := range reTableItemID.FindAllStringSubmatch(args, -1) {
			if id, _ := strconv.Atoi(m[1]); id > 0 {
				out = append(out, id)
			}
		}
		if m := reItemsList.FindStringSubmatch(args); m != nil {
			for _, n := range strings.FieldsFunc(m[1], func(r rune) bool { return r == ',' || r == ' ' }) {
				if id, _ := strconv.Atoi(strings.TrimSpace(n)); id > 0 {
					out = append(out, id)
				}
			}
		}
		return out
	}
	// Positional: args = [e.self, copper, silver, gold, plat, itemid, exp...].
	// The reward item is the 6th top-level argument (index 5).
	fields := splitTopLevel(args)
	if len(fields) >= 6 {
		if id, err := strconv.Atoi(strings.TrimSpace(fields[5])); err == nil && id > 0 {
			out = append(out, id)
		}
	}
	return out
}

// callArgs returns the argument string of every `name(...)` call in src, using
// balanced-paren matching so nested parens are handled.
func callArgs(src, name string) []string {
	var out []string
	needle := name + "("
	for i := 0; i+len(needle) <= len(src); {
		idx := strings.Index(src[i:], needle)
		if idx < 0 {
			break
		}
		// Require a non-identifier char before the name so QuestReward doesn't
		// also match e.g. MyQuestReward.
		start := i + idx
		if start > 0 && isIdentChar(src[start-1]) {
			i = start + len(needle)
			continue
		}
		open := start + len(needle) - 1 // index of '('
		depth := 0
		end := -1
		for j := open; j < len(src); j++ {
			switch src[j] {
			case '(':
				depth++
			case ')':
				depth--
				if depth == 0 {
					end = j
				}
			}
			if end >= 0 {
				break
			}
		}
		if end < 0 {
			break
		}
		out = append(out, src[open+1:end])
		i = end + 1
	}
	return out
}

// splitTopLevel splits a comma-separated arg list, ignoring commas nested in
// (), {}, or [].
func splitTopLevel(s string) []string {
	var fields []string
	depth := 0
	last := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(', '{', '[':
			depth++
		case ')', '}', ']':
			depth--
		case ',':
			if depth == 0 {
				fields = append(fields, s[last:i])
				last = i + 1
			}
		}
	}
	fields = append(fields, s[last:])
	return fields
}

func firstInt(args string) int {
	f := splitTopLevel(args)
	if len(f) == 0 {
		return 0
	}
	if v := strings.TrimSpace(f[0]); reInt.MatchString(v) {
		n, _ := strconv.Atoi(v)
		return n
	}
	return 0
}

func isIdentChar(b byte) bool {
	return b == '_' || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

func sortedKeys(m map[int]bool) []int {
	if len(m) == 0 {
		return nil
	}
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}
