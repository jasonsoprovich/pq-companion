package backup

import (
	"bytes"
	"compress/gzip"
	"context"
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
	"time"
)

// File history keeps past versions of individual EQ settings files, per
// character: spellsets, bandolier, hotkeys and social macros, UI layout and
// so on. Unlike snapshots (a zip of every .ini at one moment, with a global
// count limit), each file keeps its own last N versions, so a spellset that
// changes twice a year isn't pushed out by a UI file that changes on every
// camp. Versions are gzipped into user.db, which also puts them in the app
// export bundle.

// File kinds.
const (
	KindSpellsets = "spellsets"
	KindBandolier = "bandolier"
	KindCharacter = "character" // <Char>_pq.proj.ini: hotkeys, social macros, chat
	KindUI        = "ui"        // UI_<Char>_pq.proj.ini: window layout
	KindBazaar    = "bazaar"    // BZR_<Char>[_pq.proj].ini: trader prices
	KindClient    = "client"    // eqclient.ini
	KindZeal      = "zeal"      // zeal.ini
	KindQuarm     = "quarm"     // quarmclient.ini
)

// DefaultFileHistoryKeep is the versions kept per file when the setting is 0.
const DefaultFileHistoryKeep = 30

// maxHistoryFileSize skips anything far bigger than an EQ settings file.
const maxHistoryFileSize = 4 << 20

// settleTime is how long a file must go unmodified before it's captured, so
// a capture never reads a file the client is still writing.
const settleTime = 5 * time.Second

var (
	reSpellsets = regexp.MustCompile(`(?i)^([a-z]+)_spellsets\.ini$`)
	reBandolier = regexp.MustCompile(`(?i)^([a-z]+)_bandolier\.ini$`)
	reUI        = regexp.MustCompile(`(?i)^UI_([a-z]+)_pq\.proj\.ini$`)
	reBazaar    = regexp.MustCompile(`(?i)^BZR_([a-z]+)(?:_pq\.proj)?\.ini$`)
	reCharacter = regexp.MustCompile(`(?i)^([a-z]+)_pq\.proj\.ini$`)
)

// ClassifyFile reports which kind of tracked file name is and the character
// it belongs to ("" for shared files). ok is false for files not tracked,
// such as hand-made copies ("eqclient - Copy.ini").
func ClassifyFile(name string) (kind, character string, ok bool) {
	for _, c := range []struct {
		re   *regexp.Regexp
		kind string
	}{
		{reSpellsets, KindSpellsets}, {reBandolier, KindBandolier},
		{reUI, KindUI}, {reBazaar, KindBazaar}, {reCharacter, KindCharacter},
	} {
		if m := c.re.FindStringSubmatch(name); m != nil {
			return c.kind, capitalize(m[1]), true
		}
	}
	switch strings.ToLower(name) {
	case "eqclient.ini":
		return KindClient, "", true
	case "zeal.ini":
		return KindZeal, "", true
	case "quarmclient.ini":
		return KindQuarm, "", true
	}
	return "", "", false
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + strings.ToLower(s[1:])
}

// FileVersion is one saved copy of a tracked file.
type FileVersion struct {
	ID         int64     `json:"id"`
	FileName   string    `json:"file_name"`
	CapturedAt time.Time `json:"captured_at"`
	// ModifiedAt is the file's modification time when it was captured.
	ModifiedAt time.Time `json:"modified_at"`
	SizeBytes  int64     `json:"size_bytes"`
	SHA256     string    `json:"sha256"`
}

// TrackedFile is a tracked file with its saved versions, newest first.
type TrackedFile struct {
	FileName  string `json:"file_name"`
	Kind      string `json:"kind"`
	Character string `json:"character"`
	// Exists is whether the file is in the EQ folder now.
	Exists   bool          `json:"exists"`
	Versions []FileVersion `json:"versions"`
}

