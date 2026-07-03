package appbackup

import (
	"os"
	"path/filepath"
	"testing"
)

// newTestManager builds a Manager rooted at a temp appHome with user.db,
// backups/, and config.yaml laid out the way the real app arranges them.
func newTestManager(t *testing.T) (*Manager, string) {
	t.Helper()
	home := t.TempDir()
	m := New(
		filepath.Join(home, "user.db"),
		filepath.Join(home, "backups"),
		home,
		filepath.Join(home, "config.yaml"),
		"test",
	)
	return m, home
}

func seedAppFiles(t *testing.T, home string) {
	t.Helper()
	mustWrite(t, filepath.Join(home, "user.db"), "db")
	mustWrite(t, filepath.Join(home, "user.db-wal"), "wal")
	mustWrite(t, filepath.Join(home, "user.db-shm"), "shm")
	mustWrite(t, filepath.Join(home, "config.yaml"), "cfg")
	if err := os.MkdirAll(filepath.Join(home, "backups"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(home, "backups", "b1.zip"), "zip")
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestDataResetKeepsConfig(t *testing.T) {
	m, home := newTestManager(t)
	seedAppFiles(t, home)

	if err := m.StageReset(ResetModeData); err != nil {
		t.Fatal(err)
	}
	if pending, mode := m.HasPendingReset(); !pending || mode != ResetModeData {
		t.Fatalf("expected pending data reset, got pending=%v mode=%q", pending, mode)
	}

	mode, err := m.ApplyPendingReset()
	if err != nil {
		t.Fatal(err)
	}
	if mode != ResetModeData {
		t.Fatalf("expected mode data, got %q", mode)
	}
	// user.db + sidecars + backups gone from their live paths...
	for _, p := range []string{"user.db", "user.db-wal", "user.db-shm", "backups"} {
		if exists(filepath.Join(home, p)) {
			t.Errorf("%s should have been moved aside", p)
		}
	}
	// ...but config.yaml stays put.
	if !exists(filepath.Join(home, "config.yaml")) {
		t.Error("config.yaml must survive a data reset")
	}
	// Sentinel cleared so it doesn't loop next launch.
	if pending, _ := m.HasPendingReset(); pending {
		t.Error("sentinel should be cleared after apply")
	}
	// Set-aside copy exists for recovery.
	if len(preresetFiles(t, home)) == 0 {
		t.Error("expected .prereset recovery files")
	}
}

func TestFactoryResetMovesConfigAside(t *testing.T) {
	m, home := newTestManager(t)
	seedAppFiles(t, home)

	if err := m.StageReset(ResetModeFactory); err != nil {
		t.Fatal(err)
	}
	if _, err := m.ApplyPendingReset(); err != nil {
		t.Fatal(err)
	}
	if exists(filepath.Join(home, "config.yaml")) {
		t.Error("config.yaml must be moved aside by a factory reset")
	}
}

func TestApplyResetNoSentinelIsNoop(t *testing.T) {
	m, home := newTestManager(t)
	seedAppFiles(t, home)

	mode, err := m.ApplyPendingReset()
	if err != nil {
		t.Fatal(err)
	}
	if mode != "" {
		t.Fatalf("expected no-op, got mode %q", mode)
	}
	if !exists(filepath.Join(home, "user.db")) {
		t.Error("user.db must be untouched when nothing is staged")
	}
}

func TestStageResetRejectsUnknownMode(t *testing.T) {
	m, _ := newTestManager(t)
	if err := m.StageReset("nuke"); err == nil {
		t.Fatal("expected error for unknown mode")
	}
}

func TestCancelStagedReset(t *testing.T) {
	m, _ := newTestManager(t)
	if err := m.StageReset(ResetModeData); err != nil {
		t.Fatal(err)
	}
	if err := m.CancelStagedReset(); err != nil {
		t.Fatal(err)
	}
	if pending, _ := m.HasPendingReset(); pending {
		t.Error("reset should not be pending after cancel")
	}
}

func preresetFiles(t *testing.T, home string) []string {
	t.Helper()
	entries, err := os.ReadDir(home)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".prereset" {
			out = append(out, e.Name())
		}
	}
	return out
}
