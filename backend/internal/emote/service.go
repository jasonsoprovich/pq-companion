package emote

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
)

const (
	defaultBackupName = "spells_en.default.txt"
	editedBackupName  = "spells_en.edited.txt"

	metaDefaultHash    = "default_hash"
	metaLastWriteHash  = "last_write_hash"
	metaDefaultCapture = "default_captured_at"
)

// Service orchestrates reading/writing spells_en.txt and keeping it in sync
// with the overrides stored in Store. backupDir holds the pristine default
// and last-edited copies (never inside the EQ install directory).
type Service struct {
	store     *Store
	cfgMgr    *config.Manager
	backupDir string

	mu              sync.Mutex
	pendingContent  string // candidate new live-file content from an external change
	pendingDetected time.Time
}

// NewService constructs a Service. backupDir is typically
// ~/.pq-companion/spell-emotes.
func NewService(store *Store, cfgMgr *config.Manager, backupDir string) *Service {
	return &Service{store: store, cfgMgr: cfgMgr, backupDir: backupDir}
}

func (s *Service) livePath() (string, error) {
	eqPath := s.cfgMgr.Get().EQPath
	if eqPath == "" {
		return "", fmt.Errorf("EverQuest directory is not configured")
	}
	return filepath.Join(eqPath, fileName), nil
}

func (s *Service) defaultBackupPath() string { return filepath.Join(s.backupDir, defaultBackupName) }
func (s *Service) editedBackupPath() string  { return filepath.Join(s.backupDir, editedBackupName) }

func (s *Service) readLive() (string, error) {
	path, err := s.livePath()
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", fileName, err)
	}
	return string(b), nil
}

