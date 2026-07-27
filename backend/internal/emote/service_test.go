package emote

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
)

// newTestService wires a Service against a temp copy of the real
// spells_en.txt fixture, a temp backup dir, and a temp user.db-backed Store —
// nothing here ever touches the checked-in fixture or a real EQ install.
func newTestService(t *testing.T) *Service {
	t.Helper()
	eqDir := copyFixture(t)

	cfgMgr, err := config.LoadFrom(filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatalf("config.LoadFrom: %v", err)
	}
	if err := cfgMgr.Modify(func(c *config.Config) { c.EQPath = eqDir }); err != nil {
		t.Fatalf("set EQPath: %v", err)
	}

	store, err := OpenStore(filepath.Join(t.TempDir(), "user.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	backupDir := filepath.Join(t.TempDir(), "spell-emotes")
	return NewService(store, cfgMgr, backupDir)
}

func strPtr(s string) *string { return &s }

func TestServiceSetOverrideWritesLiveFileAndBackups(t *testing.T) {
	svc := newTestService(t)

	if err := svc.SetOverride(1588, ColumnsPatch{CastOnOther: strPtr(" yawns. (Turgur's Insects)")}); err != nil {
		t.Fatalf("SetOverride: %v", err)
	}

	livePath, _ := svc.livePath()
	live, err := os.ReadFile(livePath)
	if err != nil {
		t.Fatalf("read live file: %v", err)
	}
	fields := findSpellLine(string(live), 1588)
	if fields[idxCastOnOther] != " yawns. (Turgur's Insects)" {
		t.Fatalf("override not reflected in live file: %q", fields[idxCastOnOther])
	}

	if _, ok, _ := readBackup(svc.defaultBackupPath()); !ok {
		t.Fatal("expected default backup to be captured on first edit")
	}
	editedContent, ok, _ := readBackup(svc.editedBackupPath())
	if !ok {
		t.Fatal("expected edited backup to be written")
	}
	if findSpellLine(editedContent, 1588)[idxCastOnOther] != " yawns. (Turgur's Insects)" {
		t.Fatal("edited backup doesn't reflect the override")
	}

	se, err := svc.GetSpellEmote(1588)
	if err != nil {
		t.Fatalf("GetSpellEmote: %v", err)
	}
	if !se.Customized || se.Current.CastOnOther != " yawns. (Turgur's Insects)" {
		t.Fatalf("unexpected SpellEmote: %+v", se)
	}
	if se.Default.CastOnOther != " yawns." {
		t.Fatalf("default should still read the pristine emote, got %q", se.Default.CastOnOther)
	}
}

func TestServiceRevertOverrideRestoresDefaultText(t *testing.T) {
	svc := newTestService(t)
	if err := svc.SetOverride(207, ColumnsPatch{CastOnOther: strPtr(" has had Divine Aura cast on them.")}); err != nil {
		t.Fatalf("SetOverride: %v", err)
	}
	if err := svc.RevertOverride(207); err != nil {
		t.Fatalf("RevertOverride: %v", err)
	}

	se, err := svc.GetSpellEmote(207)
	if err != nil {
		t.Fatalf("GetSpellEmote: %v", err)
	}
	if se.Customized {
		t.Fatal("expected spell to no longer be customized after revert")
	}
	if se.Current.CastOnOther != "" {
		t.Fatalf("expected reverted emote to go back to empty, got %q", se.Current.CastOnOther)
	}
}

func TestServiceRestoreDefaultsClearsAllOverrides(t *testing.T) {
	svc := newTestService(t)
	if err := svc.SetOverride(1588, ColumnsPatch{CastOnOther: strPtr(" yawns. (Turgur's Insects)")}); err != nil {
		t.Fatalf("SetOverride: %v", err)
	}
	if err := svc.SetOverride(207, ColumnsPatch{CastOnOther: strPtr(" has had Divine Aura cast on them.")}); err != nil {
		t.Fatalf("SetOverride: %v", err)
	}

	if err := svc.RestoreDefaults(); err != nil {
		t.Fatalf("RestoreDefaults: %v", err)
	}

	livePath, _ := svc.livePath()
	live, err := os.ReadFile(livePath)
	if err != nil {
		t.Fatalf("read live file: %v", err)
	}
	original := readFixture(t)
	if string(live) != original {
		t.Fatal("expected live file to be byte-identical to the pristine fixture after restore")
	}

	overrides, err := svc.store.ListOverrides()
	if err != nil {
		t.Fatalf("ListOverrides: %v", err)
	}
	if len(overrides) != 0 {
		t.Fatalf("expected all overrides cleared, got %d", len(overrides))
	}
}

func TestServiceDiffReportsChangedColumnsOnly(t *testing.T) {
	svc := newTestService(t)
	if err := svc.SetOverride(1588, ColumnsPatch{CastOnOther: strPtr(" yawns. (Turgur's Insects)")}); err != nil {
		t.Fatalf("SetOverride: %v", err)
	}

	diffs, err := svc.Diff()
	if err != nil {
		t.Fatalf("Diff: %v", err)
	}
	if len(diffs) != 1 {
		t.Fatalf("expected exactly 1 changed spell, got %d", len(diffs))
	}
	d := diffs[0]
	if d.SpellID != 1588 || d.Name != "Turgur's Insects" {
		t.Fatalf("unexpected diff entry: %+v", d)
	}
	if len(d.Fields) != 1 {
		t.Fatalf("expected exactly 1 changed field, got %d: %+v", len(d.Fields), d.Fields)
	}
	f := d.Fields[0]
	if f.Field != "cast_on_other" || f.Old != " yawns." || f.New != " yawns. (Turgur's Insects)" {
		t.Fatalf("unexpected field diff: %+v", f)
	}
}

// TestServerPatchReapplyPreservesBothChanges simulates a Project Quarm patch
// republishing spells_en.txt with an unrelated data change, while the app
// still has a stored emote override for a different spell. ReapplyAll must
// land on a file that carries BOTH the server's new content AND the user's
// override — this is the core guarantee the whole feature exists for.
func TestServerPatchReapplyPreservesBothChanges(t *testing.T) {
	svc := newTestService(t)
	if err := svc.SetOverride(1588, ColumnsPatch{CastOnOther: strPtr(" yawns. (Turgur's Insects)")}); err != nil {
		t.Fatalf("SetOverride: %v", err)
	}

	// Simulate the server patch: take the (overridden) live content and
	// change an unrelated field on a different spell, as if the server
	// pushed new data — then write that directly to the live path,
	// bypassing the app (as an external process would).
	livePath, _ := svc.livePath()
	live, err := os.ReadFile(livePath)
	if err != nil {
		t.Fatalf("read live file: %v", err)
	}
	patched := applyOverrides(string(live), map[int]OverrideRow{
		207: {SpellID: 207, CastOnOther: strPtr("SERVER CHANGED THIS")},
	})
	if err := os.WriteFile(livePath, []byte(patched), 0o644); err != nil {
		t.Fatalf("simulate external patch: %v", err)
	}

	svc.MarkExternalChange(patched)
	if err := svc.ReapplyAll(); err != nil {
		t.Fatalf("ReapplyAll: %v", err)
	}

	final, err := os.ReadFile(livePath)
	if err != nil {
		t.Fatalf("read live file after reapply: %v", err)
	}
	turgurs := findSpellLine(string(final), 1588)
	if turgurs[idxCastOnOther] != " yawns. (Turgur's Insects)" {
		t.Fatalf("user override lost after reapply: %q", turgurs[idxCastOnOther])
	}
	divineAura := findSpellLine(string(final), 207)
	if divineAura[idxCastOnOther] != "SERVER CHANGED THIS" {
		t.Fatalf("server's change lost after reapply: %q", divineAura[idxCastOnOther])
	}

	st, err := svc.Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.PendingExternalChange {
		t.Fatal("expected pending external change to clear after ReapplyAll")
	}
}
