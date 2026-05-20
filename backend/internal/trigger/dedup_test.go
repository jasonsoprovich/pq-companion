package trigger

import (
	"testing"
)

// makePack builds a two-trigger pack used by the dedup tests. The first
// trigger carries the supplied dedup_key; the second is class-specific
// (no dedup_key, must always install).
func makePack(packName, dedupKey string) TriggerPack {
	return TriggerPack{
		PackName: packName,
		Triggers: []Trigger{
			{
				Name:     "Shared Trigger",
				Enabled:  true,
				Pattern:  `^shared$`,
				PackName: packName,
				DedupKey: dedupKey,
				Actions:  []Action{{Type: ActionOverlayText, Text: packName}},
			},
			{
				Name:     packName + " Local",
				Enabled:  true,
				Pattern:  `^` + packName + `$`,
				PackName: packName,
				Actions:  []Action{{Type: ActionOverlayText, Text: packName}},
			},
		},
	}
}

func TestInstallPack_SkipsDedupKeyCollision(t *testing.T) {
	s := openTestStore(t)

	if err := InstallPack(s, makePack("PackA", "shared_key")); err != nil {
		t.Fatalf("install PackA: %v", err)
	}
	if err := InstallPack(s, makePack("PackB", "shared_key")); err != nil {
		t.Fatalf("install PackB: %v", err)
	}

	list, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	// PackA contributes both its triggers; PackB only contributes its
	// non-dedup trigger because the shared one was skipped.
	if got, want := len(list), 3; got != want {
		t.Fatalf("expected %d triggers (2 from A + 1 from B), got %d", want, got)
	}

	owner, err := s.FindByDedupKey("shared_key")
	if err != nil {
		t.Fatalf("FindByDedupKey: %v", err)
	}
	if owner == nil {
		t.Fatal("shared_key has no owner")
	}
	if owner.PackName != "PackA" {
		t.Errorf("shared_key owned by %q, want PackA", owner.PackName)
	}
}

func TestUninstallPack_PromotesOrphanedDedupKey(t *testing.T) {
	s := openTestStore(t)

	// Stub AllPacks-equivalent: we can't monkey-patch AllPacks() in tests,
	// so we exercise the promote path by calling promoteOrphanedTriggers
	// directly with a known pack definition. This is the unit under test.
	packA := makePack("PackA", "shared_key")
	packB := makePack("PackB", "shared_key")
	if err := InstallPack(s, packA); err != nil {
		t.Fatalf("install PackA: %v", err)
	}
	if err := InstallPack(s, packB); err != nil {
		t.Fatalf("install PackB: %v", err)
	}

	// Uninstall PackA. shared_key is now orphaned; promote from PackB.
	if err := s.DeleteByPack("PackA"); err != nil {
		t.Fatalf("DeleteByPack: %v", err)
	}
	if err := promoteOrphanedTriggers(s, packB); err != nil {
		t.Fatalf("promoteOrphanedTriggers: %v", err)
	}

	owner, err := s.FindByDedupKey("shared_key")
	if err != nil {
		t.Fatalf("FindByDedupKey: %v", err)
	}
	if owner == nil {
		t.Fatal("shared_key has no owner after promotion")
	}
	if owner.PackName != "PackB" {
		t.Errorf("shared_key promoted to %q, want PackB", owner.PackName)
	}

	list, _ := s.List()
	// PackA wiped (2 triggers gone), PackB's local trigger remains, plus
	// the promoted shared one = 2 total.
	if got, want := len(list), 2; got != want {
		t.Errorf("expected %d triggers after uninstall+promote, got %d", want, got)
	}
}

func TestPromoteOrphanedTriggers_NoopWhenKeyClaimed(t *testing.T) {
	s := openTestStore(t)
	packA := makePack("PackA", "shared_key")
	packB := makePack("PackB", "shared_key")
	if err := InstallPack(s, packA); err != nil {
		t.Fatalf("install PackA: %v", err)
	}
	if err := InstallPack(s, packB); err != nil {
		t.Fatalf("install PackB: %v", err)
	}

	// PackA still installed → promote should be a no-op.
	before, _ := s.List()
	if err := promoteOrphanedTriggers(s, packB); err != nil {
		t.Fatalf("promoteOrphanedTriggers: %v", err)
	}
	after, _ := s.List()
	if len(before) != len(after) {
		t.Errorf("promote was not a no-op: before=%d after=%d", len(before), len(after))
	}
}

func TestInstalledPackNames_ExcludesUserAuthored(t *testing.T) {
	s := openTestStore(t)
	if err := InstallPack(s, makePack("PackA", "")); err != nil {
		t.Fatalf("install PackA: %v", err)
	}
	// Insert a user-authored trigger (empty pack_name) directly.
	id, _ := NewID()
	user := &Trigger{
		ID:      id,
		Name:    "User Trigger",
		Enabled: true,
		Pattern: `^user$`,
		Actions: []Action{},
	}
	if err := s.Insert(user); err != nil {
		t.Fatalf("Insert user trigger: %v", err)
	}

	installed, err := s.InstalledPackNames()
	if err != nil {
		t.Fatalf("InstalledPackNames: %v", err)
	}
	if !installed["PackA"] {
		t.Errorf("PackA should be in installed set: %v", installed)
	}
	if installed[""] {
		t.Errorf("empty pack_name should not be tracked as installed")
	}
	if len(installed) != 1 {
		t.Errorf("expected exactly 1 installed pack, got %d: %v", len(installed), installed)
	}
}

func TestDedupKey_RoundtripPersists(t *testing.T) {
	s := openTestStore(t)
	id, _ := NewID()
	tr := &Trigger{
		ID:       id,
		Name:     "Test",
		Enabled:  true,
		Pattern:  `^test$`,
		DedupKey: "test_key",
		Actions:  []Action{},
	}
	if err := s.Insert(tr); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	got, err := s.Get(id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.DedupKey != "test_key" {
		t.Errorf("DedupKey roundtrip: got %q, want %q", got.DedupKey, "test_key")
	}
}
