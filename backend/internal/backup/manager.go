package backup

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
)

// Manager handles backup creation, restoration, and deletion.
// Backups are zip archives stored under <backupDir>/.
// Metadata is persisted in a Store backed by user.db.
type Manager struct {
	store     *Store
	cfgMgr    *config.Manager
	backupDir string
}

// NewManager opens (or creates) the user.db under ~/.pq-companion/ and
// returns a ready Manager.  Backup archives are stored in <exe_dir>/backups/.
func NewManager(cfgMgr *config.Manager) (*Manager, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("home dir: %w", err)
	}
	return NewManagerAt(cfgMgr, filepath.Join(home, ".pq-companion"), exeBackupDir())
}

// NewManagerAt is like NewManager but uses explicit directories.  baseDir is
// the root for user.db; backupDir is where zip archives are written.
// Intended for testing.
func NewManagerAt(cfgMgr *config.Manager, baseDir, backupDir string) (*Manager, error) {
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return nil, fmt.Errorf("create pq-companion dir: %w", err)
	}

	store, err := OpenStore(filepath.Join(baseDir, "user.db"))
	if err != nil {
		return nil, err
	}

	return &Manager{store: store, cfgMgr: cfgMgr, backupDir: backupDir}, nil
}

// exeBackupDir returns <exe_dir>/backups, falling back to "backups" for dev.
func exeBackupDir() string {
	exe, err := os.Executable()
	if err == nil {
		return filepath.Join(filepath.Dir(exe), "backups")
	}
	return "backups"
}

// Close releases the underlying database connection.
func (m *Manager) Close() error { return m.store.Close() }

// List returns all backups, newest first.
func (m *Manager) List() ([]*Backup, error) {
	return m.store.List()
}

// Get returns the backup with the given ID.
func (m *Manager) Get(id string) (*Backup, error) {
	return m.store.Get(id)
}

// Create zips all *.ini files found in the configured EQ directory and records
// the backup in user.db.  name and notes are free-form strings from the caller.
func (m *Manager) Create(name, notes string) (*Backup, error) {
	return m.create(name, notes, TriggerManual)
}

// create is the internal implementation; triggerReason is one of the Trigger* constants.
func (m *Manager) create(name, notes, triggerReason string) (*Backup, error) {
	eqPath := m.cfgMgr.Get().EQPath
	if eqPath == "" {
		return nil, errors.New("eq_path is not configured")
	}

	files, err := filepath.Glob(filepath.Join(eqPath, "*.ini"))
	if err != nil {
		return nil, fmt.Errorf("glob ini files: %w", err)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no *.ini files found in %s", eqPath)
	}

	backupDir := m.backupDir
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return nil, fmt.Errorf("create backups dir: %w", err)
	}

	id, err := newID()
	if err != nil {
		return nil, err
	}

	zipPath := filepath.Join(backupDir, id+".zip")
	sizeBytes, err := createZip(zipPath, files)
	if err != nil {
		return nil, fmt.Errorf("create zip: %w", err)
	}

	b := &Backup{
		ID:            id,
		Name:          name,
		Notes:         notes,
		CreatedAt:     time.Now().UTC(),
		SizeBytes:     sizeBytes,
		FileCount:     len(files),
		TriggerReason: triggerReason,
	}
	if err := m.store.Insert(b); err != nil {
		_ = os.Remove(zipPath)
		return nil, err
	}

	slog.Info("backup created", "id", id, "trigger", triggerReason, "files", len(files), "size", sizeBytes)

	// Enforce retention limit after each creation.
	cfg := m.cfgMgr.Get()
	if cfg.Backup.MaxBackups > 0 {
		if _, err := m.Prune(cfg.Backup.MaxBackups); err != nil {
			slog.Warn("prune after create failed", "err", err)
		}
	}
	return b, nil
}

// Lock marks a backup as protected from automatic cleanup.
func (m *Manager) Lock(id string) error {
	if _, err := m.store.Get(id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("backup %s: %w", id, ErrNotFound)
		}
		return err
	}
	return m.store.SetLocked(id, true)
}

// Unlock removes the protection flag from a backup.
func (m *Manager) Unlock(id string) error {
	if _, err := m.store.Get(id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("backup %s: %w", id, ErrNotFound)
		}
		return err
	}
	return m.store.SetLocked(id, false)
}

// Prune deletes the oldest unlocked backups so the total does not exceed
// maxBackups.  Returns the number of backups deleted.
func (m *Manager) Prune(maxBackups int) (int, error) {
	if maxBackups <= 0 {
		return 0, nil
	}
	total, err := m.store.Count()
	if err != nil {
		return 0, fmt.Errorf("count backups: %w", err)
	}
	excess := total - maxBackups
	if excess <= 0 {
		return 0, nil
	}
	ids, err := m.store.OldestUnlocked(excess)
	if err != nil {
		return 0, fmt.Errorf("oldest unlocked: %w", err)
	}
	deleted := 0
	for _, id := range ids {
		if err := m.Delete(id); err != nil {
			slog.Warn("prune: delete failed", "id", id, "err", err)
			continue
		}
		deleted++
	}
	if deleted > 0 {
		slog.Info("backup prune", "deleted", deleted, "max", maxBackups)
	}
	return deleted, nil
}