func (s *Service) writeLive(content string) error {
	path, err := s.livePath()
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func readBackup(path string) (string, bool, error) {
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return string(b), true, nil
}

func (s *Service) writeBackup(path, content string) error {
	if err := os.MkdirAll(s.backupDir, 0o755); err != nil {
		return fmt.Errorf("create backup dir: %w", err)
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

// EnsureDefaultBackup captures the current live file as the pristine default
// backup if one doesn't exist yet. Safe to call repeatedly; a no-op once a
// default backup is present.
func (s *Service) EnsureDefaultBackup() error {
	if _, ok, err := readBackup(s.defaultBackupPath()); err != nil {
		return err
	} else if ok {
		return nil
	}
	content, err := s.readLive()
	if err != nil {
		// No live file yet (EQ dir not set or file missing) — nothing to
		// capture; not an error, just deferred until the file exists.
		return nil
	}
	return s.captureAsDefault(content)
}

// captureAsDefault records content as the pristine default backup and, since
// it reflects the live file's current bytes at the moment of capture, also
// as the last-known-write hash — otherwise the watcher's very next poll (or
// any later restart before a first edit) would see no last_write_hash
// recorded, skip its unchanged-content short-circuit, and misfire an
// "external change" even though nothing actually changed.
func (s *Service) captureAsDefault(content string) error {
	if err := s.writeBackup(s.defaultBackupPath(), content); err != nil {
		return err
	}
	if err := s.store.SetMeta(metaDefaultHash, hashContent(content)); err != nil {
		return err
	}
	if err := s.store.SetMeta(metaLastWriteHash, hashContent(content)); err != nil {
		return err
	}
	return s.store.SetMeta(metaDefaultCapture, strconv.FormatInt(time.Now().UTC().Unix(), 10))
}

// overridesByID loads every stored override keyed by spell id.
func (s *Service) overridesByID() (map[int]OverrideRow, error) {
	rows, err := s.store.ListOverrides()
	if err != nil {
		return nil, err
	}
	out := make(map[int]OverrideRow, len(rows))
	for _, r := range rows {
		out[r.SpellID] = r
	}
	return out, nil
}

// rebuildAndWrite re-applies every stored override onto the pristine default
// backup and writes the result to the live file and the edited backup.
// Rebuilding from the default (rather than the current live content) is what
// makes a revert actually restore default text instead of re-stamping
// whatever was already written — overrides are always layered fresh onto a
// clean base, never onto a previously-overridden file.
func (s *Service) rebuildAndWrite() error {
	base, ok, err := readBackup(s.defaultBackupPath())
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no default backup captured yet")
	}
	overrides, err := s.overridesByID()
	if err != nil {
		return err
	}
	rebuilt := applyOverrides(base, overrides)
	if err := s.writeLive(rebuilt); err != nil {
		return err
	}
	if err := s.writeBackup(s.editedBackupPath(), rebuilt); err != nil {
		return err
	}
	return s.store.SetMeta(metaLastWriteHash, hashContent(rebuilt))
}

// SetOverride sets or updates spellID's override columns from patch and
// rewrites spells_en.txt.
func (s *Service) SetOverride(spellID int, patch ColumnsPatch) error {
	if err := s.EnsureDefaultBackup(); err != nil {
		return err
	}
	if err := s.store.SetColumns(spellID, patch); err != nil {
		return err
	}
	return s.rebuildAndWrite()
}

// RevertOverride reverts spellID to its default emotes and rewrites the file.
func (s *Service) RevertOverride(spellID int) error {
	if err := s.store.DeleteOverride(spellID); err != nil {
		return err
	}
	return s.rebuildAndWrite()
}

// RestoreDefaults writes the pristine default backup back to the live file
// and clears every stored override.
func (s *Service) RestoreDefaults() error {
	content, ok, err := readBackup(s.defaultBackupPath())
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no default backup captured yet")
	}
	if err := s.store.DeleteAllOverrides(); err != nil {
		return err
	}
	if err := s.writeLive(content); err != nil {
		return err
	}
	if err := s.writeBackup(s.editedBackupPath(), content); err != nil {
		return err
	}
	s.clearPending()
	return s.store.SetMeta(metaLastWriteHash, hashContent(content))
}

// MarkExternalChange records that the live file changed outside the app
// (most likely a server patch republishing spells_en.txt), stashing its new
// content for ReapplyAll/IgnoreExternalChange to act on.
func (s *Service) MarkExternalChange(content string) {
	s.mu.Lock()
	s.pendingContent = content
	s.pendingDetected = time.Now().UTC()
	s.mu.Unlock()
}

func (s *Service) pending() (string, time.Time, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.pendingContent, s.pendingDetected, s.pendingContent != ""
}

func (s *Service) clearPending() {
	s.mu.Lock()
	s.pendingContent = ""
	s.pendingDetected = time.Time{}
	s.mu.Unlock()
}

// ReapplyAll re-applies every stored override onto the pending externally-
// changed content (adopting it as the new default backup, since it reflects
// whatever the server just shipped) and writes the result to the live file.
func (s *Service) ReapplyAll() error {
	content, _, ok := s.pending()
	if !ok {
		// Nothing pending — fall back to re-applying overrides onto whatever
		// is currently live (e.g. called manually, not from a detected change).
		live, err := s.readLive()
		if err != nil {
			return err
		}
		content = live
	}
	if err := s.captureAsDefault(content); err != nil {
		return err
	}
	if err := s.rebuildAndWrite(); err != nil {
		return err
	}
	s.clearPending()
	return nil
}

// IgnoreExternalChange adopts the current live content as the new pristine
// default without re-applying overrides — the user's customizations stay
// recorded but unapplied until they choose to re-apply or edit again.
func (s *Service) IgnoreExternalChange() error {
	content, _, ok := s.pending()
	if !ok {
		return nil
	}
	if err := s.captureAsDefault(content); err != nil {
		return err
	}
	if err := s.writeBackup(s.editedBackupPath(), content); err != nil {
		return err
	}
	if err := s.store.SetMeta(metaLastWriteHash, hashContent(content)); err != nil {
		return err
	}
	s.clearPending()
	return nil
}

// GetSpellEmote returns spellID's default/current emote text and which
// columns are actively overridden. Reads directly from the live file (or the
// default backup if the live file/EQ dir isn't available) plus the default
// backup for comparison.
func (s *Service) GetSpellEmote(spellID int) (*SpellEmote, error) {
	defaultContent, hasDefault, err := readBackup(s.defaultBackupPath())
	if err != nil {
		return nil, err
	}
	current, err := s.readLive()
	if err != nil {
		if !hasDefault {
			return nil, err
		}
		current = defaultContent
	}

	currentFields := findSpellLine(current, spellID)
	if currentFields == nil {
		return nil, fmt.Errorf("spell %d not found in spells_en.txt", spellID)
	}
	var defaultFields []string
	if hasDefault {
		defaultFields = findSpellLine(defaultContent, spellID)
	}
	if defaultFields == nil {
		defaultFields = currentFields
	}

	ov, err := s.store.GetOverride(spellID)
	if err != nil {
		return nil, err
	}
	var overridden []string
	if ov != nil {
		ptrs := ov.fieldPtrs()
		for i, p := range ptrs {
			if p != nil {
				overridden = append(overridden, columnField(i))
			}
		}
	}

	return &SpellEmote{
		SpellID:          spellID,
		Name:             currentFields[1],
		Default:          lineEmoteText(defaultFields),
		Current:          lineEmoteText(currentFields),
		Customized:       len(overridden) > 0,
		OverriddenFields: overridden,
	}, nil
}

func columnField(i int) string {
	return [5]string{"you_cast", "other_casts", "cast_on_you", "cast_on_other", "spell_fades"}[i]
}

// ListCustomized returns every spell with at least one stored override,
// enriched with its current emote text.
func (s *Service) ListCustomized() ([]SpellEmote, error) {
	rows, err := s.store.ListOverrides()
	if err != nil {
		return nil, err
	}
	out := make([]SpellEmote, 0, len(rows))
	for _, r := range rows {
		se, err := s.GetSpellEmote(r.SpellID)
		if err != nil {
			continue // spell removed from the game DB/file since being overridden
		}
		out = append(out, *se)
	}
	return out, nil
}

// Diff computes the per-spell field-level differences between the pristine
// default backup and the last-written edited backup.
func (s *Service) Diff() ([]SpellDiff, error) {
	defaultContent, hasDefault, err := readBackup(s.defaultBackupPath())
	if err != nil {
		return nil, err
	}
	editedContent, hasEdited, err := readBackup(s.editedBackupPath())
	if err != nil {
		return nil, err
	}
	if !hasDefault || !hasEdited {
		return []SpellDiff{}, nil
	}

	editedByID := make(map[int][]string)
	for _, line := range splitLines(editedContent) {
		fields := parseFields(line)
		if len(fields) < minEmoteFields {
			continue
		}
		if id, ok := lineSpellID(fields); ok {
			editedByID[id] = fields
		}
	}

	var diffs []SpellDiff
	for _, line := range splitLines(defaultContent) {
		defFields := parseFields(line)
		if len(defFields) < minEmoteFields {
			continue
		}
		id, ok := lineSpellID(defFields)
		if !ok {
			continue
		}
		editFields, ok := editedByID[id]
		if !ok {
			continue
		}
		idxs := [5]int{idxYouCast, idxOtherCasts, idxCastOnYou, idxCastOnOther, idxSpellFades}
		var fields []FieldDiff
		for i, idx := range idxs {
			if defFields[idx] != editFields[idx] {
				fields = append(fields, FieldDiff{
					Field: columnField(i),
					Label: columnLabels[i],
					Old:   defFields[idx],
					New:   editFields[idx],
				})
			}
		}
		if len(fields) > 0 {
			diffs = append(diffs, SpellDiff{SpellID: id, Name: defFields[1], Fields: fields})
		}
	}
	sort.Slice(diffs, func(i, j int) bool { return diffs[i].SpellID < diffs[j].SpellID })
	return diffs, nil
}

// Status reports the panel's at-a-glance state.
func (s *Service) Status() (*Status, error) {
	cfg := s.cfgMgr.Get()
	st := &Status{Configured: cfg.EQPath != ""}
	if st.Configured {
		if path, err := s.livePath(); err == nil {
			if _, err := os.Stat(path); err == nil {
				st.FilePresent = true
			}
		}
	}
	if _, ok, err := readBackup(s.defaultBackupPath()); err != nil {
		return nil, err
	} else {
		st.HasDefaultBackup = ok
	}
	overrides, err := s.store.ListOverrides()
	if err != nil {
		return nil, err
	}
	st.OverrideCount = len(overrides)
	if _, at, ok := s.pending(); ok {
		st.PendingExternalChange = true
		st.ExternalChangeAt = at.Unix()
	}
	return st, nil
}
