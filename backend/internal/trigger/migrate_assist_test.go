package trigger

import (
	"testing"
	"time"
)

// Verbatim pre-fix and current built-in patterns for the Raid Alerts assist
// call, mirrored from MigrateBroadenAssistCallPattern / RaidAlertsPack.
const (
	assistOldPattern = "(?i)^(\\w+) tells the raid,\\s*'.*?assist\\W*([A-Za-z][A-Za-z`' ]*?)(?:\\s*[>|<!']|$)"
	assistNewPattern = "(?i)^(\\w+) tells the raid,\\s*'.*?\\b(?:assist|kill)\\b\\W*([A-Za-z][A-Za-z`' ]*?)(?:\\s*[-<>|!']|$)"
)

// insertInstalledAssist writes a row mimicking a Raid Assist Call trigger
// installed before the kill-call broadening, with a user-style action so the
// test can confirm it survives.
func insertInstalledAssist(t *testing.T, s *Store, pattern string) *Trigger {
	t.Helper()
	id, err := NewID()
	if err != nil {
		t.Fatalf("NewID: %v", err)
	}
	tr := &Trigger{
		ID:         id,
		Name:       "Raid Assist Call",
		Enabled:    true,
		Pattern:    pattern,
		PackName:   "Raid Alerts",
		SourcePack: "Raid Alerts",
		CreatedAt:  time.Now().UTC(),
		Actions:    []Action{{Type: ActionOverlayText, Text: "MINE", Color: "#abcdef"}},
	}
	if err := s.Insert(tr); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	return tr
}

func TestMigrateBroadenAssistCall_UpdatesStalePattern(t *testing.T) {
	s := openTestStore(t)

	// Guard against drift: the live built-in must equal the migration's target.
	bt := builtinTrigger(t, "Raid Alerts", "Raid Assist Call")
	if bt.Pattern != assistNewPattern {
		t.Fatalf("built-in pattern drifted from migration newPattern:\n got %q\nwant %q",
			bt.Pattern, assistNewPattern)
	}

	row := insertInstalledAssist(t, s, assistOldPattern)
	if err := s.MigrateBroadenAssistCallPattern(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	got, err := s.Get(row.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Pattern != assistNewPattern {
		t.Errorf("pattern = %q, want %q", got.Pattern, assistNewPattern)
	}
	if len(got.Actions) != 1 || got.Actions[0].Text != "MINE" {
		t.Errorf("user action not preserved: %+v", got.Actions)
	}
}

func TestMigrateBroadenAssistCall_LeavesCustomizedAlone(t *testing.T) {
	s := openTestStore(t)
	custom := insertInstalledAssist(t, s, `^hand edited$`)
	if err := s.MigrateBroadenAssistCallPattern(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	got, err := s.Get(custom.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Pattern != `^hand edited$` {
		t.Errorf("customized pattern modified: %q", got.Pattern)
	}
}

func TestMigrateBroadenAssistCall_Idempotent(t *testing.T) {
	s := openTestStore(t)
	row := insertInstalledAssist(t, s, assistOldPattern)
	if err := s.MigrateBroadenAssistCallPattern(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Re-customize after the first run; a second run must not touch it.
	got, _ := s.Get(row.ID)
	got.Pattern = `^custom after migrate$`
	if err := s.Update(got); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if err := s.MigrateBroadenAssistCallPattern(); err != nil {
		t.Fatalf("migrate 2: %v", err)
	}
	again, _ := s.Get(row.ID)
	if again.Pattern != `^custom after migrate$` {
		t.Errorf("2nd run modified row: %q", again.Pattern)
	}
}
