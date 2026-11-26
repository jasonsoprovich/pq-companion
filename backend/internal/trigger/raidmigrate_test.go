package trigger

import (
	"strings"
	"testing"
	"time"
)

// TestMigrateRaidSignatureRows covers the installed-user migration: rows
// from the shipped Raid Alerts signature triggers get BossCasters, matched
// by SourcePack + PackKey (so renamed rows count) with a pattern fallback
// for legacy rows; user customizations survive; unrelated rows are untouched.
func TestMigrateRaidSignatureRows(t *testing.T) {
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
	renamed.BossCasters, renamed.DedupKey = nil, ""
	renamed.SourcePack, renamed.PackKey, renamed.PackName = raidAlertsPackName, "Fling", raidAlertsPackName
	renamed.BarColor = "#00ff00"

	// Legacy row: no SourcePack/PackKey, renamed, matched by pattern.
	legacy := find("Torturing Winds")
	legacy.ID, legacy.Name = "r2", "Seru winds"
	legacy.BossCasters, legacy.DedupKey = nil, ""
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
	if r1.DedupKey != "raid_sig_fling" {
		t.Errorf("renamed row DedupKey = %q, want raid_sig_fling", r1.DedupKey)
	}
	r2 := got("r2")
	if len(r2.BossCasters) != 1 || r2.BossCasters[0].Name != "Lord Inquisitor Seru" {
		t.Errorf("legacy row casters = %+v, want Lord Inquisitor Seru", r2.BossCasters)
	}
	if r2.SourcePack != raidAlertsPackName || r2.PackKey != "Torturing Winds" || r2.DedupKey != "raid_sig_torturing_winds" {
		t.Errorf("legacy row identity = %q/%q/%q", r2.SourcePack, r2.PackKey, r2.DedupKey)
	}
	if r3 := got("r3"); r3.DedupKey != "" {
		t.Errorf("unrelated row got DedupKey %q", r3.DedupKey)
	}
	// Both rows now have a Raid Alerts baseline, so the update lists them.
	base, err := s.PackBaselines(raidAlertsPackName)
	if err != nil {
		t.Fatal(err)
	}
	if base["Fling"] == nil || base["Torturing Winds"] == nil {
		t.Errorf("missing Raid Alerts baselines: %v", base)
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

// oldRaidAlertsInstall recreates a pre-move install: Raid Alerts with its
// five signature rows as the old build wrote them (no DedupKey, no
// BossCasters), Fling customized, and only some rows baselined (the old
// one-off inserts never wrote one). Then the startup migration runs.
func oldRaidAlertsInstall(t *testing.T) *Store {
	t.Helper()
	s := openTestStore(t)
	if err := InstallPack(s, RaidAlertsPack()); err != nil {
		t.Fatal(err)
	}
	for i, d := range raidSignatureSpellAlerts() {
		row := d
		row.ID = "sig" + string(rune('a'+i))
		row.CreatedAt = time.Now().UTC()
		row.DedupKey, row.BossCasters = "", nil
		row.SourcePack, row.PackKey, row.PackName = raidAlertsPackName, d.Name, raidAlertsPackName
		if d.Name == "Fling" {
			row.Name, row.Enabled = "Fling (mine)", false
			row.Characters = []string{"Tankname"}
			row.BarColor = "#00ff00"
			row.Actions = []Action{{Type: ActionTextToSpeech, Text: "fling!", Volume: 1}}
		}
		if err := s.Insert(&row); err != nil {
			t.Fatal(err)
		}
		if i%2 == 0 {
			old := d
			old.DedupKey, old.BossCasters, old.PackName = "", nil, raidAlertsPackName
			if err := s.UpsertPackBaseline(raidAlertsPackName, &old); err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := ApplyDefaultUpdates(s, DefaultUpdates()); err != nil {
		t.Fatal(err)
	}
	return s
}

func builtinPack(t *testing.T, name string) TriggerPack {
	t.Helper()
	for _, p := range AllPacks() {
		if p.PackName == name {
			return p
		}
	}
	t.Fatalf("no built-in pack %q", name)
	return TriggerPack{}
}

// sigRows returns the installed rows carrying each signature DedupKey.
func sigRows(t *testing.T, s *Store) map[string][]*Trigger {
	t.Helper()
	all, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]*Trigger{}
	for _, d := range raidSignatureSpellAlerts() {
		out[d.DedupKey] = nil
	}
	for _, tr := range all {
		if _, ok := out[tr.DedupKey]; ok {
			out[tr.DedupKey] = append(out[tr.DedupKey], tr)
		}
	}
	return out
}

func applyAll(t *testing.T, s *Store, p TriggerPack) *PackUpdateResult {
	t.Helper()
	res, err := ApplyPackUpdate(s, p, p, UpdateModePreserve, nil)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// TestRaidSignatureMove_InstallThenUpdate: install Raid: Luclin, then apply
// the Raid Alerts update. Luclin's install skips the five (Raid Alerts owns
// them); the update moves each row, customizations intact, into Luclin.
func TestRaidSignatureMove_InstallThenUpdate(t *testing.T) {
	s := oldRaidAlertsInstall(t)
	luclin := builtinPack(t, "Raid: Luclin")
	if err := InstallPack(s, builtinPack(t, "Raid: Luclin")); err != nil {
		t.Fatal(err)
	}
	for k, rows := range sigRows(t, s) {
		if len(rows) != 1 || rows[0].SourcePack != raidAlertsPackName {
			t.Fatalf("%s after Luclin install: %d rows, want the 1 Raid Alerts row", k, len(rows))
		}
	}

	diff, err := ComputePackDiff(s, RaidAlertsPack())
	if err != nil {
		t.Fatal(err)
	}
	if len(diff.Removed) != 5 {
		t.Fatalf("Raid Alerts removed = %+v, want the 5 signature rows", diff.Removed)
	}
	for _, r := range diff.Removed {
		if r.MovedTo != "Raid: Luclin" || !r.Rehome {
			t.Errorf("%s: moved_to %q rehome %v, want Raid: Luclin / true", r.Name, r.MovedTo, r.Rehome)
		}
	}

	res := applyAll(t, s, RaidAlertsPack())
	if res.Moved != 5 || res.Removed != 0 {
		t.Fatalf("update result %+v, want 5 moved", res)
	}
	for k, rows := range sigRows(t, s) {
		if len(rows) != 1 {
			t.Fatalf("%s: %d rows after move, want 1", k, len(rows))
		}
		r := rows[0]
		if r.SourcePack != "Raid: Luclin" || r.PackName != "Raid: Luclin" {
			t.Errorf("%s: source %q category %q, want Raid: Luclin", k, r.SourcePack, r.PackName)
		}
		if k == "raid_sig_fling" {
			if r.Name != "Fling (mine)" || r.Enabled || r.BarColor != "#00ff00" ||
				len(r.Characters) != 1 || len(r.Actions) != 1 || r.Actions[0].Text != "fling!" {
				t.Errorf("Fling customizations lost: %+v", r)
			}
		}
	}
	// The receiving pack now has nothing pending for them, and its later
	// diffs treat the user's edits as customizations.
	ld, err := ComputePackDiff(s, luclin)
	if err != nil {
		t.Fatal(err)
	}
	if ld.HasUpdates() {
		t.Errorf("Raid: Luclin has pending updates after the move: %+v", ld)
	}
	if rd, _ := ComputePackDiff(s, RaidAlertsPack()); rd.HasUpdates() {
		t.Errorf("Raid Alerts still has updates: %+v", rd)
	}
	// Uninstalling Luclin takes them with it.
	if err := UninstallPack(s, "Raid: Luclin", map[string]bool{raidAlertsPackName: true}); err != nil {
		t.Fatal(err)
	}
	for k, rows := range sigRows(t, s) {
		if len(rows) != 0 {
			t.Errorf("%s: %d rows after uninstalling Luclin, want 0", k, len(rows))
		}
	}
}

// TestRaidSignatureMove_UpdateThenInstall: apply the Raid Alerts update with
// no raid pack installed (the rows go), then install Luclin: one fresh copy.
func TestRaidSignatureMove_UpdateThenInstall(t *testing.T) {
	s := oldRaidAlertsInstall(t)
	diff, err := ComputePackDiff(s, RaidAlertsPack())
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range diff.Removed {
		if r.MovedTo != "Raid: Luclin" || r.Rehome {
			t.Errorf("%s: moved_to %q rehome %v, want Raid: Luclin / false", r.Name, r.MovedTo, r.Rehome)
		}
	}
	if res := applyAll(t, s, RaidAlertsPack()); res.Removed != 5 || res.Moved != 0 {
		t.Fatalf("update result %+v, want 5 removed", res)
	}
	for k, rows := range sigRows(t, s) {
		if len(rows) != 0 {
			t.Fatalf("%s: %d rows after update, want 0", k, len(rows))
		}
	}
	if err := InstallPack(s, builtinPack(t, "Raid: Luclin")); err != nil {
		t.Fatal(err)
	}
	for k, rows := range sigRows(t, s) {
		if len(rows) != 1 || rows[0].SourcePack != "Raid: Luclin" {
			t.Errorf("%s: %d rows after Luclin install, want 1 from Luclin", k, len(rows))
		}
	}
}

// TestRaidSignatureMove_UninstallRaidAlertsPromotes: with Raid Alerts still
// owning the five, uninstalling it promotes Luclin's copies.
func TestRaidSignatureMove_UninstallRaidAlertsPromotes(t *testing.T) {
	s := oldRaidAlertsInstall(t)
	if err := InstallPack(s, builtinPack(t, "Raid: Luclin")); err != nil {
		t.Fatal(err)
	}
	if err := UninstallPack(s, raidAlertsPackName, map[string]bool{"Raid: Luclin": true}); err != nil {
		t.Fatal(err)
	}
	for k, rows := range sigRows(t, s) {
		if len(rows) != 1 || rows[0].SourcePack != "Raid: Luclin" {
			t.Errorf("%s: %d rows after uninstalling Raid Alerts, want 1 from Luclin", k, len(rows))
		}
	}
}

// TestRaidAlertsPack_NoSignatureSpells: fresh Raid Alerts installs no longer
// get the five.
func TestRaidAlertsPack_NoSignatureSpells(t *testing.T) {
	for _, tr := range RaidAlertsPack().Triggers {
		if len(tr.BossCasters) > 0 || strings.HasPrefix(tr.DedupKey, "raid_sig_") {
			t.Errorf("Raid Alerts still ships %q", tr.Name)
		}
	}
}
