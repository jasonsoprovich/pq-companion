package trigger

import (
	"strings"
	"testing"
	"time"
)

// insertInstalledBuff writes a row mimicking a pack buff trigger installed
// before target capture: the given (pre-feature) pattern, empty target
// capture, and user-style actions so the test can confirm they survive.
func insertInstalledBuff(t *testing.T, s *Store, pack, name, pattern string) *Trigger {
	t.Helper()
	id, err := NewID()
	if err != nil {
		t.Fatalf("NewID: %v", err)
	}
	tr := &Trigger{
		ID:                id,
		Name:              name,
		Enabled:           true,
		Pattern:           pattern,
		PackName:          pack,
		SourcePack:        pack,
		TimerType:         TimerTypeBuff,
		TimerDurationSecs: 2520,
		CreatedAt:         time.Now().UTC(),
		Actions:           []Action{{Type: ActionOverlayText, Text: "MINE", Color: "#abcdef"}},
	}
	if err := s.Insert(tr); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	return tr
}

// TestMigrateAddBuffTargetCapture upgrades an un-customized installed buff to
// capture the target, leaves a user-customized pattern untouched, and is
// idempotent.
func TestMigrateAddBuffTargetCapture(t *testing.T) {
	s := openTestStore(t)

	// The live built-in VoG trigger (post-transform) is what an install would
	// get today; the old install held the same pattern with the name unwrapped.
	vog := builtinTrigger(t, "Enchanter", "Visions of Grandeur")
	if vog.TimerTargetCapture != "target" || !strings.Contains(vog.Pattern, "(?P<target>") {
		t.Fatalf("built-in VoG not transformed: capture=%q pattern=%q", vog.TimerTargetCapture, vog.Pattern)
	}
	oldPattern := strings.Replace(vog.Pattern, targetCaptureGroup, playerNameClass, 1)

	upgraded := insertInstalledBuff(t, s, "Enchanter", "Visions of Grandeur", oldPattern)

	// A different built-in buff the user has customized (edited pattern) — must
	// be left exactly as-is.
	custom := insertInstalledBuff(t, s, "Enchanter", "Koadic's Endless Intellect",
		`^You hand-edited this\.$`)

	if err := s.MigrateAddBuffTargetCapture(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	got, err := s.Get(upgraded.ID)
	if err != nil {
		t.Fatalf("Get upgraded: %v", err)
	}
	if got.Pattern != vog.Pattern || got.TimerTargetCapture != "target" {
		t.Errorf("upgraded row = pattern %q capture %q, want wrapped + target",
			got.Pattern, got.TimerTargetCapture)
	}
	if len(got.Actions) != 1 || got.Actions[0].Text != "MINE" {
		t.Errorf("user action not preserved: %+v", got.Actions)
	}

	gotCustom, err := s.Get(custom.ID)
	if err != nil {
		t.Fatalf("Get custom: %v", err)
	}
	if gotCustom.Pattern != `^You hand-edited this\.$` || gotCustom.TimerTargetCapture != "" {
		t.Errorf("customized row was modified: pattern %q capture %q",
			gotCustom.Pattern, gotCustom.TimerTargetCapture)
	}

	// Idempotent: a second run patches nothing further (ledger-guarded).
	if err := s.MigrateAddBuffTargetCapture(); err != nil {
		t.Fatalf("migrate (2nd): %v", err)
	}
}
