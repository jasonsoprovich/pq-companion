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

// questEntry is one NPC's quest activity. Rewards/TurnIns are the aggregate
// item ids (used for era classification and search); Dialogue is the ordered
// walkthrough — each branch is something the player does (hail, say a keyword,
// or turn in items) and what the NPC does in response (its text + any items it
// grants). The per-step turn-in→reward pairings used to chain a multi-NPC
// questline are derived from the trade/grant branches.
type questEntry struct {
	Zone     string           `json:"zone"`
	NPC      string           `json:"npc"`
	Rewards  []int            `json:"rewards,omitempty"`
	TurnIns  []int            `json:"turnins,omitempty"`
	Dialogue []dialogueBranch `json:"dialogue,omitempty"`
}

// factionDelta is one faction adjustment a dialogue branch applies, as an
// exact `e.other:Faction(e.self, factionID, delta)` call in the source
// script — the one place quest scripts carry a numeric faction amount at
// all (the in-game log only ever prints direction, never a number).
type factionDelta struct {
	FactionID int `json:"faction_id"`
	Delta     int `json:"delta"`
}

// dialogueBranch is one conditional branch of an NPC's quest handlers: the
// player action that triggers it (Triggers = say keywords, or TurnIn = items
// handed over; both empty = an unconditional/hail response), the NPC's spoken
// Text, any items it Grants, and any faction standing it adjusts.
type dialogueBranch struct {
	Triggers []string       `json:"triggers,omitempty"`
	TurnIn   []int          `json:"turnin,omitempty"`
	Text     string         `json:"text,omitempty"`
	Grants   []int          `json:"grants,omitempty"`
	Factions []factionDelta `json:"factions,omitempty"`
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
	// { 12345, 67890 } item-id list, as passed to count_handed_item.
	reIntBraceList = regexp.MustCompile(`\{\s*([0-9,\s]+)\s*\}`)
	// local varName = item_lib.count_handed_item(...) — the common style
	// where the turn-in check is hoisted into a local before the if-block
	// that uses it, rather than inlined directly in the condition (as
	// check_turn_in almost always is). Captures varName so the later
	// `if(varName > 0)` guard can be matched back to its item list.
	reHandedItemDecl = regexp.MustCompile(`local\s+(\w+)\s*=\s*(?:item_lib\.)?count_handed_item\(`)
	// if(varName > 0) / if(varName ~= 0) — the guard that gates a
	// hoisted count_handed_item result's branch body.
	reHandedItemGuard = regexp.MustCompile(`if\s*\(\s*(\w+)\s*(?:>\s*0|~=\s*0|>=\s*1)\s*\)`)
	// Matches immediately before a hoisted count_handed_item call to
	// recover the local variable name it's assigned to.
	reDeclSuffix = regexp.MustCompile(`local\s+(\w+)\s*=\s*(?:item_lib\.)?$`)
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
	log.Printf("wrote %d quest entries across %d zones (%d reward refs, %d turn-in refs, %d faction deltas) to %s",
		len(entries), stats.zones, stats.rewards, stats.turnins, stats.factions, *outPath)
}

type parseStats struct{ zones, rewards, turnins, factions int }

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
		rewards, turnins, dialogue := parseScript(string(src))
		factionCount := 0
		for _, b := range dialogue {
			factionCount += len(b.Factions)
		}
		if len(rewards) == 0 && len(turnins) == 0 && factionCount == 0 {
			return nil
		}
		zoneSeen[zone] = true
		stats.rewards += len(rewards)
		stats.turnins += len(turnins)
		stats.factions += factionCount
		entries = append(entries, questEntry{
			Zone: zone, NPC: npc,
			Rewards: rewards, TurnIns: turnins, Dialogue: dialogue,
		})
		return nil
	})
	stats.zones = len(zoneSeen)
	return entries, stats
}

// dialogue token kinds, ordered by file position to drive the branch builder.
const (
	tokKeyword = iota // a findi("...") condition on a say handler
	tokTurnIn         // a check_turn_in({...}) or count_handed_item({...}) condition
	tokText           // an NPC Say/Emote line
	tokGrant          // an item handed to the player
	tokFaction        // an e.other:Faction(e.self, factionID, delta) call
)

