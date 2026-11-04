package backup

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
)

// DefaultBackupDir returns the user-home backups directory
// (~/.pq-companion/backups). Exported so the server entrypoint can reuse the
// same path for the app-export/import manager without duplicating the
// "user home + .pq-companion + backups" join.
//
// Backups historically lived in <exe_dir>/backups, but that broke for users
// who installed under C:\Program Files (no write access for standard
// accounts). The path was moved into the user's home directory so it's
// always writable. The legacy location is migrated on startup; see
// MigrateLegacyDir.
func DefaultBackupDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, ".pq-companion", "backups"), nil
}

// Manager handles backup creation, restoration, and deletion.
// Backups are zip archives stored under <backupDir>/.
// Metadata is persisted in a Store backed by user.db.
type Manager struct {
	store     *Store
	cfgMgr    *config.Manager
	backupDir string

	// historyMu serialises file-history capture passes; historySeen holds
	// each file's mod time and size at its last pass, to skip unchanged ones.
	historyMu   sync.Mutex
	historySeen map[string]statKey
}

// NewManager opens (or creates) the user.db under ~/.pq-companion/ and
// returns a ready Manager. Backup archives are stored in
// ~/.pq-companion/backups/ — see DefaultBackupDir for the rationale.
func NewManager(cfgMgr *config.Manager) (*Manager, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("home dir: %w", err)
	}
	backupDir, err := DefaultBackupDir()
	if err != nil {
		return nil, err
	}
	return NewManagerAt(cfgMgr, filepath.Join(home, ".pq-companion"), backupDir)
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

	return &Manager{
		store:       store,
		cfgMgr:      cfgMgr,
		backupDir:   backupDir,
		historySeen: map[string]statKey{},
	}, nil
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
	return m.create(name, notes, TriggerManual, false)
}

// CreateWithLogs is Create that also copies the EverQuest log files
// (eqlog_*_pq.proj.txt). Only manual snapshots take logs: they're large and
// change constantly, so automatic ones would fill the disk.
func (m *Manager) CreateWithLogs(name, notes string) (*Backup, error) {
	return m.create(name, notes, TriggerManual, true)
}