// StartWatcher monitors *.ini files in the configured EQ directory for
// modifications and creates an auto-backup whenever changes are detected.
// It polls every 60 seconds.  The caller must hold cfgMgr.AutoBackup = true.
func (m *Manager) StartWatcher(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	// Track last-seen modification times.
	lastMod := map[string]time.Time{}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cfg := m.cfgMgr.Get()
			if !cfg.Backup.AutoBackup || cfg.EQPath == "" {
				continue
			}
			files, _ := filepath.Glob(filepath.Join(cfg.EQPath, "*.ini"))
			changed := false
			for _, f := range files {
				info, err := os.Stat(f)
				if err != nil {
					continue
				}
				if prev, ok := lastMod[f]; !ok || info.ModTime().After(prev) {
					lastMod[f] = info.ModTime()
					if ok {
						changed = true
					}
				}
			}
			if changed {
				name := "Auto-backup " + time.Now().UTC().Format("2006-01-02 15:04")
				if _, err := m.create(name, "", TriggerAuto); err != nil {
					slog.Warn("auto-backup failed", "err", err)
				}
			}
		}
	}
}

// StartScheduler creates periodic backups according to the configured schedule
// ("hourly" or "daily").  Runs until ctx is cancelled.
func (m *Manager) StartScheduler(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	lastRun := time.Time{}

	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			cfg := m.cfgMgr.Get()
			var interval time.Duration
			switch cfg.Backup.Schedule {
			case "hourly":
				interval = time.Hour
			case "daily":
				interval = 24 * time.Hour
			default:
				lastRun = time.Time{} // reset so it fires immediately when re-enabled
				continue
			}
			if lastRun.IsZero() || now.Sub(lastRun) >= interval {
				lastRun = now
				name := "Scheduled " + now.UTC().Format("2006-01-02 15:04")
				if _, err := m.create(name, "", TriggerScheduled); err != nil {
					slog.Warn("scheduled backup failed", "err", err)
				}
			}
		}
	}
}

// Delete removes a backup's zip archive and its database record.
func (m *Manager) Delete(id string) error {
	if _, err := m.store.Get(id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("backup %s: %w", id, ErrNotFound)
		}
		return err
	}

	zipPath := filepath.Join(m.backupDir, id+".zip")
	if err := os.Remove(zipPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove zip: %w", err)
	}

	if err := m.store.Delete(id); err != nil {
		return err
	}
	slog.Info("backup deleted", "id", id)
	return nil
}

// Restore extracts the backup's zip archive into the configured EQ directory,
// overwriting any existing files with the same names.
func (m *Manager) Restore(id string) error {
	eqPath := m.cfgMgr.Get().EQPath
	if eqPath == "" {
		return errors.New("eq_path is not configured")
	}

	if _, err := m.store.Get(id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("backup %s: %w", id, ErrNotFound)
		}
		return err
	}

	zipPath := filepath.Join(m.backupDir, id+".zip")
	if err := extractZip(zipPath, eqPath); err != nil {
		return fmt.Errorf("restore zip: %w", err)
	}

	slog.Info("backup restored", "id", id, "eq_path", eqPath)
	return nil
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func newID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// createZip writes files into a new zip at dest and returns the zip's size in bytes.
func createZip(dest string, files []string) (int64, error) {
	f, err := os.Create(dest)
	if err != nil {
		return 0, err
	}
	defer f.Close()

	w := zip.NewWriter(f)
	for _, src := range files {
		if err := addFileToZip(w, src); err != nil {
			w.Close()
			return 0, err
		}
	}
	if err := w.Close(); err != nil {
		return 0, err
	}

	info, err := f.Stat()
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func addFileToZip(w *zip.Writer, src string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	hdr, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	hdr.Name = filepath.Base(src)
	hdr.Method = zip.Deflate

	out, err := w.CreateHeader(hdr)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	return err
}

// extractZip extracts all entries from src into destDir.
func extractZip(src, destDir string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		// Security: reject any path with a separator to prevent path traversal.
		if filepath.Base(f.Name) != f.Name {
			return fmt.Errorf("invalid entry name %q in zip", f.Name)
		}

		dest := filepath.Join(destDir, f.Name)
		if err := writeZipEntry(f, dest); err != nil {
			return fmt.Errorf("extract %s: %w", f.Name, err)
		}
	}
	return nil
}

func writeZipEntry(f *zip.File, dest string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, rc)
	return err
}
