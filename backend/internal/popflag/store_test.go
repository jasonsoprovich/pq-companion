package popflag

import (
	"path/filepath"
	"testing"
	"time"
)

func openTempStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenStore(filepath.Join(t.TempDir(), "user.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func doneByID(t *testing.T, s *Store, char string) map[string]State {
	t.Helper()
	rows, err := s.Get(char)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	m := make(map[string]State, len(rows))
	for _, r := range rows {
		m[r.FlagID] = r
	}
	return m
}

func TestApplySeerManualPrecedence(t *testing.T) {
	s := openTempStore(t)
	const char = "Osui"

	// User has manually RETRACTED potor_saryrn (a deliberate correction).
	if err := s.SetManual(char, "potor_saryrn", false); err != nil {
		t.Fatalf("SetManual: %v", err)
	}

	// A Seer reading with only cipher would normally mark potor_saryrn done
	// (via SatisfiedBy). The manual retraction must win.
	q := ParseSeer("The Cipher of the Divine Language appears on your arms for a brief moment then fades.")
	done, err := s.ApplySeer(char, q, "raw", time.Unix(1000, 0))
	if err != nil {
		t.Fatalf("ApplySeer: %v", err)
	}
	if !contains(done, "potor_saryrn") {
		t.Fatalf("derivation should include potor_saryrn (sanity)")
	}

	rows := doneByID(t, s, char)
	if r := rows["potor_saryrn"]; r.Done || r.Source != SourceManual {
		t.Errorf("manual retraction lost: %+v", r)
	}
	// hoh_mithaniel had no manual row → seer wins.
	if r := rows["hoh_mithaniel"]; !r.Done || r.Source != SourceSeer {
		t.Errorf("hoh_mithaniel should be seer-done: %+v", r)
	}
}

func TestApplySeerRetraction(t *testing.T) {
	s := openTempStore(t)
	const char = "Osui"

	// First reading: cipher present → hoh_mithaniel seer-done.
	q1 := ParseSeer("The Cipher of the Divine Language appears on your arms.")
	if _, err := s.ApplySeer(char, q1, "r1", time.Unix(1, 0)); err != nil {
		t.Fatalf("ApplySeer 1: %v", err)
	}
	if !doneByID(t, s, char)["hoh_mithaniel"].Done {
		t.Fatalf("expected hoh_mithaniel done after first reading")
	}

	// Second reading: empty (nothing detected) → stale seer row cleared.
	if _, err := s.ApplySeer(char, map[string]string{}, "r2", time.Unix(2, 0)); err != nil {
		t.Fatalf("ApplySeer 2: %v", err)
	}
	if r, ok := doneByID(t, s, char)["hoh_mithaniel"]; ok && r.Done {
		t.Errorf("hoh_mithaniel should be retracted by the empty reading: %+v", r)
	}
}

func TestSnapshotRoundTrip(t *testing.T) {
	s := openTempStore(t)
	const char = "Osui"
	q := ParseSeer("Your soul has formed a bond with the Plane of Time.")
	if _, err := s.ApplySeer(char, q, "the raw text", time.Unix(42, 0)); err != nil {
		t.Fatalf("ApplySeer: %v", err)
	}
	snap, err := s.GetSnapshot(char)
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}
	if snap == nil {
		t.Fatal("expected a snapshot")
	}
	if snap.Qglobals["time"] != "1" || snap.RawText != "the raw text" || snap.TakenAt != 42 {
		t.Errorf("snapshot round-trip mismatch: %+v", snap)
	}
	if other, _ := s.GetSnapshot("Nobody"); other != nil {
		t.Errorf("expected nil snapshot for unknown character")
	}
}

func TestSetManualOrdering(t *testing.T) {
	s := openTempStore(t)
	const char = "Osui"

	// Can't check a locked step out of order.
	if err := s.SetManual(char, "poj_trial_mark", true); err == nil {
		t.Fatalf("expected SetManual to reject a locked flag")
	}
	if doneByID(t, s, char)["poj_trial_mark"].Done {
		t.Errorf("locked flag should not have been recorded done")
	}

	// Completing the prereq first unlocks it.
	if err := s.SetManual(char, "poj_preflag", true); err != nil {
		t.Fatalf("root flag should be settable: %v", err)
	}
	if err := s.SetManual(char, "poj_trial_mark", true); err != nil {
		t.Fatalf("flag should be settable once prereq is done: %v", err)
	}

	// Retraction of a locked flag is always allowed.
	if err := s.SetManual(char, "hoh_mithaniel", false); err != nil {
		t.Errorf("retracting a locked flag should be allowed: %v", err)
	}

	// Confirming an already-done auto detection on a locked node is allowed
	// (the exception that keeps "auto — confirm?" working).
	if _, err := s.SetAuto(char, "ponb_terris"); err != nil { // locked: ponb_hedge undone
		t.Fatalf("SetAuto: %v", err)
	}
	if err := s.SetManual(char, "ponb_terris", true); err != nil {
		t.Errorf("confirming an already-done locked auto flag should be allowed: %v", err)
	}
	if r := doneByID(t, s, char)["ponb_terris"]; !r.Done || r.Source != SourceManual {
		t.Errorf("confirmed flag should be manual-done: %+v", r)
	}
}

func TestSetManualRetractionGuard(t *testing.T) {
	s := openTempStore(t)
	const char = "Osui"

	// Complete two linked steps in order.
	if err := s.SetManual(char, "poj_preflag", true); err != nil {
		t.Fatalf("set poj_preflag: %v", err)
	}
	if err := s.SetManual(char, "poj_trial_mark", true); err != nil {
		t.Fatalf("set poj_trial_mark: %v", err)
	}

	// Can't retract the prereq while its dependent is still done.
	if err := s.SetManual(char, "poj_preflag", false); err == nil {
		t.Fatalf("expected retraction of poj_preflag to be blocked (poj_trial_mark depends on it)")
	}
	if !doneByID(t, s, char)["poj_preflag"].Done {
		t.Errorf("poj_preflag should still be done after blocked retraction")
	}

	// Retract top-down: the dependent first (no done dependents of its own)...
	if err := s.SetManual(char, "poj_trial_mark", false); err != nil {
		t.Fatalf("retracting the dependent should be allowed: %v", err)
	}
	// ...then the prereq is free to retract.
	if err := s.SetManual(char, "poj_preflag", false); err != nil {
		t.Errorf("retracting poj_preflag should be allowed once nothing depends on it: %v", err)
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
