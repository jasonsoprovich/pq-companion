package popflag

import "testing"

func statusByID(r Resolved) map[string]FlagStatus {
	m := make(map[string]FlagStatus, len(r.Flags))
	for _, f := range r.Flags {
		m[f.ID] = f
	}
	return m
}

// TestResolveLocking verifies prereq AND-locking: a node is locked until every
// prereq is done, and the missing list names the unmet ones.
func TestResolveLocking(t *testing.T) {
	// Nothing done: every node with prereqs is locked; roots are unlocked.
	r := Resolve(nil)
	by := statusByID(r)

	if by["poj_preflag"].Locked {
		t.Errorf("root poj_preflag should be unlocked with no state")
	}
	if !by["poj_trial_mark"].Locked {
		t.Errorf("poj_trial_mark should be locked until poj_preflag is done")
	}

	// hoh_mithaniel needs all three trials + aerindar. Mark only some done.
	states := []State{
		{FlagID: "pov_aerindar", Done: true, Source: SourceManual},
		{FlagID: "poj_mavuin_return", Done: true, Source: SourceManual},
		{FlagID: "hoh_trial_rydda", Done: true, Source: SourceManual},
		{FlagID: "hoh_trial_villagers", Done: true, Source: SourceManual},
		// hoh_trial_maidens deliberately left undone
	}
	by = statusByID(Resolve(states))
	mith := by["hoh_mithaniel"]
	if !mith.Locked {
		t.Fatalf("hoh_mithaniel should be locked with a trial missing")
	}
	if len(mith.Missing) != 1 || mith.Missing[0] != "hoh_trial_maidens" {
		t.Errorf("expected missing=[hoh_trial_maidens], got %v", mith.Missing)
	}

	// Trials themselves should now be unlocked (aerindar + return done).
	if by["hoh_trial_maidens"].Locked {
		t.Errorf("hoh_trial_maidens should be unlocked once aerindar + return are done")
	}
}

// TestResolveProgress checks the done/total tallies overall, per-tier, and
// per-zone.
func TestResolveProgress(t *testing.T) {
	states := []State{
		{FlagID: "poj_preflag", Done: true, Source: SourceManual},
		{FlagID: "poj_trial_mark", Done: true, Source: SourceManual},
		{FlagID: "ponb_preflag", Done: false, Source: SourceManual}, // explicit not-done
	}
	r := Resolve(states)

	if r.Done != 2 {
		t.Errorf("overall done = %d, want 2", r.Done)
	}
	if r.Total != len(Flags()) {
		t.Errorf("overall total = %d, want %d", r.Total, len(Flags()))
	}

	var poj *Progress
	for i := range r.Zones {
		if r.Zones[i].Key == "Plane of Justice" {
			poj = &r.Zones[i]
		}
	}
	if poj == nil {
		t.Fatal("Plane of Justice zone tally missing")
	}
	if poj.Done != 2 || poj.Total != 3 {
		t.Errorf("PoJ progress = %d/%d, want 2/3", poj.Done, poj.Total)
	}

	if len(r.Tiers) == 0 || r.Tiers[0].Tier != 1 {
		t.Errorf("tiers should be sorted ascending starting at tier 1, got %+v", r.Tiers)
	}
	if last := r.Tiers[len(r.Tiers)-1]; last.Tier != 5 {
		t.Errorf("last tier should be 5 (Plane of Time), got %d", last.Tier)
	}
}

// TestResolveProvenance confirms the source travels through to the status so
// the UI can show the provenance chip.
func TestResolveProvenance(t *testing.T) {
	states := []State{{FlagID: "poj_preflag", Done: true, Source: SourceSeer}}
	by := statusByID(Resolve(states))
	if by["poj_preflag"].Source != SourceSeer {
		t.Errorf("source = %q, want %q", by["poj_preflag"].Source, SourceSeer)
	}
}
