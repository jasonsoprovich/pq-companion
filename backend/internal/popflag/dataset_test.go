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
	valid := map[string]bool{"kill": true, "timed_hail": true, "hail": true, "loot": true, "zone": true}
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

// TestGroupsAndRoles guards the any-of group and optional/role invariants:
// members reference a real, non-optional anchor; roles are known and imply
// Optional; and no optional row is used as a prereq (optional rows must never
// block another flag).
func TestGroupsAndRoles(t *testing.T) {
	all := Flags()
	byID := map[string]PoPFlag{}
	for _, f := range all {
		byID[f.ID] = f
	}
	validRole := map[string]bool{"key": true, "keyring": true, "optional": true}

	for _, f := range all {
		if f.Group != "" {
			anchor, ok := byID[f.Group]
			if !ok {
				t.Errorf("member %q references unknown group anchor %q", f.ID, f.Group)
			} else if anchor.Group != "" {
				t.Errorf("member %q anchor %q is itself a member (no nested groups)", f.ID, f.Group)
			} else if anchor.Optional {
				t.Errorf("member %q anchor %q must not be optional (the anchor is the counted milestone)", f.ID, f.Group)
			}
		}
		if f.Role != "" {
			if !validRole[f.Role] {
				t.Errorf("flag %q has unknown role %q", f.ID, f.Role)
			}
			if !f.Optional {
				t.Errorf("flag %q has role %q but is not optional (a non-empty role implies optional)", f.ID, f.Role)
			}
		}
	}

	// A required (counted) flag must never depend on an optional row — optional
	// steps must not gate real progress. Optional→optional chains are fine
	// (e.g. the Seventh Hammer bonus fight needs the Mark of Justice).
	for _, f := range all {
		if f.Optional {
			continue
		}
		for _, p := range f.Prereqs {
			if pre, ok := byID[p]; ok && pre.Optional {
				t.Errorf("required flag %q lists optional flag %q as a prereq (optional rows must not block progress)", f.ID, p)
			}
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
