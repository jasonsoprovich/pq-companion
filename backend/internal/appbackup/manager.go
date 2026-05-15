package appbackup

import (
	"archive/zip"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Manager handles export of the current app state into a .pqcb bundle and
// staging of an import bundle for the next startup to apply.
//
// Export reads the live user.db via `VACUUM INTO` (consistent snapshot under
// WAL mode) plus every zip in the EQ-config backups directory, and writes
// the result as a single zip with the .pqcb extension.
//
// Import does not swap files in place — it stages them in <home>/.pq-companion/
// import-staging/ and drops a sentinel. The next backend startup sees the
// sentinel and atomically swaps before any user.db connections open.
type Manager struct {
	// userDBPath is the on-disk path to the live user.db (e.g.
	// ~/.pq-companion/user.db).
	userDBPath string

	// backupsDirPath is the on-disk path to <exe_dir>/backups where the EQ-config
	// Backup Manager keeps its zips.
	backupsDirPath string

	// appHome is the per-user app dir (~/.pq-companion). Staging files and
	// the import sentinel live under it.
	appHome string

	// appVersion is the running app's version string, stamped into manifests.
	appVersion string
}

// New constructs a Manager. All paths are absolute; the caller resolves them
// from the same sources the rest of the app uses (os.UserHomeDir + exe-dir).
func New(userDBPath, backupsDirPath, appHome, appVersion string) *Manager {
	return &Manager{
		userDBPath:     userDBPath,
		backupsDirPath: backupsDirPath,
		appHome:        appHome,
		appVersion:     appVersion,
	}
}

// stagingDir is where an in-progress import lands its files before the next
// startup applies them.
func (m *Manager) stagingDir() string {
	return filepath.Join(m.appHome, "import-staging")
}

// sentinelPath is the marker file the backend checks at startup to know an
// import is pending.
func (m *Manager) sentinelPath() string {
	return filepath.Join(m.appHome, ".import-pending")
}

// Export writes the current app state to a .pqcb bundle at destination.
// Returns the manifest that was written into the bundle and the final bundle
// path (which may differ from destination if it didn't already end in the
// bundle extension).
func (m *Manager) Export(destination string) (string, *Manifest, error) {
	if !strings.HasSuffix(strings.ToLower(destination), BundleExt) {
		destination += BundleExt
	}
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return "", nil, fmt.Errorf("create destination dir: %w", err)
	}

	// VACUUM INTO a temp file so the live DB stays consistent under WAL.
	// Writing the snapshot inside the bundle directly isn't possible — the
	// zip Writer is sequential and SQLite needs random-access on the
	// destination file.
	tmpDir, err := os.MkdirTemp("", "pqcb-export-")
	if err != nil {
		return "", nil, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	dbSnapshot := filepath.Join(tmpDir, userDBName)
	if err := vacuumInto(m.userDBPath, dbSnapshot); err != nil {
		return "", nil, fmt.Errorf("snapshot user.db: %w", err)
	}

	// Discover backup zips.
	backupZips, err := listBackupZips(m.backupsDirPath)
	if err != nil {
		return "", nil, fmt.Errorf("list backup zips: %w", err)
	}

	// Write bundle atomically: build to <dest>.tmp then rename. Renaming over
	// the destination is a single OS operation, so a partially-written bundle
	// never appears at the destination path.
	tmpBundle := destination + ".tmp"
	bundle, err := os.Create(tmpBundle)
	if err != nil {
		return "", nil, fmt.Errorf("create bundle: %w", err)
	}
	zipWriter := zip.NewWriter(bundle)
	closed := false
	cleanup := func() {
		if !closed {
			_ = zipWriter.Close()
			_ = bundle.Close()
			_ = os.Remove(tmpBundle)
		}
	}
	defer cleanup()

	manifest := &Manifest{
		FormatVersion: FormatVersion,
		AppVersion:    m.appVersion,
		ExportedAt:    time.Now().UTC().Format(time.RFC3339),
		Files:         []FileEntry{},
	}

	// Add the db snapshot.
	entry, err := addFileToZip(zipWriter, dbSnapshot, userDBName)
	if err != nil {
		return "", nil, fmt.Errorf("write user.db into bundle: %w", err)
	}
	manifest.Files = append(manifest.Files, entry)
	manifest.Stats.TotalSizeBytes += entry.SizeBytes

	// Add every backup zip under backups/.
	for _, zipPath := range backupZips {
		nameInBundle := backupsDir + "/" + filepath.Base(zipPath)
		fe, err := addFileToZip(zipWriter, zipPath, nameInBundle)
		if err != nil {
			return "", nil, fmt.Errorf("write %s into bundle: %w", filepath.Base(zipPath), err)
		}
		manifest.Files = append(manifest.Files, fe)
		manifest.Stats.BackupCount++
		manifest.Stats.TotalSizeBytes += fe.SizeBytes
	}

	// Manifest last so it can describe everything.
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", nil, fmt.Errorf("marshal manifest: %w", err)
	}
	mw, err := zipWriter.Create(manifestName)
	if err != nil {
		return "", nil, fmt.Errorf("create manifest in bundle: %w", err)
	}
	if _, err := mw.Write(manifestBytes); err != nil {
		return "", nil, fmt.Errorf("write manifest: %w", err)
	}

	if err := zipWriter.Close(); err != nil {
		return "", nil, fmt.Errorf("close bundle zip: %w", err)
	}
	if err := bundle.Close(); err != nil {
		return "", nil, fmt.Errorf("close bundle file: %w", err)
	}
	closed = true

	if err := os.Rename(tmpBundle, destination); err != nil {
		_ = os.Remove(tmpBundle)
		return "", nil, fmt.Errorf("finalize bundle: %w", err)
	}
	return destination, manifest, nil
}

