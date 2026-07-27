package emote

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
)

const (
	defaultBackupName = "spells_en.default.txt"
	editedBackupName  = "spells_en.edited.txt"

	metaDefaultHash    = "default_hash"
	metaLastWriteHash  = "last_write_hash"
	metaDefaultCapture = "default_captured_at"
	metaPendingImport  = "pending_import"
)

// Service orchestrates reading/writing spells_en.txt and keeping it in sync
// with the overrides stored in Store. backupDir holds the pristine default
// and last-edited copies (never inside the EQ install directory). database
// is the read-only game DB, queried for quarm.db's canonical emote text —
// the true pristine default, since a user's spells_en.txt may already carry
// hand-edits from before they ever used this feature (see bootstrapDefault).
type Service struct {
	store     *Store
	cfgMgr    *config.Manager
	backupDir string
	database  *db.DB

	mu              sync.Mutex
	pendingContent  string // candidate new live-file content from an external change
	pendingDetected time.Time
}

// NewService constructs a Service. backupDir is typically
// ~/.pq-companion/spell-emotes.
func NewService(store *Store, cfgMgr *config.Manager, backupDir string, database *db.DB) *Service {
	return &Service{store: store, cfgMgr: cfgMgr, backupDir: backupDir, database: database}
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

// EnsureDefaultBackup captures a pristine default backup if one doesn't
// exist yet, bootstrapping it from quarm.db's canonical emote text rather
// than trusting the live file's current bytes (see bootstrapDefault). Safe
// to call repeatedly; a no-op once a default backup is present.
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
	return s.bootstrapDefault(content)
}

// canonicalize rewrites content's emote columns to quarm.db's current
// canonical text. Used for every default capture so "default" never means
// "whatever happened to be on disk," only ever "what the server's data
// actually says."
func (s *Service) canonicalize(content string) (string, error) {
	defaults, err := s.database.LoadSpellEmoteDefaults()
	if err != nil {
		return "", fmt.Errorf("load canonical spell emotes: %w", err)
	}
	return deriveDefaultContent(content, defaults), nil
}