func (s *Store) migrateHistory() error {
	if _, err := s.db.Exec(`
		CREATE TABLE IF NOT EXISTS file_versions (
			id          INTEGER PRIMARY KEY AUTOINCREMENT,
			file_name   TEXT    NOT NULL,
			captured_at INTEGER NOT NULL,
			modified_at INTEGER NOT NULL,
			size_bytes  INTEGER NOT NULL,
			sha256      TEXT    NOT NULL,
			content     BLOB    NOT NULL
		)
	`); err != nil {
		return err
	}
	_, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_file_versions_name
		ON file_versions(file_name, captured_at)`)
	return err
}

// latestHash returns the sha256 of the newest saved version of name, or "".
func (s *Store) latestHash(name string) (string, error) {
	var h string
	err := s.db.QueryRow(
		`SELECT sha256 FROM file_versions WHERE file_name = ?
		 ORDER BY captured_at DESC, id DESC LIMIT 1`, name,
	).Scan(&h)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return h, err
}

func (s *Store) insertVersion(v *FileVersion, gz []byte) error {
	res, err := s.db.Exec(
		`INSERT INTO file_versions (file_name, captured_at, modified_at, size_bytes, sha256, content)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		v.FileName, v.CapturedAt.UnixMilli(), v.ModifiedAt.UnixMilli(), v.SizeBytes, v.SHA256, gz,
	)
	if err != nil {
		return fmt.Errorf("insert file version: %w", err)
	}
	v.ID, _ = res.LastInsertId()
	return nil
}

// pruneVersions keeps the newest keep versions of name.
func (s *Store) pruneVersions(name string, keep int) error {
	_, err := s.db.Exec(
		`DELETE FROM file_versions WHERE file_name = ? AND id NOT IN (
			SELECT id FROM file_versions WHERE file_name = ?
			ORDER BY captured_at DESC, id DESC LIMIT ?)`,
		name, name, keep,
	)
	return err
}