// PreviewImport reads the manifest from a bundle without applying anything.
// Used by the import-confirmation UI.
func (m *Manager) PreviewImport(bundlePath string) (*Manifest, error) {
	reader, err := zip.OpenReader(bundlePath)
	if err != nil {
		return nil, fmt.Errorf("open bundle: %w", err)
	}
	defer reader.Close()
	return readManifest(&reader.Reader)
}

// StageImport extracts a bundle into the staging dir under appHome and writes
// the sentinel. It does NOT swap files in place — the actual swap happens at
// next backend startup via ApplyPendingImport.
//
// Returns the manifest that was extracted so the caller can display it on the
// "restart required" confirmation.
func (m *Manager) StageImport(bundlePath string) (*Manifest, error) {
	reader, err := zip.OpenReader(bundlePath)
	if err != nil {
		return nil, fmt.Errorf("open bundle: %w", err)
	}
	defer reader.Close()

	manifest, err := readManifest(&reader.Reader)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	if manifest.FormatVersion > FormatVersion {
		return nil, fmt.Errorf("bundle format version %d exceeds supported %d — upgrade the app first",
			manifest.FormatVersion, FormatVersion)
	}

	staging := m.stagingDir()
	if err := os.RemoveAll(staging); err != nil {
		return nil, fmt.Errorf("clear staging dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(staging, backupsDir), 0o755); err != nil {
		return nil, fmt.Errorf("create staging dirs: %w", err)
	}

	for _, f := range reader.File {
		// Skip manifest — already parsed.
		if f.Name == manifestName {
			continue
		}
		if err := extractEntry(f, staging); err != nil {
			return nil, fmt.Errorf("extract %s: %w", f.Name, err)
		}
	}

	// Sentinel last, only after staging is fully written. Its presence is
	// the all-clear signal for ApplyPendingImport.
	if err := os.WriteFile(m.sentinelPath(), []byte(time.Now().UTC().Format(time.RFC3339)), 0o644); err != nil {
		_ = os.RemoveAll(staging)
		return nil, fmt.Errorf("write sentinel: %w", err)
	}
	return manifest, nil
}

// CancelStagedImport removes any pending import without applying it. Used if
// the user reconsiders before restarting.
func (m *Manager) CancelStagedImport() error {
	_ = os.Remove(m.sentinelPath())
	if err := os.RemoveAll(m.stagingDir()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// HasPendingImport reports whether a sentinel + staging dir are present.
func (m *Manager) HasPendingImport() bool {
	if _, err := os.Stat(m.sentinelPath()); err != nil {
		return false
	}
	if info, err := os.Stat(m.stagingDir()); err != nil || !info.IsDir() {
		return false
	}
	return true
}

// ApplyPendingImport is invoked at backend startup BEFORE any user.db
// connections are opened. If a sentinel is present, the staged files are
// swapped into place. The previous user.db and backups dir are renamed aside
// with a timestamp suffix so the user can recover if anything went wrong.
//
// Returns true if a swap was applied (the caller can log it), false if there
// was nothing to do.
func (m *Manager) ApplyPendingImport() (bool, error) {
	if !m.HasPendingImport() {
		return false, nil
	}
	staging := m.stagingDir()
	stagedDB := filepath.Join(staging, userDBName)
	stagedBackups := filepath.Join(staging, backupsDir)

	if _, err := os.Stat(stagedDB); err != nil {
		// Staging is incomplete — abort and clear so we don't loop on next start.
		_ = m.CancelStagedImport()
		return false, fmt.Errorf("staged user.db missing: %w", err)
	}

	ts := time.Now().Format("20060102-150405")

	// Move existing user.db aside, then replace.
	if _, err := os.Stat(m.userDBPath); err == nil {
		aside := m.userDBPath + "." + ts + ".preimport"
		if err := os.Rename(m.userDBPath, aside); err != nil {
			return false, fmt.Errorf("set aside existing user.db: %w", err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(m.userDBPath), 0o755); err != nil {
		return false, fmt.Errorf("ensure user.db parent dir: %w", err)
	}
	if err := os.Rename(stagedDB, m.userDBPath); err != nil {
		return false, fmt.Errorf("install staged user.db: %w", err)
	}

	// Move existing backups dir aside, then replace with the staged one.
	if info, err := os.Stat(m.backupsDirPath); err == nil && info.IsDir() {
		aside := m.backupsDirPath + "." + ts + ".preimport"
		if err := os.Rename(m.backupsDirPath, aside); err != nil {
			return false, fmt.Errorf("set aside existing backups dir: %w", err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(m.backupsDirPath), 0o755); err != nil {
		return false, fmt.Errorf("ensure backups dir parent: %w", err)
	}
	if err := os.Rename(stagedBackups, m.backupsDirPath); err != nil {
		return false, fmt.Errorf("install staged backups dir: %w", err)
	}

	// Cleanup staging + sentinel.
	_ = os.Remove(m.sentinelPath())
	_ = os.RemoveAll(staging)
	return true, nil
}

// --- helpers ---------------------------------------------------------------

// vacuumInto runs SQLite's `VACUUM INTO` against the live user.db, producing
// a consistent snapshot at dest. Uses a short-lived connection — never
// touches whatever long-lived connections the rest of the app holds.
func vacuumInto(srcPath, destPath string) error {
	// `?_pragma=journal_mode(WAL)&_pragma=busy_timeout(30000)` matches how
	// the other stores open user.db.
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(30000)", srcPath)
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := conn.Ping(); err != nil {
		return err
	}
	// Escape single quotes in path for the SQL literal. VACUUM INTO takes a
	// literal, not a bound parameter.
	escaped := strings.ReplaceAll(destPath, "'", "''")
	if _, err := conn.Exec("VACUUM INTO '" + escaped + "'"); err != nil {
		return err
	}
	return nil
}

// listBackupZips returns absolute paths of every *.zip in the backups dir,
// sorted alphabetically. A missing directory is not an error — fresh installs
// don't have any backups yet.
func listBackupZips(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if !strings.EqualFold(filepath.Ext(e.Name()), ".zip") {
			continue
		}
		out = append(out, filepath.Join(dir, e.Name()))
	}
	sort.Strings(out)
	return out, nil
}

// addFileToZip copies src into the zip writer at the given archive name,
// returning a FileEntry with size + sha256.
func addFileToZip(zw *zip.Writer, src, nameInZip string) (FileEntry, error) {
	in, err := os.Open(src)
	if err != nil {
		return FileEntry{}, err
	}
	defer in.Close()

	w, err := zw.Create(nameInZip)
	if err != nil {
		return FileEntry{}, err
	}

	hasher := sha256.New()
	mw := io.MultiWriter(w, hasher)
	n, err := io.Copy(mw, in)
	if err != nil {
		return FileEntry{}, err
	}
	return FileEntry{
		Name:      nameInZip,
		SizeBytes: n,
		SHA256:    hex.EncodeToString(hasher.Sum(nil)),
	}, nil
}

// readManifest extracts and decodes manifest.json from an open zip reader.
func readManifest(reader *zip.Reader) (*Manifest, error) {
	for _, f := range reader.File {
		if f.Name != manifestName {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		var m Manifest
		if err := json.NewDecoder(rc).Decode(&m); err != nil {
			return nil, fmt.Errorf("decode manifest: %w", err)
		}
		return &m, nil
	}
	return nil, fmt.Errorf("manifest.json missing — not a valid %s bundle", BundleExt)
}

// extractEntry writes a single zip entry under destRoot, refusing any path
// traversal attempts (e.g. "../../etc/passwd").
func extractEntry(f *zip.File, destRoot string) error {
	// Reject absolute paths and any segment that walks upward.
	clean := filepath.Clean(f.Name)
	if strings.HasPrefix(clean, "..") || strings.Contains(clean, string(filepath.Separator)+"..") || filepath.IsAbs(clean) {
		return fmt.Errorf("unsafe bundle entry %q", f.Name)
	}
	dest := filepath.Join(destRoot, clean)
	if f.FileInfo().IsDir() {
		return os.MkdirAll(dest, 0o755)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
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
	_, err = io.Copy(out, rc)
	return err
}
