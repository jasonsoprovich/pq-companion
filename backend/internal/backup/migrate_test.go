package backup

import (
	"os"
	"path/filepath"
	"testing"
)

// MigrateLegacyDir is tied to the executable's location via os.Executable.
// The tests here exercise the copyFile helper and confirm the package
// compiles a no-op when there's nothing to migrate; the end-to-end behavior
// is verified manually because rebinding the executable path in-test isn't
// portable.

func TestCopyFile_RoundTrips(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	src := filepath.Join(dir, "src.zip")
	dst := filepath.Join(dir, "dst.zip")
	want := []byte("pretend zip contents")
	if err := os.WriteFile(src, want, 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if string(got) != string(want) {
		t.Fatalf("contents differ: got %q want %q", got, want)
	}
	if _, err := os.Stat(src); err != nil {
		t.Fatalf("source must remain after copyFile: %v", err)
	}
}

func TestCopyFile_DestExistsIsOverwritten(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	src := filepath.Join(dir, "src.zip")
	dst := filepath.Join(dir, "dst.zip")
	if err := os.WriteFile(src, []byte("new"), 0o644); err != nil {
		t.Fatalf("write src: %v", err)
	}
	if err := os.WriteFile(dst, []byte("stale"), 0o644); err != nil {
		t.Fatalf("write dst: %v", err)
	}
	if err := copyFile(src, dst); err != nil {
		t.Fatalf("copyFile: %v", err)
	}
	got, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read dst: %v", err)
	}
	if string(got) != "new" {
		t.Fatalf("dst not overwritten: got %q", got)
	}
}

func TestMigrateLegacyDir_NoLegacyDirIsNoop(t *testing.T) {
	t.Parallel()
	newDir := filepath.Join(t.TempDir(), "backups")
	// Should not panic, should not create newDir, should not log a warning.
	MigrateLegacyDir(newDir)
	if _, err := os.Stat(newDir); !os.IsNotExist(err) {
		t.Fatalf("newDir should not exist when there's nothing to migrate: %v", err)
	}
}