// create is the internal implementation; triggerReason is one of the Trigger* constants.
func (m *Manager) create(name, notes, triggerReason string, includeLogs bool) (*Backup, error) {
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

	// Automatic snapshots skip a folder that hasn't changed since the last
	// snapshot, so the retention limit isn't spent on identical copies.
	hash, err := hashFiles(files)
	if err != nil {
		return nil, fmt.Errorf("hash ini files: %w", err)
	}
	if triggerReason == TriggerAuto || triggerReason == TriggerScheduled {
		if last, err := m.store.LatestHash(); err == nil && last == hash {
			slog.Debug("backup skipped: no changes since the last one", "trigger", triggerReason)
			return nil, nil
		}
	}
	if includeLogs {
		logs, err := filepath.Glob(filepath.Join(eqPath, "eqlog_*_pq.proj.txt"))
		if err != nil {
			return nil, fmt.Errorf("glob log files: %w", err)
		}
		files = append(files, logs...)
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
		// A failed createZip can leave a partial zip with no DB row —
		// invisible to List/Prune/Delete yet still globbed by the app-backup
		// exporter. Remove it, mirroring the store.Insert failure path below.
		_ = os.Remove(zipPath)
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
		ContentHash:   hash,
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
				if _, err := m.create(name, "", TriggerAuto, false); err != nil {
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
				if _, err := m.create(name, "", TriggerScheduled, false); err != nil {
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

// BackupFile is one file inside a snapshot.
type BackupFile struct {
	Name      string    `json:"name"`
	SizeBytes int64     `json:"size_bytes"`
	Modified  time.Time `json:"modified"`
	// Kind and Character classify settings files (see ClassifyFile); Kind is
	// "log" for log files and "" for anything else.
	Kind      string `json:"kind"`
	Character string `json:"character"`
}

var reLogName = regexp.MustCompile(`(?i)^eqlog_([a-z]+)_pq\.proj\.txt$`)

// Files lists the files in a snapshot.
func (m *Manager) Files(id string) ([]BackupFile, error) {
	if _, err := m.store.Get(id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, fmt.Errorf("backup %s: %w", id, ErrNotFound)
		}
		return nil, err
	}
	r, err := zip.OpenReader(filepath.Join(m.backupDir, id+".zip"))
	if err != nil {
		return nil, fmt.Errorf("open backup: %w", err)
	}
	defer r.Close()
	out := make([]BackupFile, 0, len(r.File))
	for _, f := range r.File {
		bf := BackupFile{Name: f.Name, SizeBytes: int64(f.UncompressedSize64), Modified: f.Modified.UTC()}
		if kind, char, ok := ClassifyFile(f.Name); ok {
			bf.Kind, bf.Character = kind, char
		} else if mm := reLogName.FindStringSubmatch(f.Name); mm != nil {
			bf.Kind, bf.Character = "log", capitalize(mm[1])
		}
		out = append(out, bf)
	}
	return out, nil
}

// Restore extracts the backup's zip archive into the configured EQ directory,
// overwriting any existing files with the same names.
func (m *Manager) Restore(id string) error {
	return m.RestoreFiles(id, nil)
}

// RestoreFiles extracts the named files from a backup into the EQ directory
// (every file when names is empty). Tracked settings files are saved to file
// history first, so the restore can be undone file by file.
func (m *Manager) RestoreFiles(id string, names []string) error {
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

	if _, err := m.CaptureFiles(true); err != nil {
		slog.Warn("restore: saving current files to history failed", "err", err)
	}

	var only map[string]bool
	if len(names) > 0 {
		only = make(map[string]bool, len(names))
		for _, n := range names {
			only[n] = true
		}
	}
	zipPath := filepath.Join(m.backupDir, id+".zip")
	if err := extractZip(zipPath, eqPath, only); err != nil {
		return fmt.Errorf("restore zip: %w", err)
	}

	slog.Info("backup restored", "id", id, "eq_path", eqPath, "files", len(names))
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

// hashFiles is a sha256 over the files' names and contents, in name order.
func hashFiles(paths []string) (string, error) {
	sorted := append([]string(nil), paths...)
	sort.Strings(sorted)
	h := sha256.New()
	for _, p := range sorted {
		f, err := os.Open(p)
		if err != nil {
			return "", err
		}
		io.WriteString(h, strings.ToLower(filepath.Base(p))+"\x00")
		_, err = io.Copy(h, f)
		f.Close()
		if err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// extractZip extracts the entries of src into destDir: all of them when only
// is nil, otherwise the named ones.
func extractZip(src, destDir string, only map[string]bool) error {
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
		if only != nil && !only[f.Name] {
			continue
		}

		dest := filepath.Join(destDir, f.Name)
		if err := writeZipEntry(f, dest); err != nil {
			return fmt.Errorf("extract %s: %w", f.Name, err)
		}
	}
	return nil
}

// maxRestoreEntrySize bounds how many decompressed bytes a single backup-zip
// entry may expand to. These zips only ever hold EQ .ini files (KB-scale);
// the cap is generous headroom while still stopping a corrupted/tampered
// backup zip from filling disk on restore.
const maxRestoreEntrySize = 256 << 20 // 256 MiB

func writeZipEntry(f *zip.File, dest string) error {
	if f.UncompressedSize64 > maxRestoreEntrySize {
		return fmt.Errorf("entry %q is too large (%d bytes, max %d)", f.Name, f.UncompressedSize64, maxRestoreEntrySize)
	}
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

	n, err := io.Copy(out, io.LimitReader(rc, maxRestoreEntrySize+1))
	if err != nil {
		return err
	}
	if n > maxRestoreEntrySize {
		return fmt.Errorf("entry %q exceeded max size %d during extraction", f.Name, maxRestoreEntrySize)
	}
	return nil
}
