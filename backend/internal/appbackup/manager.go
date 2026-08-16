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

	"gopkg.in/yaml.v3"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
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

	// configPath is the on-disk path to config.yaml (~/.pq-companion/config.yaml).
	// Only a factory reset touches it; a data reset leaves it in place.
	configPath string

	// appVersion is the running app's version string, stamped into manifests.
	appVersion string
}

// New constructs a Manager. All paths are absolute; the caller resolves them
// from the same sources the rest of the app uses (os.UserHomeDir + exe-dir).
func New(userDBPath, backupsDirPath, appHome, configPath, appVersion string) *Manager {
	return &Manager{
		userDBPath:     userDBPath,
		backupsDirPath: backupsDirPath,
		appHome:        appHome,
		configPath:     configPath,
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

// pendingClientStatePath is where a staged import's client-state.json
// (Electron window/overlay state + renderer localStorage) is written.
// Consumed by Electron at its own next boot — before the backend even runs
// ApplyPendingImport — so it lives directly under appHome rather than in the
// staging dir the backend-owned swap uses.
func (m *Manager) pendingClientStatePath() string {
	return filepath.Join(m.appHome, "pending-client-state.json")
}

// Export writes the current app state to a .pqcb bundle at destination.
// clientState is an opaque JSON blob (Electron window/overlay state +
// renderer localStorage) supplied by the caller — the Go backend has no
// visibility into either, so it's collected renderer-side and passed through
// verbatim. May be nil/empty; older callers or a browser-preview export
// simply omit it.
//
// Returns the manifest that was written into the bundle and the final bundle
// path (which may differ from destination if it didn't already end in the
// bundle extension).
func (m *Manager) Export(destination string, clientState []byte) (string, *Manifest, error) {
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

	// Add config.yaml (settings — the whole Settings tab) if present. A
	// brand-new install may not have written one yet.
	if _, err := os.Stat(m.configPath); err == nil {
		fe, err := addFileToZip(zipWriter, m.configPath, configName)
		if err != nil {
			return "", nil, fmt.Errorf("write config.yaml into bundle: %w", err)
		}
		manifest.Files = append(manifest.Files, fe)
		manifest.Stats.ConfigIncluded = true
		manifest.Stats.TotalSizeBytes += fe.SizeBytes
	}

	// Add the Electron/renderer client state (window bounds, overlay layout,
	// localStorage prefs) if the caller supplied any.
	if len(clientState) > 0 {
		csSrc := filepath.Join(tmpDir, "client-state-src.json")
		if err := os.WriteFile(csSrc, clientState, 0o644); err != nil {
			return "", nil, fmt.Errorf("stage client state: %w", err)
		}
		fe, err := addFileToZip(zipWriter, csSrc, clientStateName)
		if err != nil {
			return "", nil, fmt.Errorf("write client-state.json into bundle: %w", err)
		}
		manifest.Files = append(manifest.Files, fe)
		manifest.Stats.ClientStateIncluded = true
		manifest.Stats.TotalSizeBytes += fe.SizeBytes
	}

	// Add custom sound files referenced by triggers/config, recording how to
	// remap each one back to a local path on import. Read from the db
	// snapshot (already a consistent copy) rather than the live user.db.
	soundPaths, err := collectSoundPaths(dbSnapshot, m.configPath)
	if err != nil {
		return "", nil, fmt.Errorf("collect sound paths: %w", err)
	}
	if len(soundPaths) > 0 {
		manifest.SoundMap = make(map[string]string, len(soundPaths))
	}
	for _, p := range soundPaths {
		nameInBundle := bundleSoundName(p)
		fe, err := addFileToZip(zipWriter, p, nameInBundle)
		if err != nil {
			return "", nil, fmt.Errorf("write sound %s into bundle: %w", filepath.Base(p), err)
		}
		manifest.Files = append(manifest.Files, fe)
		manifest.SoundMap[p] = nameInBundle
		manifest.Stats.SoundCount++
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
		switch f.Name {
		case manifestName:
			// Already parsed.
			continue
		case clientStateName:
			// Electron consumes this at its own next boot, before the
			// backend even runs ApplyPendingImport — write it straight to
			// the well-known pending path instead of the backend-owned
			// staging dir.
			if err := extractFileTo(f, m.pendingClientStatePath()); err != nil {
				return nil, fmt.Errorf("stage client state: %w", err)
			}
			continue
		}
		if err := extractEntry(f, staging); err != nil {
			return nil, fmt.Errorf("extract %s: %w", f.Name, err)
		}
	}

	// Re-write the manifest into staging too — ApplyPendingImport (running
	// after a restart, in a fresh Manager) needs SoundMap to know which
	// sound_path references to remap, and has no other way to recover it.
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshal manifest for staging: %w", err)
	}
	if err := os.WriteFile(filepath.Join(staging, manifestName), manifestBytes, 0o644); err != nil {
		return nil, fmt.Errorf("stage manifest: %w", err)
	}

	// Sentinel last, only after staging is fully written. Its presence is
	// the all-clear signal for ApplyPendingImport.
	if err := os.WriteFile(m.sentinelPath(), []byte(time.Now().UTC().Format(time.RFC3339)), 0o644); err != nil {
		_ = os.RemoveAll(staging)
		_ = os.Remove(m.pendingClientStatePath())
		return nil, fmt.Errorf("write sentinel: %w", err)
	}
	return manifest, nil
}

// CancelStagedImport removes any pending import without applying it. Used if
// the user reconsiders before restarting.
func (m *Manager) CancelStagedImport() error {
	_ = os.Remove(m.sentinelPath())
	_ = os.Remove(m.pendingClientStatePath())
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

	// Best-effort: the manifest (written into staging by StageImport) tells us
	// which sound files were bundled and their original paths, for the remap
	// pass below. A missing/corrupt manifest just means "no sounds to remap"
	// rather than aborting the whole import.
	var manifest Manifest
	if data, err := os.ReadFile(filepath.Join(staging, manifestName)); err == nil {
		_ = json.Unmarshal(data, &manifest)
	}

	if _, err := os.Stat(stagedDB); err != nil {
		// Staging is incomplete — abort and clear so we don't loop on next start.
		_ = m.CancelStagedImport()
		return false, fmt.Errorf("staged user.db missing: %w", err)
	}

	ts := time.Now().Format("20060102-150405")

	// Move existing user.db aside, then replace. Track the set-aside path so we
	// can put it back if the install fails — otherwise a rename failure here
	// leaves NO user.db at the expected path, startup silently creates a fresh
	// empty one, and the user's real data sits orphaned in the .preimport file
	// with nothing telling them.
	var asideDB string
	if _, err := os.Stat(m.userDBPath); err == nil {
		asideDB = m.userDBPath + "." + ts + ".preimport"
		if err := os.Rename(m.userDBPath, asideDB); err != nil {
			return false, fmt.Errorf("set aside existing user.db: %w", err)
		}
	}
	// Move any leftover -wal/-shm sidecars aside too. The previous process may
	// have been force-killed (Electron's relaunch path taskkills the sidecar
	// on Windows) without checkpointing, leaving WAL frames that belong to the
	// database we just moved aside. If left behind, SQLite would try to
	// replay them against the freshly installed user.db on next open,
	// corrupting it ("database disk image is malformed") — same hazard
	// ApplyPendingReset already guards against.
	for _, suffix := range []string{"-wal", "-shm"} {
		p := m.userDBPath + suffix
		if _, err := os.Stat(p); err == nil {
			_ = os.Rename(p, p+"."+ts+".preimport")
		}
	}
	if err := os.MkdirAll(filepath.Dir(m.userDBPath), 0o755); err != nil {
		if asideDB != "" {
			_ = os.Rename(asideDB, m.userDBPath) // restore original
		}
		return false, fmt.Errorf("ensure user.db parent dir: %w", err)
	}
	if err := os.Rename(stagedDB, m.userDBPath); err != nil {
		if asideDB != "" {
			_ = os.Rename(asideDB, m.userDBPath) // restore original
		}
		return false, fmt.Errorf("install staged user.db: %w", err)
	}

	// Move existing backups dir aside, then replace with the staged one.
	var asideBackups string
	if info, err := os.Stat(m.backupsDirPath); err == nil && info.IsDir() {
		asideBackups = m.backupsDirPath + "." + ts + ".preimport"
		if err := os.Rename(m.backupsDirPath, asideBackups); err != nil {
			return false, fmt.Errorf("set aside existing backups dir: %w", err)
		}
	}
	if err := os.MkdirAll(filepath.Dir(m.backupsDirPath), 0o755); err != nil {
		if asideBackups != "" {
			_ = os.Rename(asideBackups, m.backupsDirPath) // restore original
		}
		return false, fmt.Errorf("ensure backups dir parent: %w", err)
	}
	if err := os.Rename(stagedBackups, m.backupsDirPath); err != nil {
		if asideBackups != "" {
			_ = os.Rename(asideBackups, m.backupsDirPath) // restore original
		}
		return false, fmt.Errorf("install staged backups dir: %w", err)
	}

	// config.yaml: merge the imported settings over whatever's local (a fresh
	// install after a wipe has nothing local yet). Unlike user.db/backups this
	// is a merge, not a swap — a handful of fields point at local filesystem
	// paths (EQ install dir, Piper binary) that may not exist on this machine.
	// A bundle exported before v2 won't have one; nothing to do in that case.
	stagedConfig := filepath.Join(staging, configName)
	if _, err := os.Stat(stagedConfig); err == nil {
		importedMgr, err := config.LoadFrom(stagedConfig)
		if err != nil {
			return false, fmt.Errorf("read staged config.yaml: %w", err)
		}

		// nil local means a fresh install with no config.yaml yet — merge
		// treats that as "nothing to protect", e.g. the imported ServerAddr
		// is used as-is rather than falling back to a zero value. Read
		// before renaming aside, and only via LoadFrom (which side-effects a
		// defaults file into existence) when a file is actually there.
		var local *config.Config
		localExists := false
		if _, err := os.Stat(m.configPath); err == nil {
			localExists = true
			localMgr, err := config.LoadFrom(m.configPath)
			if err != nil {
				return false, fmt.Errorf("read existing config.yaml: %w", err)
			}
			c := localMgr.Get()
			local = &c
		}
		merged := mergeImportedConfig(importedMgr.Get(), local)

		if localExists {
			if err := os.Rename(m.configPath, m.configPath+"."+ts+".preimport"); err != nil {
				return false, fmt.Errorf("set aside existing config.yaml: %w", err)
			}
		}
		if err := config.WriteFile(m.configPath, merged); err != nil {
			return false, fmt.Errorf("install merged config.yaml: %w", err)
		}
	}

	// Custom sound files: copy staged sounds into appHome/sounds and rewrite
	// every sound_path reference (triggers table + config.yaml) that pointed
	// at the exporting machine's original path. Runs against the now-installed
	// user.db and config.yaml, so it's safe to attempt unconditionally — if
	// config.yaml wasn't part of this import, none of its sound_path values
	// will match a remap key and rewriteSoundPathsInConfig is a no-op.
	if len(manifest.SoundMap) > 0 {
		remap, err := copyStagedSounds(staging, filepath.Join(m.appHome, "sounds"), manifest.SoundMap)
		if err != nil {
			return false, fmt.Errorf("install sounds: %w", err)
		}
		if err := rewriteTriggerSoundPaths(m.userDBPath, remap); err != nil {
			return false, fmt.Errorf("remap trigger sound paths: %w", err)
		}
		if data, err := os.ReadFile(m.configPath); err == nil {
			var cfg config.Config
			if err := yaml.Unmarshal(data, &cfg); err == nil {
				rewritten, changed, err := rewriteSoundPathsInConfig(cfg, remap)
				if err == nil && changed {
					_ = config.WriteFile(m.configPath, rewritten)
				}
			}
		}
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

// maxBundleEntrySize bounds how many decompressed bytes a single .pqcb entry
// may expand to. A bundle only ever holds one user.db snapshot plus the
// EQ-config backup zips; several hundred MB is far beyond any real install,
// but the cap still stops a hostile/corrupted bundle from exhausting disk
// during staging (classic zip-bomb).
const maxBundleEntrySize = 1 << 30 // 1 GiB

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
	return extractFileTo(f, dest)
}

// extractFileTo writes a single (non-directory) zip entry's contents to an
// exact destination path, bypassing extractEntry's destRoot-relative path
// handling — used when the destination is already a known, trusted path
// (e.g. the pending-client-state.json well-known location) rather than one
// derived from the untrusted entry name.
func extractFileTo(f *zip.File, dest string) error {
	if f.UncompressedSize64 > maxBundleEntrySize {
		return fmt.Errorf("bundle entry %q is too large (%d bytes, max %d)", f.Name, f.UncompressedSize64, maxBundleEntrySize)
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
	// Copy one byte past the cap so an entry whose declared UncompressedSize64
	// understates its real output still gets caught rather than silently
	// filling disk.
	n, err := io.Copy(out, io.LimitReader(rc, maxBundleEntrySize+1))
	if err != nil {
		return err
	}
	if n > maxBundleEntrySize {
		return fmt.Errorf("bundle entry %q exceeded max size %d during extraction", f.Name, maxBundleEntrySize)
	}
	return nil
}
