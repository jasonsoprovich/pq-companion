package appbackup

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Reset modes. A reset is applied at the next startup (like an import) because
// the live user.db — and, for a factory reset, config.yaml — can't be moved
// aside while the running process still holds them open.
const (
	// ResetModeData wipes all app data (user.db + the EQ-config backups dir)
	// but keeps config.yaml, so the EQ folder path and preferences survive and
	// the user is not sent back through onboarding.
	ResetModeData = "data"
	// ResetModeFactory additionally moves config.yaml aside, so the app reopens
	// to the onboarding wizard as if freshly installed.
	ResetModeFactory = "factory"
)

// resetSentinelPath is the marker the backend checks at startup to know a reset
// is pending. Its contents are the reset mode string.
func (m *Manager) resetSentinelPath() string {
	return filepath.Join(m.appHome, ".reset-pending")
}

// StageReset writes the reset sentinel with the given mode. The wipe itself
// happens at next startup via ApplyPendingReset — nothing is deleted here.
func (m *Manager) StageReset(mode string) error {
	if mode != ResetModeData && mode != ResetModeFactory {
		return fmt.Errorf("unknown reset mode %q", mode)
	}
	if err := os.MkdirAll(m.appHome, 0o755); err != nil {
		return fmt.Errorf("ensure app home: %w", err)
	}
	if err := os.WriteFile(m.resetSentinelPath(), []byte(mode), 0o644); err != nil {
		return fmt.Errorf("write reset sentinel: %w", err)
	}
	return nil
}

// CancelStagedReset removes a pending reset before the restart applies it.
func (m *Manager) CancelStagedReset() error {
	if err := os.Remove(m.resetSentinelPath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// HasPendingReset reports whether a reset sentinel is present and, if so, its
// mode. An unrecognized mode is treated as no pending reset (defensive against
// a hand-edited or truncated sentinel).
func (m *Manager) HasPendingReset() (bool, string) {
	b, err := os.ReadFile(m.resetSentinelPath())
	if err != nil {
		return false, ""
	}
	mode := strings.TrimSpace(string(b))
	if mode != ResetModeData && mode != ResetModeFactory {
		return false, ""
	}
	return true, mode
}

// ApplyPendingReset is invoked at backend startup BEFORE config is loaded and
// BEFORE any user.db connection opens. If a reset sentinel is present it moves
// the relevant files aside with a timestamped ".prereset" suffix so the next
// steps of startup recreate them fresh (an empty user.db, and for a factory
// reset a default config that re-triggers onboarding). The set-aside files are
// left on disk for recovery, mirroring the import swap's ".preimport" copies.
//
// Returns the mode that was applied ("" if nothing was pending). The sentinel
// is always cleared on the way out so a failure can't loop on every launch.
func (m *Manager) ApplyPendingReset() (string, error) {
	ok, mode := m.HasPendingReset()
	if !ok {
		return "", nil
	}
	// Clear the sentinel up front: whether the moves below succeed or fail, we
	// must not re-run this on the next launch.
	defer func() { _ = m.CancelStagedReset() }()

	ts := time.Now().Format("20060102-150405")

	// user.db (plus its -wal/-shm sidecars, so a leftover WAL can't attach to
	// the freshly created database) and the EQ-config backups dir are wiped in
	// both modes — they're all "app data".
	if err := moveAsidePreReset(m.userDBPath, ts); err != nil {
		return "", fmt.Errorf("reset user.db: %w", err)
	}
	_ = moveAsidePreReset(m.userDBPath+"-wal", ts)
	_ = moveAsidePreReset(m.userDBPath+"-shm", ts)
	if err := moveAsidePreReset(m.backupsDirPath, ts); err != nil {
		return "", fmt.Errorf("reset backups dir: %w", err)
	}

	if mode == ResetModeFactory {
		if err := moveAsidePreReset(m.configPath, ts); err != nil {
			return "", fmt.Errorf("reset config.yaml: %w", err)
		}
	}
	return mode, nil
}

// moveAsidePreReset renames path to path.<ts>.prereset if it exists. A missing
// path is a no-op (nothing to wipe). Works for both files and directories.
func moveAsidePreReset(path, ts string) error {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return os.Rename(path, path+"."+ts+".prereset")
}
