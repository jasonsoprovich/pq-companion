package trigger

import (
	"testing"
	"time"
)

// insertLegacyEnchanterDupe writes a row that mimics what an older install
// would have: the deprecated Enchanter pack triggers that duplicated the
// General Triggers ones. Returns the inserted trigger so the test can use
// its ID for follow-up assertions.
func insertLegacyEnchanterDupe(t *testing.T, s *Store, name, pattern string) *Trigger {
	t.Helper()
	id, err := NewID()
	if err != nil {
		t.Fatalf("NewID: %v", err)
	}
	tr := &Trigger{
		ID:        id,
		Name:      name,
		Enabled:   true,
		Pattern:   pattern,
		PackName:  "Enchanter",
		CreatedAt: time.Now().UTC(),
		Actions: []Action{
			{Type: ActionOverlayText, Text: "X", DurationSecs: 3, Color: "#fff"},
		},
	}
	if err := s.Insert(tr); err != nil {
		t.Fatalf("insert legacy dupe: %v", err)
	}
	return tr
}

func TestMigrateRemoveDuplicateClassPackTriggers_DeletesDefaults(t *testing.T) {
	s := openTestStore(t)
	insertLegacyEnchanterDupe(t, s, "Spell Resisted", `Your target resisted the .+ spell\.`)
	insertLegacyEnchanterDupe(t, s, "Spell Interrupted", `Your spell is interrupted\.`)

	if err := s.MigrateRemoveDuplicateClassPackTriggers(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	for _, name := range []string{"Spell Resisted", "Spell Interrupted"} {
		got, err := s.FindByPackAndName("Enchanter", name)
		if err != nil {
			t.Fatalf("find %s: %v", name, err)
		}
		if got != nil {
			t.Errorf("%s still present after migration", name)
		}
	}
}

func TestMigrateRemoveDuplicateClassPackTriggers_PreservesCustomizedPattern(t *testing.T) {
	s := openTestStore(t)
	// User customized the pattern — migration must leave it alone.
	custom := insertLegacyEnchanterDupe(t, s, "Spell Resisted", `My custom resist pattern`)

	if err := s.MigrateRemoveDuplicateClassPackTriggers(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	got, err := s.FindByPackAndName("Enchanter", "Spell Resisted")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got == nil {
		t.Fatal("customized trigger was deleted; should have been preserved")
	}
	if got.ID != custom.ID {
		t.Errorf("ID changed: got %q, want %q", got.ID, custom.ID)
	}
}

func TestMigrateRemoveDuplicateClassPackTriggers_Idempotent(t *testing.T) {
	s := openTestStore(t)
	insertLegacyEnchanterDupe(t, s, "Spell Resisted", `Your target resisted the .+ spell\.`)

	if err := s.MigrateRemoveDuplicateClassPackTriggers(); err != nil {
		t.Fatalf("first run: %v", err)
	}
	// Second run must be a no-op even if a user re-creates the trigger
	// after the migration ran the first time — the migration is keyed and
	// shouldn't re-fire.
	insertLegacyEnchanterDupe(t, s, "Spell Resisted", `Your target resisted the .+ spell\.`)
	if err := s.MigrateRemoveDuplicateClassPackTriggers(); err != nil {
		t.Fatalf("second run: %v", err)
	}
	got, err := s.FindByPackAndName("Enchanter", "Spell Resisted")
	if err != nil {
		t.Fatalf("find: %v", err)
	}
	if got == nil {
		t.Error("second run should NOT have deleted a re-created trigger (migration already marked applied)")
	}
}

func TestMigrateRemoveDuplicateClassPackTriggers_NoOpWhenAbsent(t *testing.T) {
	// Fresh store with nothing to delete — should run cleanly and mark applied.
	s := openTestStore(t)
	if err := s.MigrateRemoveDuplicateClassPackTriggers(); err != nil {
		t.Fatalf("migrate on empty store: %v", err)
	}
	applied, err := s.IsDefaultUpdateApplied("ClassPackDupes:RemoveResistInterrupt:v1")
	if err != nil {
		t.Fatalf("IsDefaultUpdateApplied: %v", err)
	}
	if !applied {
		t.Error("migration should be marked applied even on empty store")
	}
}
