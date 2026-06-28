// Package popflag is the curated Planes of Power flag dataset plus the
// per-character progression store and resolver.
//
// The dataset is the single source of truth for PoP flagging. It is embedded
// from flags.json (ships locally with the app, like quarm.db / pop_gated.json)
// and served to the frontend over the local API — there is no mirrored TS data
// file to drift.
//
// Authoring sources (under testdata/pop-flags/): the prereq DAG comes from the
// community flag bot's popFlag dict (WFH_Magelo.py "Requirement" arrays — pure
// AND-lists, no either/or); qglobal values, bitmask sub-steps, and the Seer
// text come from the TAKP Seer Lua (seer_script.txt). The only genuine "any-of"
// in PoP flagging is the Seer Lua's replacement semantics — cipher substitutes
// for mmarr+saryrn, zebuxoruk for mmarr_book+karana — which is modelled at the
// node-completion level via SatisfiedBy, not as prereq OR-groups.
package popflag

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
)

//go:embed flags.json
var flagsJSON []byte

// EventRule is a live-log signal (Phase 3) that optimistically marks a flag
// complete as an 'auto'-sourced row. Kind is "kill" | "zone" | "say" | "loot";
// Match is the entity name (mob, zone, …), compared case- and article-
// insensitively. Only authored on nodes where the event is itself the
// milestone — never on a node that strictly requires a post-kill hail, so a
// bare kill can't produce a false positive.
type EventRule struct {
	Kind  string `json:"kind"`
	Match string `json:"match"`
}

// QualifyCond is an alternate qglobal condition that also marks a flag
// complete. Used for the cipher/zebuxoruk replacement any-of: a node backed by
// saryrn/mmarr is also satisfied by cipher==1, and a node backed by karana is
// also satisfied by zebuxoruk>=1 (the server deletes the original qglobal once
// the replacement is granted). Evaluated against a Seer qglobal snapshot in
// Phase 2; authored here so the dataset is correct from the start.
type QualifyCond struct {
	Qglobal string `json:"qglobal"`
	Value   string `json:"value"`
}

// PoPFlag is one discrete progression flag (a node in the dependency DAG).
type PoPFlag struct {
	ID        string   `json:"id"`              // stable slug, e.g. "poj_mavuin_return"
	Tier      int      `json:"tier"`            // 1..4, 5 = Plane of Time
	Zone      string   `json:"zone"`            // "Plane of Justice"
	ZoneShort string   `json:"zone_short"`      // "PoJ"
	Label     string   `json:"label"`           // short checklist label
	Detail    string   `json:"detail"`          // step instructions
	Prereqs   []string `json:"prereqs"`         // AND-list of flag IDs that gate this one
	Level     int      `json:"level,omitempty"` // level at which the chain can be skipped to (display)

	// Completion detection — consumed by the Phase 2 Seer parser; authored now.
	Qglobal      string        `json:"qglobal,omitempty"`       // backing qglobal name
	QglobalValue string        `json:"qglobal_value,omitempty"` // value marking THIS node complete
	Counter      bool          `json:"counter,omitempty"`       // done when current value >= QglobalValue
	BitPosition  int           `json:"bit_position,omitempty"`  // 1-based bitmask sub-step (hohtrials/sol_room)
	SatisfiedBy  []QualifyCond `json:"satisfied_by,omitempty"`  // any-of replacement conditions
	SeerPhrases  []string      `json:"seer_phrases,omitempty"`  // Phase 2: Seer guided-meditation substrings
	Events       []EventRule   `json:"events,omitempty"`        // Phase 3: live-log auto-detection signals
}

var (
	flagsOnce sync.Once
	flags     []PoPFlag
	byID      map[string]PoPFlag
)

func load() {
	flagsOnce.Do(func() {
		if err := json.Unmarshal(flagsJSON, &flags); err != nil {
			// flags.json is embedded at build time, so a parse failure is a
			// programming error, not a runtime condition.
			panic(fmt.Sprintf("popflag: parse flags.json: %v", err))
		}
		byID = make(map[string]PoPFlag, len(flags))
		for _, f := range flags {
			byID[f.ID] = f
		}
	})
}

// Flags returns all curated PoP flags in dataset order.
func Flags() []PoPFlag {
	load()
	return flags
}

// ByID returns the flag with the given ID and whether it exists.
func ByID(id string) (PoPFlag, bool) {
	load()
	f, ok := byID[id]
	return f, ok
}

// MatchEvent returns the IDs of flags whose EventRules match a live event of
// the given kind and entity name (e.g. kind="kill", name="Terris Thule").
// Matching is case- and leading-article-insensitive so "a construct" and
// "Construct" both resolve. Returns an empty slice when nothing matches.
func MatchEvent(kind, name string) []string {
	load()
	want := normName(name)
	if want == "" {
		return nil
	}
	out := []string{}
	for _, f := range flags {
		for _, r := range f.Events {
			if r.Kind == kind && normName(r.Match) == want {
				out = append(out, f.ID)
				break
			}
		}
	}
	return out
}

// normName lowercases and strips a leading English article for name matching.
func normName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	for _, art := range []string{"a ", "an ", "the "} {
		if strings.HasPrefix(s, art) {
			return s[len(art):]
		}
	}
	return s
}
