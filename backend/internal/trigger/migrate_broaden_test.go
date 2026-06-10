package trigger

import (
	"strings"
	"testing"
	"time"
)

const migrateBroadenKey = "Packs:BroadenDebuffPatterns+BardDurations:v1"

// builtinTrigger returns the (pack, name) trigger from the current built-in
// definitions so tests assert against the live target values rather than a
// hand-copied string.
func builtinTrigger(t *testing.T, pack, name string) Trigger {
	t.Helper()
	for _, p := range AllPacks() {
		if p.PackName != pack {
			continue
		}
		for _, tr := range p.Triggers {
			if tr.Name == name {
				return tr
			}
		}
	}
	t.Fatalf("built-in trigger %s/%s not found", pack, name)
	return Trigger{}
}

// preBroadenPattern reverses npcNameClass back to the old uppercase-only name
// class, reconstructing what an old install's pattern column held.
func preBroadenPattern(p string) string {
	return strings.ReplaceAll(p, npcNameClass, `[A-Z][a-zA-Z']{2,14}`)
}

// insertInstalledTrigger writes a row mimicking an already-installed pack
// trigger, with the given pre-fix pattern/duration and a user-style action so
// tests can confirm actions survive the migration.
func insertInstalledTrigger(t *testing.T, s *Store, pack, name, pattern string, dur int) *Trigger {
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
		TimerType:         TimerTypeDetrimental,
		TimerDurationSecs: dur,
		CreatedAt:         time.Now().UTC(),
		Actions: []Action{
			{Type: ActionTextToSpeech, Text: "my custom voice line", Volume: 0.5},
			{Type: ActionOverlayText, Text: "CUSTOM", DurationSecs: 7, Color: "#abcdef"},
		},
		TimerAlerts: []TimerAlert{
			{ID: "user-alert", Seconds: 5, Type: TimerAlertTypeTextToSpeech, TTSTemplate: "mine", TTSVolume: 80},
		},
	}
	if err := s.Insert(tr); err != nil {
		t.Fatalf("insert installed trigger: %v", err)
	}
	return tr
}

