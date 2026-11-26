package trigger

import (
	"testing"
	"time"
)

// TestStampRaidSignatureCasters covers the installed-user migration: rows
// from the shipped Raid Alerts signature triggers get BossCasters, matched
// by SourcePack + PackKey (so renamed rows count) with a pattern fallback
// for legacy rows; user customizations survive; unrelated rows are untouched.
func TestStampRaidSignatureCasters(t *testing.T) {
	s := openTestStore(t)
	defs := raidSignatureSpellAlerts()
	find := func(name string) Trigger {
		for _, d := range defs {
			if d.Name == name {
				return d
			}
		}
		t.Fatalf("no def %q", name)
		return Trigger{}
	}

	// Renamed + disabled + re-colored row, installed with full identity.
	renamed := find("Fling")
	renamed.ID, renamed.Name, renamed.Enabled = "r1", "Fling (mine)", false
	renamed.BossCasters = nil
	renamed.SourcePack, renamed.PackKey, renamed.PackName = raidAlertsPackName, "Fling", raidAlertsPackName
	renamed.BarColor = "#00ff00"

	// Legacy row: no SourcePack/PackKey, renamed, matched by pattern.
	legacy := find("Torturing Winds")
	legacy.ID, legacy.Name = "r2", "Seru winds"
	legacy.BossCasters = nil
	legacy.SourcePack, legacy.PackKey, legacy.PackName = "", "", raidAlertsPackName

	// Same name in an unrelated pack must not be touched.
	other := Trigger{ID: "r3", Name: "Fling", Enabled: true, Pattern: `^foo$`, PackName: "My Stuff", Actions: []Action{}}

	for _, tr := range []*Trigger{&renamed, &legacy, &other} {
		tr.CreatedAt = time.Now().UTC()
		if err := s.Insert(tr); err != nil {
			t.Fatalf("Insert %s: %v", tr.ID, err)
		}
	}

	if _, err := ApplyDefaultUpdates(s, DefaultUpdates()); err != nil {
		t.Fatalf("ApplyDefaultUpdates: %v", err)
	}

	got := func(id string) *Trigger {
		all, err := s.List()
		if err != nil {
			t.Fatal(err)
		}
		for _, tr := range all {
			if tr.ID == id {
				return tr
			}
		}
		t.Fatalf("row %s missing", id)
		return nil
	}
	r1 := got("r1")
	if len(r1.BossCasters) != 4 || r1.BossCasters[0].Name != "Aten Ha Ra" {
		t.Errorf("renamed row casters = %+v, want the 4 Fling casters", r1.BossCasters)
	}
	if r1.Name != "Fling (mine)" || r1.Enabled || r1.BarColor != "#00ff00" {
		t.Errorf("renamed row customizations lost: %+v", r1)
	}
	if r2 := got("r2"); len(r2.BossCasters) != 1 || r2.BossCasters[0].Name != "Lord Inquisitor Seru" {
		t.Errorf("legacy row casters = %+v, want Lord Inquisitor Seru", r2.BossCasters)
	}
	if r3 := got("r3"); len(r3.BossCasters) != 0 {
		t.Errorf("unrelated row got casters: %+v", r3.BossCasters)
	}

	// Runs once: a second pass is a no-op.
	n, err := ApplyDefaultUpdates(s, DefaultUpdates())
	if err != nil || n != 0 {
		t.Errorf("second ApplyDefaultUpdates = %d, %v; want 0, nil", n, err)
	}
}
