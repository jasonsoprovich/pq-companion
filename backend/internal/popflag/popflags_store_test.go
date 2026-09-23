package popflag

import (
	"testing"
	"time"
)

// TestApplyPopFlagsReportMerges verifies that a second, partial report (e.g.
// syncing '#popflags 2' after '#popflags 1') ADDS to the stored snapshot
// rather than replacing it — Tier 1's derived flags must survive a later
// Tier 2-only report that says nothing about Tier 1's qglobals.
func TestApplyPopFlagsReportMerges(t *testing.T) {
	s := openTempStore(t)
	const char = "Ismedge"

	tier1 := ParsePopFlagsReport([]string{
		"=== Tier 1 Progression ===",
		"Mavuin's case: Mavuin's case is complete",
	})
	if _, err := s.ApplyPopFlagsReport(char, tier1, "raw1", time.Unix(1000, 0)); err != nil {
		t.Fatalf("ApplyPopFlagsReport (tier1): %v", err)
	}
	if !doneByID(t, s, char)["poj_trial_mark"].Done {
		t.Fatalf("poj_trial_mark should be done after mavuin=3 sync")
	}

	tier2 := ParsePopFlagsReport([]string{
		"=== Tier 2 Progression ===",
		"Lower Crypt access: Unlocked",
	})
	if _, err := s.ApplyPopFlagsReport(char, tier2, "raw2", time.Unix(2000, 0)); err != nil {
		t.Fatalf("ApplyPopFlagsReport (tier2): %v", err)
	}
	rows := doneByID(t, s, char)
	if !rows["poj_trial_mark"].Done {
		t.Errorf("poj_trial_mark (from tier1) should still be done after a tier2-only report")
	}
	if !rows["cod_carpryn"].Done {
		t.Errorf("cod_carpryn (bertox_key) should be done from the tier2 report")
	}

	snap, err := s.GetSnapshot(char)
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}
	if snap.Qglobals["mavuin"] != "3" {
		t.Errorf("snapshot should retain mavuin=3 from the first report, got %q", snap.Qglobals["mavuin"])
	}
	if snap.Qglobals["bertox_key"] != "1" {
		t.Errorf("snapshot should carry bertox_key=1 from the second report, got %q", snap.Qglobals["bertox_key"])
	}
}

// TestApplyPopFlagsReportPreservesAutoKills verifies that a popflags reading
// never retracts an 'auto' row on a flag with no qglobal backing (the server
// has no way to print that state, so a reading must not silently un-flag it).
func TestApplyPopFlagsReportPreservesAutoKills(t *testing.T) {
	s := openTempStore(t)
	const char = "Fenwei"

	// bot_agnarr has no Qglobal — only a live kill event can set it.
	if _, err := s.SetAuto(char, "bot_agnarr"); err != nil {
		t.Fatalf("SetAuto: %v", err)
	}
	if !doneByID(t, s, char)["bot_agnarr"].Done {
		t.Fatalf("bot_agnarr should be done after SetAuto")
	}

	report := ParsePopFlagsReport([]string{
		"=== Tier 3 Progression ===",
		"Halls of Honor trials: None completed",
	})
	if _, err := s.ApplyPopFlagsReport(char, report, "raw", time.Unix(1000, 0)); err != nil {
		t.Fatalf("ApplyPopFlagsReport: %v", err)
	}
	if !doneByID(t, s, char)["bot_agnarr"].Done {
		t.Errorf("bot_agnarr (auto, no qglobal) should survive a popflags reading")
	}
}

// TestApplyPopFlagsReportManualPrecedence verifies a manual retraction is
// never overwritten by a later popflags reading — same guarantee ApplySeer
// already provides.
func TestApplyPopFlagsReportManualPrecedence(t *testing.T) {
	s := openTempStore(t)
	const char = "Osui"

	report := ParsePopFlagsReport([]string{
		"=== Tier 2 Progression ===",
		"Mithaniel Marr cipher half: Combined into Cipher",
	})
	if _, err := s.ApplyPopFlagsReport(char, report, "raw", time.Unix(1000, 0)); err != nil {
		t.Fatalf("ApplyPopFlagsReport: %v", err)
	}
	// hoh_mithaniel is satisfied_by cipher>=1; confirm it landed, then
	// manually retract it, then re-apply the same report.
	if !doneByID(t, s, char)["hoh_mithaniel"].Done {
		t.Fatalf("hoh_mithaniel should be satisfied by cipher")
	}
	if err := s.SetManual(char, "hoh_mithaniel", false); err != nil {
		t.Fatalf("SetManual retract: %v", err)
	}
	if _, err := s.ApplyPopFlagsReport(char, report, "raw", time.Unix(2000, 0)); err != nil {
		t.Fatalf("ApplyPopFlagsReport (re-apply): %v", err)
	}
	if row := doneByID(t, s, char)["hoh_mithaniel"]; row.Done || row.Source != SourceManual {
		t.Errorf("manual retraction should survive a popflags re-sync, got %+v", row)
	}
}

// TestApplyPopFlagsReportSeerAndPopflagsShareScope verifies the two reading
// sources coexist: a popflags reading can supersede a prior seer-sourced row
// on the same qglobal-backed flag (latest-applied wins between them).
func TestApplyPopFlagsReportSeerAndPopflagsShareScope(t *testing.T) {
	s := openTempStore(t)
	const char = "Talvashan"

	if _, err := s.ApplySeer(char, map[string]string{"mavuin": "1"}, "seer raw", time.Unix(1000, 0)); err != nil {
		t.Fatalf("ApplySeer: %v", err)
	}
	if row := doneByID(t, s, char)["poj_preflag"]; !row.Done || row.Source != SourceSeer {
		t.Fatalf("poj_preflag should be seer-sourced after the seer reading, got %+v", row)
	}

	report := ParsePopFlagsReport([]string{
		"=== Tier 1 Progression ===",
		"Mavuin's case: Not started",
	})
	if _, err := s.ApplyPopFlagsReport(char, report, "raw", time.Unix(2000, 0)); err != nil {
		t.Fatalf("ApplyPopFlagsReport: %v", err)
	}
	if row := doneByID(t, s, char)["poj_preflag"]; row.Done {
		t.Errorf("a later popflags reading confirming mavuin absent should retract the seer-sourced row, got %+v", row)
	}
}