type dlgToken struct {
	pos      int
	kind     int
	keyword  string
	items    []int
	text     string
	factions []factionDelta
}

// reQuoted matches a Lua double-quoted string literal (with \" escapes).
var reQuoted = regexp.MustCompile(`"((?:[^"\\]|\\.)*)"`)

// parseScript extracts an NPC's quest walkthrough from one script: the ordered
// dialogue branches (say keyword / turn-in → NPC text + granted items), plus
// the aggregate reward and turn-in item sets derived from them. Uses balanced-
// paren extraction so multi-line calls and nested parens don't derail parsing.
//
// Branch grouping is positional: a script is a chain of
//
//	if(<cond>) then <body> elseif(<cond>) then <body> ... end
//
// so in file order a condition's tokens (findi / check_turn_in) precede that
// branch's body tokens (Say/Emote text, item grants). A new branch begins at a
// condition token once the current branch's body has started; consecutive
// condition tokens (e.g. `findi("a") or findi("b")`) merge into one branch.
func parseScript(src string) (rewards, turnins []int, dialogue []dialogueBranch) {
	var toks []dlgToken
	for _, c := range callMatches(src, "findi") {
		if s := firstString(c.args); s != "" {
			toks = append(toks, dlgToken{pos: c.pos, kind: tokKeyword, keyword: s})
		}
	}
	for _, c := range callMatches(src, "check_turn_in") {
		var reqs []int
		for _, m := range reTurnInItem.FindAllStringSubmatch(c.args, -1) {
			if id, _ := strconv.Atoi(m[1]); id > 0 {
				reqs = append(reqs, id)
			}
		}
		if len(reqs) > 0 {
			toks = append(toks, dlgToken{pos: c.pos, kind: tokTurnIn, items: reqs})
		}
	}
	// count_handed_item is a second, unrelated turn-in API used for
	// repeatable/any-of hand-ins (e.g. "bring me any number of goblin
	// skins"), distinct from check_turn_in's exact-set form. Two call
	// shapes occur: inline in the if-condition (handled the same way as
	// check_turn_in, directly below) and hoisted into a `local X = ...`
	// assignment ahead of the if-block that tests X (handedVars + the
	// reHandedItemGuard pass below) — the common style, since a script
	// often declares several hand-in checks before testing any of them.
	handedVars := map[string][]int{}
	for _, c := range callMatches(src, "count_handed_item") {
		ids := intBraceList(c.args)
		if len(ids) == 0 {
			continue
		}
		lookback := 64
		if c.pos < lookback {
			lookback = c.pos
		}
		if m := reDeclSuffix.FindStringSubmatch(src[c.pos-lookback : c.pos]); m != nil {
			handedVars[m[1]] = ids
			continue
		}
		toks = append(toks, dlgToken{pos: c.pos, kind: tokTurnIn, items: ids})
	}
	for _, m := range reHandedItemGuard.FindAllStringSubmatchIndex(src, -1) {
		varName := src[m[2]:m[3]]
		if ids, ok := handedVars[varName]; ok {
			toks = append(toks, dlgToken{pos: m[0], kind: tokTurnIn, items: ids})
		}
	}
	for _, c := range callMatches(src, "Faction") {
		fields := splitTopLevel(c.args)
		if len(fields) < 3 {
			continue
		}
		factionID, err1 := strconv.Atoi(strings.TrimSpace(fields[1]))
		delta, err2 := strconv.Atoi(strings.TrimSpace(fields[2]))
		if err1 != nil || err2 != nil {
			continue
		}
		toks = append(toks, dlgToken{pos: c.pos, kind: tokFaction, factions: []factionDelta{{FactionID: factionID, Delta: delta}}})
	}
	for _, fn := range []string{"Say", "Emote", "Shout"} {
		for _, c := range callMatches(src, fn) {
			if t := joinStrings(c.args); t != "" {
				toks = append(toks, dlgToken{pos: c.pos, kind: tokText, text: t})
			}
		}
	}
	for _, c := range callMatches(src, "QuestReward") {
		if ids := rewardItemsFromQuestReward(c.args); len(ids) > 0 {
			toks = append(toks, dlgToken{pos: c.pos, kind: tokGrant, items: ids})
		}
	}
	for _, fn := range []string{"SummonItem", "SummonCursorItem"} {
		for _, c := range callMatches(src, fn) {
			if id := firstInt(c.args); id > 0 {
				toks = append(toks, dlgToken{pos: c.pos, kind: tokGrant, items: []int{id}})
			}
		}
	}
	sort.Slice(toks, func(i, j int) bool { return toks[i].pos < toks[j].pos })

	rewardSet := map[int]bool{}
	turnInSet := map[int]bool{}
	var cur dialogueBranch
	has := false
	bodyStarted := false
	flush := func() {
		if has && !branchEmpty(cur) {
			dialogue = append(dialogue, cur)
		}
		cur = dialogueBranch{}
		has = false
		bodyStarted = false
	}
	for _, t := range toks {
		switch t.kind {
		case tokKeyword, tokTurnIn:
			if has && bodyStarted {
				flush()
			}
			has = true
			if t.kind == tokKeyword {
				cur.Triggers = append(cur.Triggers, t.keyword)
			} else {
				cur.TurnIn = append(cur.TurnIn, t.items...)
				for _, id := range t.items {
					turnInSet[id] = true
				}
			}
		case tokText:
			has = true
			if cur.Text != "" {
				cur.Text += " "
			}
			cur.Text += t.text
			bodyStarted = true
		case tokGrant:
			has = true
			cur.Grants = append(cur.Grants, t.items...)
			for _, id := range t.items {
				rewardSet[id] = true
			}
			bodyStarted = true
		case tokFaction:
			has = true
			cur.Factions = append(cur.Factions, t.factions...)
			bodyStarted = true
		}
	}
	flush()

	return sortedKeys(rewardSet), sortedKeys(turnInSet), dialogue
}