// bootstrapDefault is the first-ever capture of a pristine default: it
// canonicalizes liveContent against quarm.db and stores that as the default
// backup, then checks whether liveContent itself already diverged from that
// canonical text on any spell. A divergence there means the user hand-edited
// spells_en.txt before ever using this feature (exactly the scenario a
// player like the one who requested this feature was already doing) —
// those spells are recorded as a pending import so the UI can offer to adopt
// them as tracked, patch-surviving overrides rather than silently losing
// them the moment something else triggers a rebuild.
func (s *Service) bootstrapDefault(liveContent string) error {
	canonical, err := s.canonicalize(liveContent)
	if err != nil {
		return err
	}
	if err := s.captureAsDefault(canonical); err != nil {
		return err
	}
	pending := detectDivergence(liveContent, canonical)
	if len(pending) == 0 {
		return nil
	}
	b, err := json.Marshal(pending)
	if err != nil {
		return err
	}
	return s.store.SetMeta(metaPendingImport, string(b))
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

// pendingImports reads the stored pending-import list (see bootstrapDefault),
// or nil if there is none.
func (s *Service) pendingImports() ([]SpellDiff, error) {
	raw, ok, err := s.store.GetMeta(metaPendingImport)
	if err != nil {
		return nil, err
	}
	if !ok || raw == "" {
		return nil, nil
	}
	var pending []SpellDiff
	if err := json.Unmarshal([]byte(raw), &pending); err != nil {
		return nil, fmt.Errorf("decode pending import list: %w", err)
	}
	return pending, nil
}

func (s *Service) setPendingImports(pending []SpellDiff) error {
	if len(pending) == 0 {
		return s.store.SetMeta(metaPendingImport, "")
	}
	b, err := json.Marshal(pending)
	if err != nil {
		return err
	}
	return s.store.SetMeta(metaPendingImport, string(b))
}

// pendingImportOverrides converts the not-yet-imported pending list into the
// same OverrideRow shape as a tracked override, using each field's New (the
// user's pre-existing hand-edited) value. This is what protects a
// not-yet-reviewed hand-edit from being silently wiped out the moment an
// unrelated SetOverride/RevertOverride triggers a rebuild — the pending
// entry behaves like an override for rebuild purposes until the user either
// formally imports it (ImportExisting) or wipes it deliberately
// (RestoreDefaults).
func (s *Service) pendingImportOverrides() (map[int]OverrideRow, error) {
	pending, err := s.pendingImports()
	if err != nil {
		return nil, err
	}
	out := make(map[int]OverrideRow, len(pending))
	for _, sd := range pending {
		row := OverrideRow{SpellID: sd.SpellID}
		for _, f := range sd.Fields {
			v := f.New
			switch f.Field {
			case "you_cast":
				row.YouCast = &v
			case "other_casts":
				row.OtherCasts = &v
			case "cast_on_you":
				row.CastOnYou = &v
			case "cast_on_other":
				row.CastOnOther = &v
			case "spell_fades":
				row.SpellFades = &v
			}
		}
		out[sd.SpellID] = row
	}
	return out, nil
}

// effectiveOverrides merges tracked overrides with any still-pending import
// entries (tracked values win on the rare overlap). Used only when rebuilding
// the live file — reads that only care about *tracked* customizations (the
// "customized" badge, the diff view, the customized-only filter) use
// overridesByID directly instead.
func (s *Service) effectiveOverrides() (map[int]OverrideRow, error) {
	tracked, err := s.overridesByID()
	if err != nil {
		return nil, err
	}
	pending, err := s.pendingImportOverrides()
	if err != nil {
		return nil, err
	}
	out := make(map[int]OverrideRow, len(tracked)+len(pending))
	for id, row := range pending {
		out[id] = row
	}
	for id, row := range tracked {
		out[id] = row
	}
	return out, nil
}

// PendingImports returns every spell whose emote text was already customized
// in spells_en.txt before this feature ever ran, still awaiting the user's
// decision to import them as tracked overrides. Empty (not nil) when there's
// nothing pending.
func (s *Service) PendingImports() ([]SpellDiff, error) {
	pending, err := s.pendingImports()
	if err != nil {
		return nil, err
	}
	if pending == nil {
		return []SpellDiff{}, nil
	}
	return pending, nil
}

// ImportExisting adopts pending-import entries as real tracked overrides —
// spellIDs selects which ones; nil/empty imports all of them. The live file
// itself doesn't change (pending entries were already protecting that text),
// but afterward the spells show up as customized, appear in the diff view,
// and will be explicitly re-applied after a future server patch.
func (s *Service) ImportExisting(spellIDs []int) (int, error) {
	pending, err := s.pendingImports()
	if err != nil {
		return 0, err
	}
	if len(pending) == 0 {
		return 0, nil
	}
	selected := func(int) bool { return true }
	if len(spellIDs) > 0 {
		want := make(map[int]bool, len(spellIDs))
		for _, id := range spellIDs {
			want[id] = true
		}
		selected = func(id int) bool { return want[id] }
	}

	var remaining []SpellDiff
	imported := 0
	for _, sd := range pending {
		if !selected(sd.SpellID) {
			remaining = append(remaining, sd)
			continue
		}
		patch := ColumnsPatch{}
		for _, f := range sd.Fields {
			v := f.New
			switch f.Field {
			case "you_cast":
				patch.YouCast = &v
			case "other_casts":
				patch.OtherCasts = &v
			case "cast_on_you":
				patch.CastOnYou = &v
			case "cast_on_other":
				patch.CastOnOther = &v
			case "spell_fades":
				patch.SpellFades = &v
			}
		}
		if err := s.store.SetColumns(sd.SpellID, patch); err != nil {
			return imported, err
		}
		imported++
	}
	if imported == 0 {
		return 0, nil
	}
	if err := s.setPendingImports(remaining); err != nil {
		return imported, err
	}
	return imported, s.rebuildAndWrite()
}

// rebuildAndWrite re-applies every effective override (tracked + still-
// pending imports) onto the pristine default backup and writes the result to
// the live file and the edited backup. Rebuilding from the default (rather
// than the current live content) is what makes a revert actually restore
// default text instead of re-stamping whatever was already written —
// overrides are always layered fresh onto a clean base, never onto a
// previously-overridden file.
func (s *Service) rebuildAndWrite() error {
	base, ok, err := readBackup(s.defaultBackupPath())
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no default backup captured yet")
	}
	overrides, err := s.effectiveOverrides()
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
// RevertOverride reverts spellID to its default emotes and rewrites the
// file — clearing both a tracked override row (if any) and, if the spell is
// only sitting in the not-yet-reviewed pending-import list, removing it
// from there too. Without the second part, a spell that was never formally
// imported would keep its pending entry protecting the altered text as an
// implicit override (see effectiveOverrides), and "revert" would silently
// do nothing.
func (s *Service) RevertOverride(spellID int) error {
	if err := s.store.DeleteOverride(spellID); err != nil {
		return err
	}
	if err := s.removePendingImport(spellID); err != nil {
		return err
	}
	return s.rebuildAndWrite()
}

// removePendingImport drops spellID from the pending-import list, if present.
func (s *Service) removePendingImport(spellID int) error {
	pending, err := s.pendingImports()
	if err != nil {
		return err
	}
	if len(pending) == 0 {
		return nil
	}
	remaining := make([]SpellDiff, 0, len(pending))
	changed := false
	for _, sd := range pending {
		if sd.SpellID == spellID {
			changed = true
			continue
		}
		remaining = append(remaining, sd)
	}
	if !changed {
		return nil
	}
	return s.setPendingImports(remaining)
}

// RestoreDefaults writes the pristine default backup back to the live file
// and clears every stored override — including any not-yet-reviewed pending
// import, since this is the explicit, confirmed "wipe everything back to
// canonical" action.
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
	if err := s.setPendingImports(nil); err != nil {
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
//
// This deliberately does NOT re-canonicalize against quarm.db the way
// bootstrapDefault does: quarm.db is refreshed on its own data-release
// cadence, separate from (and typically slower than) the server's own
// spells_en.txt patches, so at the moment a patch is detected the live file
// itself — not PQC's possibly-stale bundled DB — is the freshest source of
// truth for what the server currently ships.
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
// recorded but unapplied until they choose to re-apply or edit again. See
// ReapplyAll for why this trusts the live content directly rather than
// re-canonicalizing against quarm.db.
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
// columns currently differ from default. Reads directly from the live file
// (or the default backup if the live file/EQ dir isn't available) plus the
// default backup for comparison.
//
// "Customized"/OverriddenFields are derived from comparing Default against
// Current column-by-column — NOT from whether a tracked override row exists.
// A spell can differ from default without being tracked yet (a pending
// import the user hasn't reviewed, or any other divergence), and the UI
// must flag that and offer to reset it either way; whether the divergence
// happens to be backed by a store row is an internal bookkeeping detail.
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

	idxs := [5]int{idxYouCast, idxOtherCasts, idxCastOnYou, idxCastOnOther, idxSpellFades}
	var overridden []string
	for i, idx := range idxs {
		if idx < len(defaultFields) && idx < len(currentFields) && defaultFields[idx] != currentFields[idx] {
			overridden = append(overridden, columnField(i))
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

// ListCustomized returns every spell whose emote text currently differs
// from default, enriched with its current emote text — both spells with a
// tracked override row and spells only sitting in the not-yet-reviewed
// pending-import list (the "Customized only" filter should show everything
// actually customized, not just the subset the app happens to be tracking).
func (s *Service) ListCustomized() ([]SpellEmote, error) {
	rows, err := s.store.ListOverrides()
	if err != nil {
		return nil, err
	}
	pending, err := s.pendingImports()
	if err != nil {
		return nil, err
	}
	seen := make(map[int]bool, len(rows)+len(pending))
	spellIDs := make([]int, 0, len(rows)+len(pending))
	for _, r := range rows {
		if !seen[r.SpellID] {
			seen[r.SpellID] = true
			spellIDs = append(spellIDs, r.SpellID)
		}
	}
	for _, sd := range pending {
		if !seen[sd.SpellID] {
			seen[sd.SpellID] = true
			spellIDs = append(spellIDs, sd.SpellID)
		}
	}

	out := make([]SpellEmote, 0, len(spellIDs))
	for _, id := range spellIDs {
		se, err := s.GetSpellEmote(id)
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
	diffs := detectDivergence(editedContent, defaultContent)
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
	pendingImports, err := s.pendingImports()
	if err != nil {
		return nil, err
	}
	st.PendingImportCount = len(pendingImports)
	if _, at, ok := s.pending(); ok {
		st.PendingExternalChange = true
		st.ExternalChangeAt = at.Unix()
	}
	return st, nil
}
