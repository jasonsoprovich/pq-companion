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
	// Total counts required milestones only — optional rows (keys, keyrings,
	// bonus content) and any-of members are excluded.
	wantTotal := 0
	for _, f := range Flags() {
		if f.Optional || f.Group != "" {
			continue
		}
		wantTotal++
	}
	if r.Total != wantTotal {
		t.Errorf("overall total = %d, want %d (counted milestones)", r.Total, wantTotal)
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

// TestResolveAnyOfGroup verifies the PoJ Trials any-of: completing ONE member
// rolls up to the anchor (poj_trial_mark), supersedes the unchosen members, and
// unlocks the downstream step — while the members themselves never count toward
// the tally.
func TestResolveAnyOfGroup(t *testing.T) {
	states := []State{
		{FlagID: "poj_preflag", Done: true, Source: SourceManual},
		{FlagID: "poj_trial_stone", Done: true, Source: SourceManual},
	}
	by := statusByID(Resolve(states))

	// Anchor effectively done via the member, carrying the member's source.
	anchor := by["poj_trial_mark"]
	if !anchor.Done {
		t.Errorf("poj_trial_mark anchor should be done via a member")
	}
	if anchor.Source != SourceManual {
		t.Errorf("anchor source = %q, want manual (inherited from member)", anchor.Source)
	}
	// The chosen member is done and NOT superseded.
	if chosen := by["poj_trial_stone"]; !chosen.Done || chosen.Superseded {
		t.Errorf("chosen member should be done and not superseded: %+v", chosen)
	}
	// An unchosen member is superseded (faded "not needed").
	if other := by["poj_trial_flame"]; other.Done || !other.Superseded {
		t.Errorf("unchosen member should be superseded and not done: %+v", other)
	}
	// Downstream step unlocked by the anchor roll-up.
	if by["poj_mavuin_return"].Locked {
		t.Errorf("poj_mavuin_return should be unlocked once a Trial is done")
	}

	// Members never count: PoJ counted total is the three milestones only.
	r := Resolve(states)
	var poj *Progress
	for i := range r.Zones {
		if r.Zones[i].Key == "Plane of Justice" {
			poj = &r.Zones[i]
		}
	}
	if poj == nil {
		t.Fatal("Plane of Justice zone tally missing")
	}
	if poj.Total != 3 {
		t.Errorf("PoJ counted total = %d, want 3 (members + optional excluded)", poj.Total)
	}
	if poj.Done != 2 { // preflag + anchor(via member)
		t.Errorf("PoJ done = %d, want 2", poj.Done)
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

// TestResolveBonusRows covers the "bonus" role (Sol Ro minis, Carpryn): left
// out of every tally until done, then counted in done AND total, while other
// optional rows stay excluded either way.
func TestResolveBonusRows(t *testing.T) {
	progress := func(r Resolved) (all, tier3, solro Progress) {
		all = Progress{Done: r.Done, Total: r.Total}
		for _, p := range r.Tiers {
			if p.Tier == 3 {
				tier3 = p
			}
		}
		for _, p := range r.Zones {
			if p.Key == "Tower of Solusek Ro" {
				solro = p
			}
		}
		return
	}

	base := []State{{FlagID: "solro_miak", Done: true, Source: SourceManual}}
	a0, t0, z0 := progress(Resolve(base))

	// Unfinished minis are not in any total.
	required := 0
	for _, f := range Flags() {
		if f.Zone == "Tower of Solusek Ro" && !f.Optional && f.Group == "" {
			required++
		}
	}
	if z0.Total != required {
		t.Errorf("Sol Ro zone total = %d, want %d (minis excluded until done)", z0.Total, required)
	}

	// Two minis done: +2 to done and total at every level.
	withMinis := append(append([]State{}, base...),
		State{FlagID: "solro_xuzl", Done: true, Source: SourceManual},
		State{FlagID: "solro_jiva", Done: true, Source: SourceManual},
	)
	a1, t1, z1 := progress(Resolve(withMinis))
	for _, c := range []struct {
		name       string
		before, af Progress
	}{{"overall", a0, a1}, {"tier 3", t0, t1}, {"Sol Ro zone", z0, z1}} {
		if c.af.Done != c.before.Done+2 || c.af.Total != c.before.Total+2 {
			t.Errorf("%s: %d/%d -> %d/%d, want +2/+2", c.name, c.before.Done, c.before.Total, c.af.Done, c.af.Total)
		}
	}

	// A done ordinary optional row (the Plane of Fire key-ring) stays out.
	withKeyring := append(append([]State{}, base...),
		State{FlagID: "solro_pofire_keyring", Done: true, Source: SourceManual})
	a2, _, _ := progress(Resolve(withKeyring))
	if a2.Done != a0.Done || a2.Total != a0.Total {
		t.Errorf("keyring changed the tally: %d/%d -> %d/%d", a0.Done, a0.Total, a2.Done, a2.Total)
	}

	// An explicitly unchecked bonus row is not counted.
	unchecked := append(append([]State{}, base...),
		State{FlagID: "solro_xuzl", Done: false, Source: SourceManual})
	a3, _, _ := progress(Resolve(unchecked))
	if a3.Done != a0.Done || a3.Total != a0.Total {
		t.Errorf("unchecked bonus changed the tally: %d/%d -> %d/%d", a0.Done, a0.Total, a3.Done, a3.Total)
	}
}
