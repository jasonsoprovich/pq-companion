package trigger

import "testing"

// Moving a pack trigger into another category must not sever its pack origin:
// deactivating the pack still removes it, and the pack still reads as installed
// while the move stands.
func TestSourcePack_MoveThenUninstall(t *testing.T) {
	s := openTestStore(t)
	if err := InstallPack(s, makePack("PackA", "")); err != nil {
		t.Fatalf("install: %v", err)
	}

	list, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var moved *Trigger
	for _, tr := range list {
		if tr.SourcePack == "PackA" {
			moved = tr
			break
		}
	}
	if moved == nil {
		t.Fatal("no PackA trigger to move")
	}

	// Move it to a custom category (pack_name changes, source_pack must not).
	moved.PackName = "My Custom"
	if err := s.Update(moved); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, err := s.Get(moved.ID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.PackName != "My Custom" || got.SourcePack != "PackA" {
		t.Fatalf("after move: pack_name=%q source_pack=%q (want My Custom / PackA)",
			got.PackName, got.SourcePack)
	}

	// The pack still reads as installed even though no trigger sits under its
	// own category any more.
	installed, err := s.InstalledPackNames()
	if err != nil {
		t.Fatalf("InstalledPackNames: %v", err)
	}
	if !installed["PackA"] {
		t.Fatal("PackA should still read installed after a trigger move")
	}

	// Deactivating the pack removes ALL its triggers, including the moved one.
	if err := s.DeleteByPack("PackA"); err != nil {
		t.Fatalf("DeleteByPack: %v", err)
	}
	after, err := s.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(after) != 0 {
		t.Fatalf("expected all PackA triggers removed, got %d: %+v", len(after), after)
	}
}

// User-authored triggers (no source_pack) survive a pack deactivation even when
// they've been filed under that pack's category name.
func TestSourcePack_UserTriggerSurvivesUninstall(t *testing.T) {
	s := openTestStore(t)
	if err := InstallPack(s, makePack("PackA", "")); err != nil {
		t.Fatalf("install: %v", err)
	}
	// A user trigger filed under PackA's category (pack_name set, source_pack
	// empty — it did not come from the pack).
	user := makeTrigger("mine", "PackA") // makeTrigger leaves SourcePack empty
	if err := s.Insert(user); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	if err := s.DeleteByPack("PackA"); err != nil {
		t.Fatalf("DeleteByPack: %v", err)
	}
	got, err := s.Get(user.ID)
	if err != nil {
		t.Fatalf("user trigger should survive pack uninstall: %v", err)
	}
	if got.Name != "mine" {
		t.Fatalf("unexpected surviving trigger: %+v", got)
	}
}
