package emote

import (
	"os"
	"testing"
)

// TestBootstrapDetectsPreExistingHandEdit is the scenario reported by an
// actual user (Discord: Fondue) who had already hand-edited spells_en.txt to
// disambiguate slow emotes before this feature existed. On first bootstrap
// the live file's Turgur's Insects line already differs from quarm.db's
// canonical text — that must surface as a pending import, not be silently
// adopted as if it were the pristine default.
func TestBootstrapDetectsPreExistingHandEdit(t *testing.T) {
	svc := newTestService(t)

	// Simulate Fondue's pre-existing hand-edit, made entirely outside the
	// app before ever opening the Spell Emotes panel.
	livePath, _ := svc.livePath()
	original, err := os.ReadFile(livePath)
	if err != nil {
		t.Fatalf("read fixture copy: %v", err)
	}
	handEdited := applyOverrides(string(original), map[int]OverrideRow{
		1588: {SpellID: 1588, CastOnOther: strPtr(" yawns. (Turgur's Insects)")},
	})
	if err := os.WriteFile(livePath, []byte(handEdited), 0o644); err != nil {
		t.Fatalf("write hand-edited file: %v", err)
	}

	// First-ever use of the feature: EnsureDefaultBackup (as SetOverride on
	// an unrelated spell would trigger) must detect the divergence.
	if err := svc.EnsureDefaultBackup(); err != nil {
		t.Fatalf("EnsureDefaultBackup: %v", err)
	}

	pending, err := svc.PendingImports()
	if err != nil {
		t.Fatalf("PendingImports: %v", err)
	}
	if len(pending) != 1 || pending[0].SpellID != 1588 {
		t.Fatalf("expected exactly one pending import for spell 1588, got %+v", pending)
	}
	f := pending[0].Fields[0]
	if f.Field != "cast_on_other" || f.Old != " yawns." || f.New != " yawns. (Turgur's Insects)" {
		t.Fatalf("unexpected pending field diff: %+v", f)
	}

	// Even before it's formally imported, GetSpellEmote must flag it as
	// customized — a user looking at this spell's emotes (e.g. on the Spells
	// page detail pane) needs to see "this differs from default" and get a
	// revert option regardless of whether the app has gotten around to
	// tracking it as an official override row yet. "Customized" reflects
	// Default vs Current, not the store's bookkeeping.
	se, err := svc.GetSpellEmote(1588)
	if err != nil {
		t.Fatalf("GetSpellEmote: %v", err)
	}
	if !se.Customized {
		t.Fatal("a pending (not-yet-imported) hand-edit must still show as customized")
	}
	if len(se.OverriddenFields) != 1 || se.OverriddenFields[0] != "cast_on_other" {
		t.Fatalf("unexpected overridden fields: %+v", se.OverriddenFields)
	}
	if se.Current.CastOnOther != " yawns. (Turgur's Insects)" {
		t.Fatalf("hand-edited text lost before import: %q", se.Current.CastOnOther)
	}
}

// TestRevertOverrideClearsPendingImportEvenWithoutTrackedRow is the direct
// regression for the bug report: a spell whose emote was altered but never
// formally imported (still just a pending-import entry, no row in
// emote_overrides) must actually reset when the user clicks "Revert to
// default" — not silently no-op because DeleteOverride had nothing to
// delete while the pending-import protection kept the altered text alive.
func TestRevertOverrideClearsPendingImportEvenWithoutTrackedRow(t *testing.T) {
	svc := newTestService(t)

	livePath, _ := svc.livePath()
	original, err := os.ReadFile(livePath)
	if err != nil {
		t.Fatalf("read fixture copy: %v", err)
	}
	handEdited := applyOverrides(string(original), map[int]OverrideRow{
		207: {SpellID: 207, CastOnYou: strPtr("The gods have rendered you invulnerable (blessed).")},
	})
	if err := os.WriteFile(livePath, []byte(handEdited), 0o644); err != nil {
		t.Fatalf("write hand-edited file: %v", err)
	}
	if err := svc.EnsureDefaultBackup(); err != nil {
		t.Fatalf("EnsureDefaultBackup: %v", err)
	}

	// Confirm it's customized-but-untracked before reverting.
	before, err := svc.GetSpellEmote(207)
	if err != nil {
		t.Fatalf("GetSpellEmote: %v", err)
	}
	if !before.Customized {
		t.Fatal("expected spell to read as customized before revert")
	}
	if ov, err := svc.store.GetOverride(207); err != nil {
		t.Fatalf("GetOverride: %v", err)
	} else if ov != nil {
		t.Fatal("expected no tracked override row yet (still just a pending import)")
	}

	if err := svc.RevertOverride(207); err != nil {
		t.Fatalf("RevertOverride: %v", err)
	}

	after, err := svc.GetSpellEmote(207)
	if err != nil {
		t.Fatalf("GetSpellEmote after revert: %v", err)
	}
	if after.Customized {
		t.Fatalf("expected spell to no longer be customized after revert, got %+v", after)
	}
	if after.Current.CastOnYou != "The gods have rendered you invulnerable." {
		t.Fatalf("expected revert to restore canonical text, got %q", after.Current.CastOnYou)
	}

	pending, err := svc.PendingImports()
	if err != nil {
		t.Fatalf("PendingImports: %v", err)
	}
	for _, sd := range pending {
		if sd.SpellID == 207 {
			t.Fatalf("expected spell 207 removed from pending imports after revert, got %+v", pending)
		}
	}
}

