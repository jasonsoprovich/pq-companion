package popflag

import "testing"

// TestDatasetIntegrity guards the curated flags.json: unique IDs, in-range
// tiers, no dangling prereq edges, and an acyclic dependency graph. This is the
// Phase 1 safety net in lieu of a Go↔TS sync test (the dataset is served to the
// frontend over the API, so there is no second copy to drift).
func TestDatasetIntegrity(t *testing.T) {
	all := Flags()
	if len(all) == 0 {
		t.Fatal("dataset is empty")
	}

	seen := map[string]bool{}
	for _, f := range all {
		if f.ID == "" {
			t.Errorf("flag with empty ID: %+v", f)
		}
		if seen[f.ID] {
			t.Errorf("duplicate flag ID %q", f.ID)
		}
		seen[f.ID] = true

		if f.Tier < 1 || f.Tier > 5 {
			t.Errorf("flag %q has out-of-range tier %d", f.ID, f.Tier)
		}
		if f.Label == "" || f.Zone == "" || f.ZoneShort == "" {
			t.Errorf("flag %q missing label/zone/zone_short", f.ID)
		}
	}

	// No dangling prereq edges.
	for _, f := range all {
		for _, p := range f.Prereqs {
			if !seen[p] {
				t.Errorf("flag %q references unknown prereq %q", f.ID, p)
			}
			if p == f.ID {
				t.Errorf("flag %q lists itself as a prereq", f.ID)
			}
		}
	}

	if cyc := findCycle(all); cyc != "" {
		t.Errorf("prereq graph has a cycle involving %s", cyc)
	}
}

// findCycle returns a node ID on a cycle, or "" if the graph is acyclic.
func findCycle(all []PoPFlag) string {
	prereqs := make(map[string][]string, len(all))
	for _, f := range all {
		prereqs[f.ID] = f.Prereqs
	}
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := make(map[string]int, len(all))
	var visit func(id string) string
	visit = func(id string) string {
		color[id] = gray
		for _, p := range prereqs[id] {
			switch color[p] {
			case gray:
				return p
			case white:
				if c := visit(p); c != "" {
					return c
				}
			}
		}
		color[id] = black
		return ""
	}
	for _, f := range all {
		if color[f.ID] == white {
			if c := visit(f.ID); c != "" {
				return c
			}
		}
	}
	return ""
}

// TestStepKind guards the per-node action classification that drives the UI's
// icon/colour coding: every node must carry one of the four known kinds.
func TestStepKind(t *testing.T) {
	valid := map[string]bool{"kill": true, "timed_hail": true, "hail": true, "loot": true}
	for _, f := range Flags() {
		if !valid[f.StepKind] {
			t.Errorf("flag %q has missing/unknown step_kind %q", f.ID, f.StepKind)
		}
	}
}

// TestReplacementAnyOf locks in the only genuine either/or in PoP flagging: the
// Seer Lua deletes mmarr+saryrn when cipher is granted, and mmarr_book+karana
// when zebuxoruk is granted. Nodes backed by those qglobals must therefore also
// be satisfiable by the replacement qglobal.
func TestReplacementAnyOf(t *testing.T) {
	wantSatisfier := map[string]string{ // backing qglobal -> required replacement qglobal
		"saryrn": "cipher",
		"mmarr":  "cipher",
		"karana": "zebuxoruk",
	}
	for _, f := range Flags() {
		repl, needs := wantSatisfier[f.Qglobal]
		if !needs {
			continue
		}
		found := false
		for _, c := range f.SatisfiedBy {
			if c.Qglobal == repl {
				found = true
			}
		}
		if !found {
			t.Errorf("flag %q (qglobal %q) must be satisfied_by %q (server deletes %q on replacement)",
				f.ID, f.Qglobal, repl, f.Qglobal)
		}
	}
}

// TestBitmaskPositions checks the only two bitmask qglobals have valid 1-based
// positions (hohtrials is 3 wide, sol_room is 5 wide).
func TestBitmaskPositions(t *testing.T) {
	width := map[string]int{"hohtrials": 3, "sol_room": 5}
	for _, f := range Flags() {
		if f.BitPosition == 0 {
			continue
		}
		w, ok := width[f.Qglobal]
		if !ok {
			t.Errorf("flag %q sets bit_position on non-bitmask qglobal %q", f.ID, f.Qglobal)
			continue
		}
		if f.BitPosition < 1 || f.BitPosition > w {
			t.Errorf("flag %q bit_position %d out of range 1..%d for %q", f.ID, f.BitPosition, w, f.Qglobal)
		}
	}
}
