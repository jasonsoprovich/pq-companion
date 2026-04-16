package backup

import (
	"archive/zip"
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
		ID:        id,
		Name:      name,
		Notes:     notes,
		CreatedAt: time.Now().UTC(),
		SizeBytes: sizeBytes,
		FileCount: len(files),
	}
	if err := m.store.Insert(b); err != nil {
		// Best-effort cleanup of the zip if the DB write fails.
		_ = os.Remove(zipPath)
		return nil, err
	}

	slog.Info("backup created", "id", id, "files", len(files), "size", sizeBytes)
	return b, nil
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