// TestPendingImportProtectedAcrossUnrelatedEdit ensures an unrelated
// SetOverride/rebuild doesn't silently erase a hand-edit still awaiting the
// user's import decision — the core correctness property of the pending
// mechanism, not just its detection.
func TestPendingImportProtectedAcrossUnrelatedEdit(t *testing.T) {
	svc := newTestService(t)

	livePath, _ := svc.livePath()
	original, err := os.ReadFile(livePath)
	if err != nil {
		t.Fatalf("read fixture copy: %v", err)
	}
	handEdited := applyOverrides(string(original), map[int]OverrideRow{
		1588: {SpellID: 1588, CastOnOther: strPtr(" yawns. (Turgur's Insects)")},
	})
	if err := os.WriteFile(livePath, []byte(handEdited), 0o644); err != nil {
		t.Fatalf("write hand-edited file: %v", err)
	}

	// Editing a completely different spell (207) triggers EnsureDefaultBackup
	// + a full rebuild — this must not wipe spell 1588's pending hand-edit.
	if err := svc.SetOverride(207, ColumnsPatch{CastOnOther: strPtr(" has had Divine Aura cast on them.")}); err != nil {
		t.Fatalf("SetOverride: %v", err)
	}

	live, err := os.ReadFile(livePath)
	if err != nil {
		t.Fatalf("read live file: %v", err)
	}
	turgurs := findSpellLine(string(live), 1588)
	if turgurs[idxCastOnOther] != " yawns. (Turgur's Insects)" {
		t.Fatalf("pending hand-edit was wiped by an unrelated rebuild: %q", turgurs[idxCastOnOther])
	}

	pending, err := svc.PendingImports()
	if err != nil {
		t.Fatalf("PendingImports: %v", err)
	}
	if len(pending) != 1 || pending[0].SpellID != 1588 {
		t.Fatalf("expected pending import to survive the unrelated edit, got %+v", pending)
	}
}

// TestImportExistingAdoptsAsTrackedOverride covers the user's explicit
// "import as tracked customizations" action: the spell becomes a real,
// diffable, patch-surviving override, the pending list clears, and the live
// file's bytes don't change (they were already correct).
func TestImportExistingAdoptsAsTrackedOverride(t *testing.T) {
	svc := newTestService(t)

	livePath, _ := svc.livePath()
	original, err := os.ReadFile(livePath)
	if err != nil {
		t.Fatalf("read fixture copy: %v", err)
	}
	handEdited := applyOverrides(string(original), map[int]OverrideRow{
		1588: {SpellID: 1588, CastOnOther: strPtr(" yawns. (Turgur's Insects)")},
	})
	if err := os.WriteFile(livePath, []byte(handEdited), 0o644); err != nil {
		t.Fatalf("write hand-edited file: %v", err)
	}
	if err := svc.EnsureDefaultBackup(); err != nil {
		t.Fatalf("EnsureDefaultBackup: %v", err)
	}

	beforeImport, err := os.ReadFile(livePath)
	if err != nil {
		t.Fatalf("read live file: %v", err)
	}

	imported, err := svc.ImportExisting(nil) // nil = import all
	if err != nil {
		t.Fatalf("ImportExisting: %v", err)
	}
	if imported != 1 {
		t.Fatalf("expected 1 spell imported, got %d", imported)
	}

	afterImport, err := os.ReadFile(livePath)
	if err != nil {
		t.Fatalf("read live file: %v", err)
	}
	if string(beforeImport) != string(afterImport) {
		t.Fatal("importing an existing customization must not change the live file's bytes")
	}

	pending, err := svc.PendingImports()
	if err != nil {
		t.Fatalf("PendingImports: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected pending list to be empty after import, got %+v", pending)
	}

	se, err := svc.GetSpellEmote(1588)
	if err != nil {
		t.Fatalf("GetSpellEmote: %v", err)
	}
	if !se.Customized {
		t.Fatal("expected spell to be a tracked customization after import")
	}

	// Now it must behave like any other tracked override: RevertOverride
	// takes it back to the true (quarm.db) default.
	if err := svc.RevertOverride(1588); err != nil {
		t.Fatalf("RevertOverride: %v", err)
	}
	se, err = svc.GetSpellEmote(1588)
	if err != nil {
		t.Fatalf("GetSpellEmote after revert: %v", err)
	}
	if se.Current.CastOnOther != " yawns." {
		t.Fatalf("expected revert to restore canonical text, got %q", se.Current.CastOnOther)
	}
}

// TestRestoreDefaultsClearsPendingImports confirms the nuclear "Restore
// Defaults" option also clears any not-yet-reviewed pending import — leaving
// a stale prompt around for spells that were just wiped back to canonical
// would be confusing and wrong.
func TestRestoreDefaultsClearsPendingImports(t *testing.T) {
	svc := newTestService(t)

	livePath, _ := svc.livePath()
	original, err := os.ReadFile(livePath)
	if err != nil {
		t.Fatalf("read fixture copy: %v", err)
	}
	handEdited := applyOverrides(string(original), map[int]OverrideRow{
		1588: {SpellID: 1588, CastOnOther: strPtr(" yawns. (Turgur's Insects)")},
	})
	if err := os.WriteFile(livePath, []byte(handEdited), 0o644); err != nil {
		t.Fatalf("write hand-edited file: %v", err)
	}
	if err := svc.EnsureDefaultBackup(); err != nil {
		t.Fatalf("EnsureDefaultBackup: %v", err)
	}

	if err := svc.RestoreDefaults(); err != nil {
		t.Fatalf("RestoreDefaults: %v", err)
	}

	pending, err := svc.PendingImports()
	if err != nil {
		t.Fatalf("PendingImports: %v", err)
	}
	if len(pending) != 0 {
		t.Fatalf("expected pending imports cleared after RestoreDefaults, got %+v", pending)
	}
}