func branchEmpty(b dialogueBranch) bool {
	return len(b.Triggers) == 0 && len(b.TurnIn) == 0 && b.Text == "" && len(b.Grants) == 0 && len(b.Factions) == 0
}

// intBraceList parses a `{ id1, id2, ... }` literal (as passed to
// count_handed_item's item-list argument) into its integer ids.
func intBraceList(args string) []int {
	m := reIntBraceList.FindStringSubmatch(args)
	if m == nil {
		return nil
	}
	var out []int
	for _, n := range strings.FieldsFunc(m[1], func(r rune) bool { return r == ',' || r == ' ' }) {
		if id, _ := strconv.Atoi(strings.TrimSpace(n)); id > 0 {
			out = append(out, id)
		}
	}
	return out
}

// firstString returns the first double-quoted literal in args, unescaped.
func firstString(args string) string {
	if m := reQuoted.FindStringSubmatch(args); m != nil {
		return unescape(m[1])
	}
	return ""
}

// joinStrings concatenates every double-quoted literal in args (Lua `..`
// concatenation with variables like player names is dropped), so a Say built
// from several pieces still yields readable text.
func joinStrings(args string) string {
	var parts []string
	for _, m := range reQuoted.FindAllStringSubmatch(args, -1) {
		if s := strings.TrimSpace(unescape(m[1])); s != "" {
			parts = append(parts, unescape(m[1]))
		}
	}
	return strings.TrimSpace(strings.Join(parts, ""))
}

func unescape(s string) string {
	return strings.NewReplacer(`\"`, `"`, `\n`, " ", `\t`, " ", `\\`, `\`).Replace(s)
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

// callMatch is one `name(...)` call: the byte offset of the name and the raw
// argument string between the outer parens.
type callMatch struct {
	pos  int
	args string
}

// callMatches returns every `name(...)` call in src, using balanced-paren
// matching so nested parens are handled.
func callMatches(src, name string) []callMatch {
	var out []callMatch
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
		out = append(out, callMatch{pos: start, args: src[open+1 : end]})
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
