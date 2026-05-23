package backup

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// MigrateLegacyDir moves any .zip backups from the legacy <exe_dir>/backups
// location into newDir. Older installs wrote backups next to the sidecar
// executable, which broke under C:\Program Files (no write access for
// standard accounts); see DefaultBackupDir.
//
// The migration tries os.Rename first. If that fails — typically because
// the legacy dir lives in a non-writable location (Program Files) but was
// populated when the user ran the app as Administrator — it falls back to
// copying the file and leaving the original behind. Existing files in
// newDir are never overwritten.
//
// Best-effort: errors are logged and skipped, never returned. Called once
// at startup; safe to invoke when the legacy dir doesn't exist.
func MigrateLegacyDir(newDir string) {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	legacy := filepath.Join(filepath.Dir(exe), "backups")
	if legacy == newDir {
		return
	}
	info, err := os.Stat(legacy)
	if err != nil || !info.IsDir() {
		return
	}
	entries, err := os.ReadDir(legacy)
	if err != nil {
		slog.Warn("legacy backups: read dir failed", "dir", legacy, "err", err)
		return
	}
	if err := os.MkdirAll(newDir, 0o755); err != nil {
		slog.Warn("legacy backups: create new dir failed", "dir", newDir, "err", err)
		return
	}
	var moved, copied int
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".zip") {
			continue
		}
		src := filepath.Join(legacy, e.Name())
		dst := filepath.Join(newDir, e.Name())
		if _, err := os.Stat(dst); err == nil {
			continue
		}
		if err := os.Rename(src, dst); err == nil {
			moved++
			continue
		}
		if err := copyFile(src, dst); err != nil {
			slog.Warn("legacy backups: copy failed", "src", src, "err", err)
			continue
		}
		copied++
	}
	if moved+copied > 0 {
		slog.Info("legacy backups migrated", "from", legacy, "to", newDir, "moved", moved, "copied", copied)
	}
	// Best-effort cleanup; will fail silently when the legacy dir is
	// non-empty or non-writable (Program Files).
	_ = os.Remove(legacy)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(dst)
		return fmt.Errorf("copy: %w", err)
	}
	return out.Close()
}