func TestMigrateBroaden_UpdatesStaleDebuffPattern(t *testing.T) {
	s := openTestStore(t)
	want := builtinTrigger(t, "Enchanter", "Cripple")
	old := preBroadenPattern(want.Pattern)
	if old == want.Pattern {
		t.Fatal("test setup: Cripple pattern does not contain npcNameClass")
	}
	insertInstalledTrigger(t, s, "Enchanter", "Cripple", old, want.TimerDurationSecs)

	if err := s.MigrateBroadenDebuffPatternsAndBardDurations(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	got, err := s.FindByPackAndName("Enchanter", "Cripple")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got.Pattern != want.Pattern {
		t.Errorf("pattern not migrated:\n got  %q\n want %q", got.Pattern, want.Pattern)
	}
}

func TestMigrateBroaden_UpdatesBardSongDuration(t *testing.T) {
	s := openTestStore(t)
	// Warsong of Zek: old install had 54s, built-in is now 18s.
	want := builtinTrigger(t, "Bard", "Warsong of Zek")
	if want.TimerDurationSecs != 18 {
		t.Fatalf("test setup: expected built-in Warsong of Zek = 18s, got %d", want.TimerDurationSecs)
	}
	insertInstalledTrigger(t, s, "Bard", "Warsong of Zek", want.Pattern, 54)

	if err := s.MigrateBroadenDebuffPatternsAndBardDurations(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	got, err := s.FindByPackAndName("Bard", "Warsong of Zek")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got.TimerDurationSecs != 18 {
		t.Errorf("duration not migrated: got %d, want 18", got.TimerDurationSecs)
	}
}

func TestMigrateBroaden_FixesBothPatternAndDuration(t *testing.T) {
	s := openTestStore(t)
	// Largo's Absonant Binding changed in BOTH columns (pattern + 54→18).
	want := builtinTrigger(t, "Bard", "Largo's Absonant Binding")
	old := preBroadenPattern(want.Pattern)
	insertInstalledTrigger(t, s, "Bard", "Largo's Absonant Binding", old, 54)

	if err := s.MigrateBroadenDebuffPatternsAndBardDurations(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	got, err := s.FindByPackAndName("Bard", "Largo's Absonant Binding")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got.Pattern != want.Pattern {
		t.Errorf("pattern not migrated:\n got  %q\n want %q", got.Pattern, want.Pattern)
	}
	if got.TimerDurationSecs != 18 {
		t.Errorf("duration not migrated: got %d, want 18", got.TimerDurationSecs)
	}
}

func TestMigrateBroaden_PreservesUserActionsAndAlerts(t *testing.T) {
	s := openTestStore(t)
	want := builtinTrigger(t, "Enchanter", "Cripple")
	old := preBroadenPattern(want.Pattern)
	orig := insertInstalledTrigger(t, s, "Enchanter", "Cripple", old, want.TimerDurationSecs)

	if err := s.MigrateBroadenDebuffPatternsAndBardDurations(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	got, err := s.FindByPackAndName("Enchanter", "Cripple")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got.ID != orig.ID {
		t.Errorf("row was replaced: id %q -> %q", orig.ID, got.ID)
	}
	if len(got.Actions) != 2 {
		t.Fatalf("actions clobbered: got %d, want 2", len(got.Actions))
	}
	if got.Actions[0].Text != "my custom voice line" || got.Actions[0].Volume != 0.5 {
		t.Errorf("TTS action changed: %+v", got.Actions[0])
	}
	if got.Actions[1].Text != "CUSTOM" || got.Actions[1].Color != "#abcdef" {
		t.Errorf("overlay action changed: %+v", got.Actions[1])
	}
	if len(got.TimerAlerts) != 1 || got.TimerAlerts[0].ID != "user-alert" {
		t.Errorf("timer alerts changed: %+v", got.TimerAlerts)
	}
}

func TestMigrateBroaden_LeavesCustomizedPatternAlone(t *testing.T) {
	s := openTestStore(t)
	custom := `^my totally custom cripple pattern$`
	insertInstalledTrigger(t, s, "Enchanter", "Cripple", custom, 450)

	if err := s.MigrateBroadenDebuffPatternsAndBardDurations(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	got, err := s.FindByPackAndName("Enchanter", "Cripple")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got.Pattern != custom {
		t.Errorf("customized pattern was overwritten: got %q", got.Pattern)
	}
}

func TestMigrateBroaden_LeavesCustomizedBardDurationAlone(t *testing.T) {
	s := openTestStore(t)
	// User set a non-default 30s duration — not the pre-fix 54s — so leave it.
	want := builtinTrigger(t, "Bard", "Warsong of Zek")
	insertInstalledTrigger(t, s, "Bard", "Warsong of Zek", want.Pattern, 30)

	if err := s.MigrateBroadenDebuffPatternsAndBardDurations(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	got, err := s.FindByPackAndName("Bard", "Warsong of Zek")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got.TimerDurationSecs != 30 {
		t.Errorf("customized duration was overwritten: got %d, want 30", got.TimerDurationSecs)
	}
}

func TestMigrateBroaden_Idempotent(t *testing.T) {
	s := openTestStore(t)
	want := builtinTrigger(t, "Enchanter", "Cripple")
	old := preBroadenPattern(want.Pattern)
	insertInstalledTrigger(t, s, "Enchanter", "Cripple", old, want.TimerDurationSecs)

	if err := s.MigrateBroadenDebuffPatternsAndBardDurations(); err != nil {
		t.Fatalf("first run: %v", err)
	}
	// Re-create a stale row after the first run; a keyed migration must not
	// re-fire and touch it.
	if err := s.DeleteByPack("Enchanter"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	stale := insertInstalledTrigger(t, s, "Enchanter", "Cripple", old, want.TimerDurationSecs)
	if err := s.MigrateBroadenDebuffPatternsAndBardDurations(); err != nil {
		t.Fatalf("second run: %v", err)
	}
	got, err := s.FindByPackAndName("Enchanter", "Cripple")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got.Pattern != stale.Pattern {
		t.Error("second run mutated a row after migration was already marked applied")
	}
}

func TestMigrateBroaden_NoOpWhenAbsentMarksApplied(t *testing.T) {
	s := openTestStore(t)
	if err := s.MigrateBroadenDebuffPatternsAndBardDurations(); err != nil {
		t.Fatalf("migrate on empty store: %v", err)
	}
	applied, err := s.IsDefaultUpdateApplied(migrateBroadenKey)
	if err != nil {
		t.Fatalf("IsDefaultUpdateApplied: %v", err)
	}
	if !applied {
		t.Error("migration should be marked applied even on empty store")
	}
}