func (s *Store) listVersions() ([]FileVersion, error) {
	rows, err := s.db.Query(
		`SELECT id, file_name, captured_at, modified_at, size_bytes, sha256
		 FROM file_versions ORDER BY file_name, captured_at DESC, id DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list file versions: %w", err)
	}
	defer rows.Close()
	var out []FileVersion
	for rows.Next() {
		var v FileVersion
		var cap, mod int64
		if err := rows.Scan(&v.ID, &v.FileName, &cap, &mod, &v.SizeBytes, &v.SHA256); err != nil {
			return nil, err
		}
		v.CapturedAt = time.UnixMilli(cap).UTC()
		v.ModifiedAt = time.UnixMilli(mod).UTC()
		out = append(out, v)
	}
	return out, rows.Err()
}

// versionContent returns a version's file name and uncompressed content.
func (s *Store) versionContent(id int64) (string, []byte, error) {
	var name string
	var gz []byte
	err := s.db.QueryRow(`SELECT file_name, content FROM file_versions WHERE id = ?`, id).Scan(&name, &gz)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil, fmt.Errorf("file version %d: %w", id, ErrNotFound)
	}
	if err != nil {
		return "", nil, err
	}
	r, err := gzip.NewReader(bytes.NewReader(gz))
	if err != nil {
		return "", nil, fmt.Errorf("file version %d: %w", id, err)
	}
	defer r.Close()
	data, err := io.ReadAll(io.LimitReader(r, maxHistoryFileSize+1))
	if err != nil {
		return "", nil, fmt.Errorf("file version %d: %w", id, err)
	}
	return name, data, nil
}

// historyKeep is the configured per-file version count.
func (m *Manager) historyKeep() int {
	if n := m.cfgMgr.Get().Backup.FileHistoryKeep; n > 0 {
		return n
	}
	return DefaultFileHistoryKeep
}

// CaptureFiles saves a new version of every tracked file in the EQ folder
// whose content differs from its newest saved version. Files modified in
// the last few seconds are left for the next pass unless force is set.
// Returns how many versions were saved.
func (m *Manager) CaptureFiles(force bool) (int, error) {
	m.historyMu.Lock()
	defer m.historyMu.Unlock()

	eqPath := m.cfgMgr.Get().EQPath
	if eqPath == "" {
		return 0, errors.New("eq_path is not configured")
	}
	paths, err := filepath.Glob(filepath.Join(eqPath, "*.ini"))
	if err != nil {
		return 0, fmt.Errorf("glob ini files: %w", err)
	}
	keep := m.historyKeep()
	now := time.Now()
	saved := 0
	for _, p := range paths {
		name := filepath.Base(p)
		if _, _, ok := ClassifyFile(name); !ok {
			continue
		}
		info, err := os.Stat(p)
		if err != nil || info.IsDir() || info.Size() > maxHistoryFileSize {
			continue
		}
		if !force && now.Sub(info.ModTime()) < settleTime {
			continue
		}
		if prev, ok := m.historySeen[name]; ok && !force &&
			prev.mod.Equal(info.ModTime()) && prev.size == info.Size() {
			continue
		}
		ok, err := m.captureOne(p, info, keep)
		if err != nil {
			slog.Warn("file history: capture failed", "file", name, "err", err)
			continue
		}
		m.historySeen[name] = statKey{mod: info.ModTime(), size: info.Size()}
		if ok {
			saved++
		}
	}
	if saved > 0 {
		slog.Info("file history: saved versions", "count", saved)
	}
	return saved, nil
}

type statKey struct {
	mod  time.Time
	size int64
}

// captureOne saves p if its content isn't the newest saved version.
func (m *Manager) captureOne(p string, info os.FileInfo, keep int) (bool, error) {
	data, err := os.ReadFile(p)
	if err != nil {
		return false, err
	}
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	name := filepath.Base(p)
	last, err := m.store.latestHash(name)
	if err != nil {
		return false, err
	}
	if last == hash {
		return false, nil
	}
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(data); err != nil {
		return false, err
	}
	if err := zw.Close(); err != nil {
		return false, err
	}
	v := &FileVersion{
		FileName:   name,
		CapturedAt: time.Now().UTC(),
		ModifiedAt: info.ModTime().UTC(),
		SizeBytes:  int64(len(data)),
		SHA256:     hash,
	}
	if err := m.store.insertVersion(v, buf.Bytes()); err != nil {
		return false, err
	}
	return true, m.store.pruneVersions(name, keep)
}

// TrackedFiles lists every file with saved versions or present in the EQ
// folder, sorted by character (shared files last) then file name.
func (m *Manager) TrackedFiles() ([]TrackedFile, error) {
	versions, err := m.store.listVersions()
	if err != nil {
		return nil, err
	}
	byName := map[string]*TrackedFile{}
	add := func(name string) *TrackedFile {
		if f, ok := byName[name]; ok {
			return f
		}
		kind, char, ok := ClassifyFile(name)
		if !ok {
			return nil
		}
		f := &TrackedFile{FileName: name, Kind: kind, Character: char, Versions: []FileVersion{}}
		byName[name] = f
		return f
	}
	for _, v := range versions {
		if f := add(v.FileName); f != nil {
			f.Versions = append(f.Versions, v)
		}
	}
	if eqPath := m.cfgMgr.Get().EQPath; eqPath != "" {
		paths, _ := filepath.Glob(filepath.Join(eqPath, "*.ini"))
		for _, p := range paths {
			if f := add(filepath.Base(p)); f != nil {
				f.Exists = true
			}
		}
	}
	out := make([]TrackedFile, 0, len(byName))
	for _, f := range byName {
		out = append(out, *f)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if (a.Character == "") != (b.Character == "") {
			return b.Character == ""
		}
		if a.Character != b.Character {
			return a.Character < b.Character
		}
		return strings.ToLower(a.FileName) < strings.ToLower(b.FileName)
	})
	return out, nil
}

// VersionContent returns a saved version's file name and content.
func (m *Manager) VersionContent(id int64) (string, []byte, error) {
	return m.store.versionContent(id)
}

// RestoreVersion writes a saved version back into the EQ folder. The file's
// current content is saved as a version first, so the restore can be undone.
func (m *Manager) RestoreVersion(id int64) error {
	eqPath := m.cfgMgr.Get().EQPath
	if eqPath == "" {
		return errors.New("eq_path is not configured")
	}
	name, data, err := m.store.versionContent(id)
	if err != nil {
		return err
	}
	if filepath.Base(name) != name {
		return fmt.Errorf("invalid file name %q", name)
	}
	if _, err := m.CaptureFiles(true); err != nil {
		return fmt.Errorf("save current version first: %w", err)
	}
	if err := writeFileAtomic(filepath.Join(eqPath, name), data); err != nil {
		return fmt.Errorf("restore %s: %w", name, err)
	}
	slog.Info("file history: restored", "file", name, "version", id)
	return nil
}

// writeFileAtomic writes data to a temp file beside dest, then renames it
// over dest, so a failed write never leaves a half-written settings file.
func writeFileAtomic(dest string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".pqc-restore-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), dest)
}

// StartFileHistory captures changed settings files every minute while file
// history is on. The first pass waits a little so it stays off startup.
func (m *Manager) StartFileHistory(ctx context.Context) {
	first := time.NewTimer(30 * time.Second)
	defer first.Stop()
	select {
	case <-ctx.Done():
		return
	case <-first.C:
	}
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		cfg := m.cfgMgr.Get()
		if !cfg.Backup.FileHistoryOff && cfg.EQPath != "" {
			if _, err := m.CaptureFiles(false); err != nil {
				slog.Warn("file history: capture pass failed", "err", err)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
