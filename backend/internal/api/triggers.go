package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jasonsoprovich/pq-companion/backend/internal/character"
	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/trigger"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

type triggerHandler struct {
	store     *trigger.Store
	engine    *trigger.Engine
	hub       *ws.Hub
	charStore *character.Store
	tailer    *logparser.Tailer
	cfgMgr    *config.Manager

	// Latest active test/positioning session. Held in memory so the trigger
	// overlay window can hydrate after a fresh mount even if it missed the
	// initial WS broadcast (window-startup race).
	testMu     sync.Mutex
	latestTest *testOverlayRequest

	// tester runs real-time Trigger Tester playback sessions (see
	// trigger/tester.go). Wired by NewRouter; instant (non-realtime) test
	// requests call engine.RunTest directly and never touch this.
	tester *trigger.Tester
}

// list returns all triggers.
func (h *triggerHandler) list(w http.ResponseWriter, r *http.Request) {
	triggers, err := h.store.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if triggers == nil {
		triggers = []*trigger.Trigger{}
	}
	writeJSON(w, http.StatusOK, triggers)
}

// triggerRequest is the shared JSON payload accepted by create and update.
type triggerRequest struct {
	Name                 string            `json:"name"`
	Enabled              bool              `json:"enabled"`
	Pattern              string            `json:"pattern"`
	Actions              []trigger.Action  `json:"actions"`
	TimerType            trigger.TimerType `json:"timer_type"`
	TimerDurationSecs    float64           `json:"timer_duration_secs"`
	TimerDurationCapture string            `json:"timer_duration_capture"`
	TimerKeyCapture      string            `json:"timer_key_capture"`
	TimerTargetCapture   string            `json:"timer_target_capture"`
	WornOffPattern       string            `json:"worn_off_pattern"`
	SpellID              int               `json:"spell_id"`
	// RefireCooldownSecs is a pointer so an omitted field on update preserves
	// the existing value (matching how cooldown_secs is left untouched). A
	// present value — including 0 to clear it — replaces it. The editor always
	// sends it; bulk paths (toggle, move category) omit it.
	RefireCooldownSecs   *float64 `json:"refire_cooldown_secs,omitempty"`
	DisplayThresholdSecs float64  `json:"display_threshold_secs"`
	BarColor             string   `json:"bar_color"`
	Pinned               bool     `json:"pinned"`
	// TimerStack requests "stack timers" — see Trigger.TimerStack. The
	// handler coerces this to false unless the trigger's TimerType is
	// "custom".
	TimerStack bool `json:"timer_stack"`
	// CustomGroupID selects which Custom Timers window a timer_type="custom"
	// trigger's timer appears in. Empty = the original/default window.
	CustomGroupID   string                 `json:"custom_group_id"`
	Characters      []string               `json:"characters"`
	TimerAlerts     []trigger.TimerAlert   `json:"timer_alerts"`
	ExcludePatterns []string               `json:"exclude_patterns"`
	ExtraPatterns   []trigger.ExtraPattern `json:"extra_patterns"`
	Source          string                 `json:"source,omitempty"`
	PipeCondition   *trigger.PipeCondition `json:"pipe_condition,omitempty"`
	// PackName is the trigger's category, by name — root-level only, kept for
	// older callers. Pointer so an omitted field on update leaves the existing
	// category untouched (a present value — even "" for Uncategorized —
	// replaces it). nil on create defaults to "". CategoryID, when present,
	// takes precedence over PackName — it's the only way to place a trigger
	// into a subcategory, since a bare name always resolves at the top level
	// (see resolveCategoryLink).
	PackName   *string `json:"pack_name,omitempty"`
	CategoryID *string `json:"category_id,omitempty"`
}

// validateTriggerRequest enforces the per-source field rules: log triggers
// require a regex pattern; pipe triggers require a non-empty PipeCondition.
// Returns an error message suitable for writeError when validation fails,
// or "" when the request is acceptable.
func validateTriggerRequest(req *triggerRequest) string {
	if req.Name == "" {
		return "name is required"
	}
	src := req.Source
	if src == "" {
		src = trigger.SourceLog
	}
	switch src {
	case trigger.SourceLog:
		if req.Pattern == "" {
			return "pattern is required for log-source triggers"
		}
	case trigger.SourcePipe:
		if req.PipeCondition == nil || req.PipeCondition.Kind == "" {
			return "pipe_condition.kind is required for pipe-source triggers"
		}
	default:
		return "source must be 'log' or 'pipe'"
	}
	return ""
}

// normalizeTimerType coerces an incoming timer_type into one of the valid
// values, defaulting to "none" for anything else (including blank).
func normalizeTimerType(t trigger.TimerType) trigger.TimerType {
	switch t {
	case trigger.TimerTypeBuff, trigger.TimerTypeDetrimental, trigger.TimerTypeCustom:
		return t
	}
	return trigger.TimerTypeNone
}

// create adds a new trigger.
func (h *triggerHandler) create(w http.ResponseWriter, r *http.Request) {
	var req triggerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if msg := validateTriggerRequest(&req); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	id, err := trigger.NewID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	src := req.Source
	if src == "" {
		src = trigger.SourceLog
	}
	packName := ""
	if req.PackName != nil {
		packName = strings.TrimSpace(*req.PackName)
	}
	categoryID := ""
	if req.CategoryID != nil && strings.TrimSpace(*req.CategoryID) != "" {
		cat, err := h.store.CategoryByID(strings.TrimSpace(*req.CategoryID))
		if err != nil {
			writeCategoryError(w, err)
			return
		}
		categoryID = cat.ID
		packName = cat.Name
	}
	t := &trigger.Trigger{
		ID:                   id,
		Name:                 req.Name,
		Enabled:              req.Enabled,
		Pattern:              req.Pattern,
		Actions:              req.Actions,
		PackName:             packName,
		CategoryID:           categoryID,
		CreatedAt:            time.Now().UTC(),
		TimerType:            normalizeTimerType(req.TimerType),
		TimerDurationSecs:    req.TimerDurationSecs,
		TimerDurationCapture: strings.TrimSpace(req.TimerDurationCapture),
		TimerKeyCapture:      strings.TrimSpace(req.TimerKeyCapture),
		TimerTargetCapture:   strings.TrimSpace(req.TimerTargetCapture),
		WornOffPattern:       req.WornOffPattern,
		SpellID:              req.SpellID,
		DisplayThresholdSecs: req.DisplayThresholdSecs,
		BarColor:             strings.TrimSpace(req.BarColor),
		Pinned:               req.Pinned,
		// TimerStack only ever applies to custom timers — see its doc comment
		// on Trigger. Coerced here (not just in the frontend) so a crafted or
		// stale request can't enable it on a buff/detrimental trigger.
		TimerStack:      req.TimerStack && normalizeTimerType(req.TimerType) == trigger.TimerTypeCustom,
		CustomGroupID:   strings.TrimSpace(req.CustomGroupID),
		Characters:      req.Characters,
		TimerAlerts:     req.TimerAlerts,
		ExcludePatterns: req.ExcludePatterns,
		ExtraPatterns:   req.ExtraPatterns,
		Source:          src,
		PipeCondition:   req.PipeCondition,
	}
	if req.RefireCooldownSecs != nil {
		t.RefireCooldownSecs = *req.RefireCooldownSecs
	}
	if t.Actions == nil {
		t.Actions = []trigger.Action{}
	}
	if t.Characters == nil {
		t.Characters = []string{}
	}
	if t.TimerAlerts == nil {
		t.TimerAlerts = []trigger.TimerAlert{}
	}
	if t.ExcludePatterns == nil {
		t.ExcludePatterns = []string{}
	}
	if t.ExtraPatterns == nil {
		t.ExtraPatterns = []trigger.ExtraPattern{}
	}
	// Append the new trigger to the end of its category's manual order.
	if order, err := h.store.NextTriggerSortOrder(t.PackName); err == nil {
		t.SortOrder = order
	}
	if err := h.store.Insert(t); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.engine.Reload()
	writeJSON(w, http.StatusCreated, t)
}

// update replaces a trigger's mutable fields.
func (h *triggerHandler) update(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	existing, err := h.store.Get(id)
	if err != nil {
		if errors.Is(err, trigger.ErrNotFound) {
			writeError(w, http.StatusNotFound, "trigger not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	var req triggerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if msg := validateTriggerRequest(&req); msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	src := req.Source
	if src == "" {
		src = trigger.SourceLog
	}

	existing.Name = req.Name
	existing.Enabled = req.Enabled
	existing.Pattern = req.Pattern
	existing.Actions = req.Actions
	existing.TimerType = normalizeTimerType(req.TimerType)
	existing.TimerDurationSecs = req.TimerDurationSecs
	existing.TimerDurationCapture = strings.TrimSpace(req.TimerDurationCapture)
	existing.TimerKeyCapture = strings.TrimSpace(req.TimerKeyCapture)
	existing.TimerTargetCapture = strings.TrimSpace(req.TimerTargetCapture)
	existing.WornOffPattern = req.WornOffPattern
	existing.SpellID = req.SpellID
	// Only touch the refire cooldown when the request carries it — bulk paths
	// (toggle enabled, move category) omit it and must leave it intact.
	if req.RefireCooldownSecs != nil {
		existing.RefireCooldownSecs = *req.RefireCooldownSecs
	}
	existing.DisplayThresholdSecs = req.DisplayThresholdSecs
	existing.BarColor = strings.TrimSpace(req.BarColor)
	existing.Pinned = req.Pinned
	existing.TimerStack = req.TimerStack && existing.TimerType == trigger.TimerTypeCustom
	existing.CustomGroupID = strings.TrimSpace(req.CustomGroupID)
	existing.Characters = req.Characters
	existing.TimerAlerts = req.TimerAlerts
	existing.ExcludePatterns = req.ExcludePatterns
	existing.ExtraPatterns = req.ExtraPatterns
	existing.Source = src
	existing.PipeCondition = req.PipeCondition
	// Only touch the category when the request carries category_id or
	// pack_name — an omitted field (older callers, edits that don't change
	// category) leaves the existing value intact. On a category change,
	// re-append to the end of the destination's manual order. category_id
	// takes precedence — it's the only way to target a subcategory, since a
	// bare pack_name always resolves at the top level.
	switch {
	case req.CategoryID != nil:
		newID := strings.TrimSpace(*req.CategoryID)
		if newID != existing.CategoryID {
			newPack := ""
			if newID != "" {
				cat, err := h.store.CategoryByID(newID)
				if err != nil {
					writeCategoryError(w, err)
					return
				}
				newPack = cat.Name
			}
			existing.CategoryID = newID
			existing.PackName = newPack
			if order, err := h.store.NextTriggerSortOrder(newPack); err == nil {
				existing.SortOrder = order
			}
		}
	case req.PackName != nil:
		newPack := strings.TrimSpace(*req.PackName)
		if newPack != existing.PackName {
			existing.PackName = newPack
			existing.CategoryID = ""
			if order, err := h.store.NextTriggerSortOrder(newPack); err == nil {
				existing.SortOrder = order
			}
		}
	}
	if existing.Actions == nil {
		existing.Actions = []trigger.Action{}
	}
	if existing.Characters == nil {
		existing.Characters = []string{}
	}
	if existing.TimerAlerts == nil {
		existing.TimerAlerts = []trigger.TimerAlert{}
	}
	if existing.ExcludePatterns == nil {
		existing.ExcludePatterns = []string{}
	}
	if existing.ExtraPatterns == nil {
		existing.ExtraPatterns = []trigger.ExtraPattern{}
	}

	if err := h.store.Update(existing); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.engine.Reload()
	writeJSON(w, http.StatusOK, existing)
}

// del removes a trigger.
func (h *triggerHandler) del(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.store.Delete(id); err != nil {
		if errors.Is(err, trigger.ErrNotFound) {
			writeError(w, http.StatusNotFound, "trigger not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.engine.Reload()
	w.WriteHeader(http.StatusNoContent)
}

// clearAll removes every trigger in one statement, then runs a single engine
// reload — replacing the prior per-id fan-out from the frontend's Clear All
// button.
func (h *triggerHandler) clearAll(w http.ResponseWriter, r *http.Request) {
	if err := h.store.DeleteAll(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.engine.Reload()
	w.WriteHeader(http.StatusNoContent)
}

// history returns recent trigger firing events (newest last).
func (h *triggerHandler) history(w http.ResponseWriter, r *http.Request) {
	events := h.engine.GetHistory()
	if events == nil {
		events = []trigger.TriggerFired{}
	}
	writeJSON(w, http.StatusOK, events)
}

// test runs the Trigger Tester (see trigger/tester.go) against a pasted
// blob of log lines. By default (Realtime false) it runs synchronously and
// returns the full report in the response. With Realtime true, it instead
// starts a playback session that paces lines by their own log-timestamp
// gaps and streams results as trigger:test:line / trigger:test:status WS
// events; the response is 202 Accepted with no body.
//
// Character defaults to the live active character when omitted, matching
// how a real log line is attributed.
func (h *triggerHandler) test(w http.ResponseWriter, r *http.Request) {
	var req trigger.TestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if strings.TrimSpace(req.Lines) == "" {
		writeError(w, http.StatusBadRequest, "lines is required")
		return
	}
	if req.Character == "" {
		req.Character = h.activeCharacterName()
	}

	if !req.Realtime {
		writeJSON(w, http.StatusOK, h.engine.RunTest(req))
		return
	}

	if err := h.tester.Start(req); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	h.hub.Broadcast(ws.Event{Type: trigger.WSEventTriggerTestStatus, Data: map[string]any{"state": string(trigger.TestPlaybackPlaying)}})
	w.WriteHeader(http.StatusAccepted)
}

// testStop aborts an active real-time Trigger Tester session. No-op when
// idle.
func (h *triggerHandler) testStop(w http.ResponseWriter, r *http.Request) {
	h.tester.Stop()
	w.WriteHeader(http.StatusNoContent)
}

// testStatus returns the real-time Trigger Tester session's current state.
func (h *triggerHandler) testStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"state": string(h.tester.Status())})
}

// activeCharacterName returns the currently selected character — manual config
// override if set, otherwise the auto-detected character from the most-recent
// log file. Empty string when nothing is known.
func (h *triggerHandler) activeCharacterName() string {
	if h.cfgMgr != nil {
		if name := h.cfgMgr.Get().Character; name != "" {
			return name
		}
	}
	if h.tailer != nil {
		return h.tailer.ActiveCharacter()
	}
	return ""
}

// listCharacters returns every stored character (with class info), or nil when
// the store is unavailable or empty.
func (h *triggerHandler) listCharacters() []character.Character {
	if h.charStore == nil {
		return nil
	}
	chars, err := h.charStore.List()
	if err != nil {
		return nil
	}
	return chars
}

// applyDefaultCharacters fills in Characters for any trigger in the pack that
// doesn't already specify them, using the handler's character store and the
// currently active character as inputs.
func (h *triggerHandler) applyDefaultCharacters(pack *trigger.TriggerPack) {
	defaultPackCharacters(pack, h.listCharacters(), h.activeCharacterName())
}

// defaultPackCharacters is the pure decision logic for the default Characters
// list assigned to triggers on pack import. Extracted from the handler so it
// can be unit-tested without spinning up a config.Manager / tailer.
//
// Behavior depends on whether the pack is class-specific (e.g. "Beastlord")
// or class-agnostic (e.g. "General Triggers"):
//
//   - Class-agnostic pack (pack.Class == nil): default to all known characters.
//     The pack applies to anyone the user plays.
//   - Class-specific pack: default only to characters whose class matches the
//     pack. If the active character matches, prefer it alone (most likely the
//     character the user is importing the pack for); otherwise use every other
//     stored character whose class matches; otherwise (no character of this
//     class exists) leave Characters empty AND disable the triggers — the
//     user can later enable them and pick characters via the per-trigger chips.
//
// Triggers that already specify Characters are left untouched (Enabled is
// also untouched in that case — the pack author opted in explicitly).
func defaultPackCharacters(pack *trigger.TriggerPack, chars []character.Character, active string) {
	classAgnostic := pack.Class == nil

	var defaults []string
	if classAgnostic {
		defaults = make([]string, 0, len(chars))
		for _, c := range chars {
			if c.Name != "" {
				defaults = append(defaults, c.Name)
			}
		}
	} else {
		want := *pack.Class
		var matches []string
		var activeMatchName string
		for _, c := range chars {
			if c.Name == "" || c.Class != want {
				continue
			}
			if active != "" && strings.EqualFold(c.Name, active) {
				activeMatchName = c.Name
			}
			matches = append(matches, c.Name)
		}
		switch {
		case activeMatchName != "":
			defaults = []string{activeMatchName}
		case len(matches) > 0:
			defaults = matches
		}
	}

	for i := range pack.Triggers {
		if len(pack.Triggers[i].Characters) > 0 {
			continue // pack author scoped this trigger explicitly — respect it
		}
		if len(defaults) > 0 {
			pack.Triggers[i].Characters = append([]string(nil), defaults...)
			continue
		}
		// Class-specific pack with no character of the matching class.
		// Leave Characters empty and disable the trigger so it doesn't fire
		// for "any character" via the engine's legacy fallback.
		if !classAgnostic {
			pack.Triggers[i].Enabled = false
		}
	}
}

// importPack imports triggers from a JSON trigger pack in the request body.
// Existing triggers for the same pack_name are replaced.
func (h *triggerHandler) importPack(w http.ResponseWriter, r *http.Request) {
	var pack trigger.TriggerPack
	if err := json.NewDecoder(r.Body).Decode(&pack); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if pack.PackName == "" {
		writeError(w, http.StatusBadRequest, "pack_name is required")
		return
	}
	h.applyDefaultCharacters(&pack)
	if err := trigger.InstallPack(h.store, pack); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.engine.Reload()
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "pack_name": pack.PackName})
}

// exportPack exports all triggers as a JSON trigger pack.
func (h *triggerHandler) exportPack(w http.ResponseWriter, r *http.Request) {
	triggers, err := h.store.List()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	plain := make([]trigger.Trigger, len(triggers))
	for i, t := range triggers {
		plain[i] = *t
	}
	pack := trigger.TriggerPack{
		PackName:    "Custom Export",
		Description: "Exported from PQ Companion",
		Triggers:    plain,
	}
	writeJSON(w, http.StatusOK, pack)
}

// exportCategory exports all triggers filed under one category — including
// any subcategory's triggers, if it has children — as a JSON trigger pack, so
// a curated category (e.g. "Raid Triggers", with a subcategory per raid) can
// be shared and imported by others via the same Import wizard used for any
// other pack. Each trigger's pack_name is rewritten to its path relative to
// the exported root (empty for a trigger filed directly in the root, or the
// child's own name for one filed in a subcategory) — the same convention
// GINA's own nested export uses — so importCommit can rebuild the hierarchy
// under whatever category the importer chooses.
func (h *triggerHandler) exportCategory(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	cats, err := h.store.ListCategories()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var root *trigger.Category
	childName := make(map[string]string)
	ids := []string{id}
	for i := range cats {
		c := &cats[i]
		if c.ID == id {
			root = c
		} else if c.ParentID == id {
			ids = append(ids, c.ID)
			childName[c.ID] = c.Name
		}
	}
	if root == nil {
		writeError(w, http.StatusNotFound, "category not found")
		return
	}
	triggers, err := h.store.ListByCategoryIDs(ids)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(triggers) == 0 {
		writeError(w, http.StatusNotFound, "category has no triggers")
		return
	}
	plain := make([]trigger.Trigger, len(triggers))
	for i, t := range triggers {
		p := *t
		if p.CategoryID == id {
			p.PackName = ""
		} else {
			p.PackName = childName[p.CategoryID]
		}
		plain[i] = p
	}
	pack := trigger.TriggerPack{
		PackName:    root.Name,
		Description: "Exported from PQ Companion",
		Triggers:    plain,
	}
	writeJSON(w, http.StatusOK, pack)
}

// importPreview detects the source app of an uploaded trigger file (PQ
// Companion / GINA / EQNag / EQLogParser), parses it into a normalized preview,
// and returns it WITHOUT persisting anything. The wizard reviews/selects from
// the preview and then calls importCommit. The ?filename= query supplies the
// original name (used to suggest a category and as a weak format hint).
// maxImportUploadBytes caps the raw upload accepted by importPreview. Real
// GINA/EQNag/EQLogParser/PQC export files are at most a few MB; this is
// generous headroom while still bounding memory use against an oversized or
// hostile upload (the archive itself is further capped post-unzip in
// trigger.DetectAndParse).
const maxImportUploadBytes = 128 << 20 // 128 MiB

func (h *triggerHandler) importPreview(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxImportUploadBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "read body (max 128 MiB): "+err.Error())
		return
	}
	if len(body) == 0 {
		writeError(w, http.StatusBadRequest, "empty body")
		return
	}
	filename := r.URL.Query().Get("filename")
	preview, err := trigger.DetectAndParse(filename, body)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(preview.Triggers) == 0 {
		writeError(w, http.StatusBadRequest, "no triggers found in file")
		return
	}
	writeJSON(w, http.StatusOK, preview)
}

// importCommitRequest is the wizard's commit payload: the user-chosen subset of
// previewed triggers and the category to file them under.
type importCommitRequest struct {
	Category string            `json:"category"`
	Triggers []trigger.Trigger `json:"triggers"`
}

// importCommit installs a selected subset of previewed triggers into a category.
// The category is created if it doesn't exist; triggers are appended (not
// replaced) so several imports can share one category, and removal is via the
// category's Delete-all. Characters default to all known characters (imports
// are class-agnostic).
func (h *triggerHandler) importCommit(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxImportUploadBytes)
	var req importCommitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	category := strings.TrimSpace(req.Category)
	if category == "" {
		writeError(w, http.StatusBadRequest, "category is required")
		return
	}
	if len(req.Triggers) == 0 {
		writeError(w, http.StatusBadRequest, "no triggers selected")
		return
	}

	// "Parent/Child" files the import into a subcategory, creating the parent
	// alongside it if neither exists yet. Only the first "/" is significant —
	// matches the one-level nesting cap and the path convention exportCategory
	// and GINA's own nested groups (gina.go's walk()) already use. A reserved/
	// builtin name or an already-existing category at that spot is fine —
	// ResolveOrCreateCategory reuses it; only hard errors abort.
	segments := strings.SplitN(category, "/", 2)
	leafName := strings.TrimSpace(segments[0])
	parentID := ""
	if len(segments) == 2 && strings.TrimSpace(segments[1]) != "" {
		parent, err := h.store.ResolveOrCreateCategory("", leafName)
		if err != nil {
			writeCategoryError(w, err)
			return
		}
		parentID = parent.ID
		leafName = strings.TrimSpace(segments[1])
	}
	leaf, err := h.store.ResolveOrCreateCategory(parentID, leafName)
	if err != nil {
		writeCategoryError(w, err)
		return
	}

	// Default Characters via the shared (class-agnostic) pack logic.
	pack := trigger.TriggerPack{PackName: leaf.Name, Triggers: req.Triggers}
	h.applyDefaultCharacters(&pack)

	// Fetch the base sort order once: the whole batch is inserted in a single
	// transaction below, so a per-trigger NextTriggerSortOrder query would see
	// none of the pending rows and hand every trigger the same order. Offset by
	// the loop index instead to preserve the selected order.
	baseOrder := 0
	if order, err := h.store.NextTriggerSortOrder(leaf.Name); err == nil {
		baseOrder = order
	}
	triggers := make([]*trigger.Trigger, 0, len(pack.Triggers))
	for i := range pack.Triggers {
		t := pack.Triggers[i]
		t.CategoryID = leaf.ID
		t.SourcePack = "" // user-owned: removal is via the category, not a pack
		t.TimerType = normalizeTimerType(t.TimerType)
		// Re-validate the pattern here rather than trusting the earlier
		// /preview step: this endpoint accepts client-submitted trigger JSON
		// directly, so a hand-built commit request (bypassing the wizard)
		// could otherwise install a trigger with an uncompilable pattern that
		// silently never fires while still showing as enabled.
		if t.Pattern != "" && !trigger.ValidatePattern(t.Pattern) {
			t.Enabled = false
		}
		id, err := trigger.NewID()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		t.ID = id
		t.CreatedAt = time.Now().UTC()
		t.SortOrder = baseOrder + i
		if t.Actions == nil {
			t.Actions = []trigger.Action{}
		}
		if t.Characters == nil {
			t.Characters = []string{}
		}
		if t.TimerAlerts == nil {
			t.TimerAlerts = []trigger.TimerAlert{}
		}
		if t.ExcludePatterns == nil {
			t.ExcludePatterns = []string{}
		}
		if t.ExtraPatterns == nil {
			t.ExtraPatterns = []trigger.ExtraPattern{}
		}
		if t.Source == "" {
			t.Source = trigger.SourceLog
		}
		triggers = append(triggers, &t)
	}
	// All-or-nothing: a mid-batch failure rolls back every insert, so a retry
	// can't duplicate the survivors under fresh IDs.
	if err := h.store.InsertMany(triggers); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.engine.Reload()
	writeJSON(w, http.StatusOK, map[string]any{
		"status":   "ok",
		"category": category,
		"imported": len(triggers),
	})
}

// listBuiltinPacks returns all available pre-built trigger packs.
func (h *triggerHandler) listBuiltinPacks(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, trigger.AllPacks())
}

// ── Categories ───────────────────────────────────────────────────────────────
//
// Categories are trigger groupings, id-keyed (trigger_categories.id, linked
// via triggers.category_id) so a rename never cascades and a subcategory can
// share a name with one under a different parent. Nesting is capped at one
// level (see trigger.maxCategoryDepth): a top-level category may have
// children, a child may not. Custom categories persist in trigger_categories
// so an empty, freshly-created group survives a restart; built-in (class) and
// imported packs show up here too (every in-use category is materialized —
// see migrateCategoryHierarchy) but are flagged IsBuiltin and stay read-only
// — they're managed from the Packs tab. Deleting a category moves its own
// triggers to Uncategorized (or deletes them, if requested) and promotes any
// children to top-level; it never deletes a child category, which is the key
// difference from uninstalling a pack (removePack).

// listCategories returns all categories surfaced to the UI, as a flat list —
// the frontend assembles the tree from each category's parent_id.
func (h *triggerHandler) listCategories(w http.ResponseWriter, r *http.Request) {
	cats, err := h.store.ListCategories()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if cats == nil {
		cats = []trigger.Category{}
	}
	writeJSON(w, http.StatusOK, cats)
}

type categoryRequest struct {
	Name     string `json:"name"`
	ParentID string `json:"parent_id"`
}

// createCategory persists a new, empty custom category, optionally nested
// under parent_id.
func (h *triggerHandler) createCategory(w http.ResponseWriter, r *http.Request) {
	var req categoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	cat, err := h.store.CreateCategory(req.Name, req.ParentID)
	if err != nil {
		writeCategoryError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, cat)
}

type renameCategoryRequest struct {
	NewName string `json:"new_name"`
}

// renameCategory renames a category in place — its id doesn't change, so
// this never touches any other category or any trigger outside it.
func (h *triggerHandler) renameCategory(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req renameCategoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := h.store.RenameCategory(id, req.NewName); err != nil {
		writeCategoryError(w, err)
		return
	}
	h.engine.Reload()
	w.WriteHeader(http.StatusNoContent)
}

// splitCategory splits a top-level category's slash-containing name (e.g.
// left over from a GINA import, or from the pre-nesting flat category model)
// into a real parent/child pair. See trigger.Store.SplitCategoryName.
func (h *triggerHandler) splitCategory(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	cat, err := h.store.SplitCategoryName(id)
	if err != nil {
		writeCategoryError(w, err)
		return
	}
	h.engine.Reload()
	writeJSON(w, http.StatusOK, cat)
}

// deleteCategory removes a category. The ?triggers= query selects what
// happens to its own triggers: "delete" removes them outright (cascading to
// any children's triggers too), anything else (the default) moves just this
// category's triggers to Uncategorized. Either way, any children are
// promoted to top-level rather than deleted.
func (h *triggerHandler) deleteCategory(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	deleteTriggers := r.URL.Query().Get("triggers") == "delete"
	if err := h.store.DeleteCategory(id, deleteTriggers); err != nil {
		writeCategoryError(w, err)
		return
	}
	h.engine.Reload()
	w.WriteHeader(http.StatusNoContent)
}

type reorderCategoriesRequest struct {
	Items []trigger.CategoryPlacement `json:"items"`
}

// reorderCategories persists new positions and/or parents for a set of
// categories in one call — a plain reorder (parent unchanged) and a reparent
// (drag a category onto another) are both just entries in items.
func (h *triggerHandler) reorderCategories(w http.ResponseWriter, r *http.Request) {
	var req reorderCategoriesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := h.store.ReorderCategories(req.Items); err != nil {
		writeCategoryError(w, err)
		return
	}
	// Ordering doesn't affect matching, so no engine reload.
	w.WriteHeader(http.StatusNoContent)
}

type reorderTriggersRequest struct {
	IDs []string `json:"ids"`
}

// reorderTriggers persists a new manual order for the given trigger IDs
// (their position in the list becomes their sort_order).
func (h *triggerHandler) reorderTriggers(w http.ResponseWriter, r *http.Request) {
	var req reorderTriggersRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := h.store.ReorderTriggers(req.IDs); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Timer groups ──────────────────────────────────────────────────────────
//
// A timer group is a user-created Custom Timers window (see
// trigger.TimerGroup), letting raid leaders split signature-spell/boss
// timers into their own overlay separate from general trigger timers. CRUD
// mirrors the category endpoints above.

// listTimerGroups returns every user-created timer group.
func (h *triggerHandler) listTimerGroups(w http.ResponseWriter, r *http.Request) {
	groups, err := h.store.ListTimerGroups()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if groups == nil {
		groups = []trigger.TimerGroup{}
	}
	writeJSON(w, http.StatusOK, groups)
}

type timerGroupRequest struct {
	Name string `json:"name"`
}

// createTimerGroup persists a new, empty Custom Timers window.
func (h *triggerHandler) createTimerGroup(w http.ResponseWriter, r *http.Request) {
	var req timerGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	group, err := h.store.CreateTimerGroup(req.Name)
	if err != nil {
		writeTimerGroupError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, group)
}

type renameTimerGroupRequest struct {
	NewName string `json:"new_name"`
}

// renameTimerGroup renames a timer group in place.
func (h *triggerHandler) renameTimerGroup(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req renameTimerGroupRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := h.store.RenameTimerGroup(id, req.NewName); err != nil {
		writeTimerGroupError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// deleteTimerGroup removes a timer group. Its triggers fall back to the
// default Custom Timers window (custom_group_id = ”) — never deleted.
func (h *triggerHandler) deleteTimerGroup(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.store.DeleteTimerGroup(id); err != nil {
		writeTimerGroupError(w, err)
		return
	}
	h.engine.Reload()
	w.WriteHeader(http.StatusNoContent)
}

type reorderTimerGroupsRequest struct {
	Order []string `json:"order"`
}

// reorderTimerGroups persists a new display order for timer group windows.
func (h *triggerHandler) reorderTimerGroups(w http.ResponseWriter, r *http.Request) {
	var req reorderTimerGroupsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := h.store.ReorderTimerGroups(req.Order); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// writeTimerGroupError maps timer group sentinel errors to HTTP status codes.
func writeTimerGroupError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, trigger.ErrTimerGroupNameEmpty):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, trigger.ErrTimerGroupExists):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, trigger.ErrTimerGroupNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

// writeCategoryError maps category sentinel errors to HTTP status codes.
func writeCategoryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, trigger.ErrCategoryNameEmpty), errors.Is(err, trigger.ErrCategoryReserved),
		errors.Is(err, trigger.ErrCategoryDepth), errors.Is(err, trigger.ErrCategoryCycle),
		errors.Is(err, trigger.ErrCategoryNoSplit):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, trigger.ErrCategoryExists):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, trigger.ErrCategoryBuiltin):
		writeError(w, http.StatusForbidden, err.Error())
	case errors.Is(err, trigger.ErrCategoryNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

// ── Test overlay (positioning) ───────────────────────────────────────────────
//
// The Test/Position button in the overlay-text editor uses these endpoints to
// preview an alert in the live trigger overlay window and round-trip the new
// position back when the user drags the test card. No persistence happens
// here — the editor owns the unsaved form state and writes back via the
// regular update endpoint.

type testOverlayRequest struct {
	TestID       string `json:"test_id"`
	Text         string `json:"text"`
	Color        string `json:"color"`
	DurationSecs int    `json:"duration_secs"`
	FontSize     int    `json:"font_size,omitempty"`
	// GlowColor / FontFamily complete the style so the positioning card
	// doubles as a live preview. The editor sends RESOLVED values (override →
	// global default → built-in already applied), so the overlay renders them
	// as-is. Re-posting the same test_id restyles the card in place.
	GlowColor  string                  `json:"glow_color,omitempty"`
	FontFamily string                  `json:"font_family,omitempty"`
	Align      string                  `json:"align,omitempty"`
	Position   *trigger.ActionPosition `json:"position,omitempty"`
}

func (h *triggerHandler) testOverlay(w http.ResponseWriter, r *http.Request) {
	var req testOverlayRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.TestID == "" {
		writeError(w, http.StatusBadRequest, "test_id is required")
		return
	}
	h.testMu.Lock()
	stored := req
	h.latestTest = &stored
	h.testMu.Unlock()
	h.hub.Broadcast(ws.Event{Type: "trigger:test", Data: req})
	w.WriteHeader(http.StatusNoContent)
}

// testOverlayActive lets a freshly-mounted trigger overlay hydrate without
// waiting for the next broadcast. Returns the latest in-flight test session,
// or JSON null when no session is active.
func (h *triggerHandler) testOverlayActive(w http.ResponseWriter, r *http.Request) {
	h.testMu.Lock()
	defer h.testMu.Unlock()
	if h.latestTest == nil {
		writeJSON(w, http.StatusOK, nil)
		return
	}
	writeJSON(w, http.StatusOK, h.latestTest)
}

type testOverlayPositionRequest struct {
	TestID   string                 `json:"test_id"`
	Position trigger.ActionPosition `json:"position"`
}

func (h *triggerHandler) testOverlayPosition(w http.ResponseWriter, r *http.Request) {
	var req testOverlayPositionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.TestID == "" {
		writeError(w, http.StatusBadRequest, "test_id is required")
		return
	}
	// Keep the cached active test in sync with drag updates so an overlay
	// hydrating mid-session sees the latest position.
	h.testMu.Lock()
	if h.latestTest != nil && h.latestTest.TestID == req.TestID {
		pos := req.Position
		h.latestTest.Position = &pos
	}
	h.testMu.Unlock()
	h.hub.Broadcast(ws.Event{Type: "trigger:test_position", Data: req})
	w.WriteHeader(http.StatusNoContent)
}

type testOverlayEndRequest struct {
	TestID string `json:"test_id"`
	// Cancelled distinguishes a cancel (revert to the pre-session position)
	// from a confirm (keep the dragged position). The backend doesn't act on
	// it — it's relayed verbatim in the broadcast so the trigger editor can
	// decide whether to revert, regardless of which window ended the session.
	Cancelled bool `json:"cancelled,omitempty"`
}

func (h *triggerHandler) testOverlayEnd(w http.ResponseWriter, r *http.Request) {
	var req testOverlayEndRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.TestID == "" {
		writeError(w, http.StatusBadRequest, "test_id is required")
		return
	}
	h.testMu.Lock()
	if h.latestTest != nil && h.latestTest.TestID == req.TestID {
		h.latestTest = nil
	}
	h.testMu.Unlock()
	h.hub.Broadcast(ws.Event{Type: "trigger:test_session_ended", Data: req})
	w.WriteHeader(http.StatusNoContent)
}

// ── Timer fading-soon overlay alerts ─────────────────────────────────────────
//
// A trigger's "fading soon" thresholds (Trigger.TimerAlerts) are audio-only
// on the backend — the spelltimer sink just carries the resolved TimerAlert
// list along on each ActiveTimer, and the frontend's useTimerAlerts hook
// detects the remaining-seconds crossing entirely client-side (it already
// has everything it needs: the timer's own countdown). For an overlay_text
// threshold, that crossing-detection still happens client-side, but the
// actual popup has to render in the separate trigger-overlay Electron
// window — so the hook POSTs here, and this reuses the exact same
// trigger:fired broadcast + Action shape the trigger overlay window already
// knows how to render (dedup, stacking, style, pinned position and all),
// rather than teaching it a second event type.
type fireTimerAlertOverlayRequest struct {
	// TimerID scopes the trigger-overlay window's dedup key (see
	// DEDUP_WINDOW_MS in TriggerOverlayWindowPage) to this specific active
	// timer instance, not the underlying Trigger — two instances of the same
	// mez trigger on two different mobs must not suppress each other.
	TimerID      string                  `json:"timer_id"`
	Text         string                  `json:"text"`
	Color        string                  `json:"color"`
	DurationSecs float64                 `json:"duration_secs"`
	FontSize     int                     `json:"font_size,omitempty"`
	GlowColor    string                  `json:"glow_color,omitempty"`
	FontFamily   string                  `json:"font_family,omitempty"`
	Align        string                  `json:"align,omitempty"`
	Position     *trigger.ActionPosition `json:"position,omitempty"`
}

func (h *triggerHandler) fireTimerAlertOverlay(w http.ResponseWriter, r *http.Request) {
	var req fireTimerAlertOverlayRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if strings.TrimSpace(req.Text) == "" {
		writeError(w, http.StatusBadRequest, "text is required")
		return
	}
	event := trigger.TriggerFired{
		TriggerID:   req.TimerID,
		TriggerName: req.Text,
		Actions: []trigger.Action{{
			Type:         trigger.ActionOverlayText,
			Text:         req.Text,
			DurationSecs: req.DurationSecs,
			Color:        req.Color,
			FontSize:     req.FontSize,
			GlowColor:    req.GlowColor,
			FontFamily:   req.FontFamily,
			Align:        req.Align,
			Position:     req.Position,
		}},
		FiredAt: time.Now(),
	}
	h.hub.Broadcast(ws.Event{Type: trigger.WSEventTriggerFired, Data: event})
	w.WriteHeader(http.StatusNoContent)
}

// installBuiltinPack installs the named pre-built pack, replacing any existing
// triggers for that pack.
func (h *triggerHandler) installBuiltinPack(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	var found *trigger.TriggerPack
	for _, p := range trigger.AllPacks() {
		if p.PackName == name {
			p := p // capture loop var
			found = &p
			break
		}
	}
	if found == nil {
		writeError(w, http.StatusNotFound, "pack not found")
		return
	}
	h.applyDefaultCharacters(found)
	if err := trigger.InstallPack(h.store, *found); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.engine.Reload()
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "pack_name": found.PackName})
}

// listPackUpdates returns a summary of pending updates for every installed
// built-in pack — the shipped definitions changed since the pack was
// installed/last updated. Drives the "updates available" banner + per-pack
// badges on the Packs tab.
func (h *triggerHandler) listPackUpdates(w http.ResponseWriter, r *http.Request) {
	summaries, err := trigger.ComputeUpdateSummaries(h.store)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if summaries == nil {
		summaries = []trigger.PackUpdateSummary{}
	}
	writeJSON(w, http.StatusOK, summaries)
}

// packDiff returns the full changelist for one installed built-in pack:
// per-trigger changed/added/removed/deleted-locally groups with field-level
// old/new/current values for the side-by-side compare view.
func (h *triggerHandler) packDiff(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	pack := findBuiltinPack(name)
	if pack == nil {
		writeError(w, http.StatusNotFound, "pack not found")
		return
	}
	diff, err := trigger.ComputePackDiff(h.store, *pack)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, diff)
}

type applyPackUpdateRequest struct {
	// Mode: "preserve" (default — user-customized fields, actions and
	// character lists keep their values) or "reset" (full pack defaults).
	Mode string `json:"mode"`
	// Keys selects which triggers (pack_key) to update; empty = everything
	// pending except locally-deleted triggers, which are opt-in only.
	Keys []string `json:"keys"`
}

// applyPackUpdate applies pending pack updates to the user's installed
// triggers, per the requested mode and selection.
func (h *triggerHandler) applyPackUpdate(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	pack := findBuiltinPack(name)
	if pack == nil {
		writeError(w, http.StatusNotFound, "pack not found")
		return
	}
	var req applyPackUpdateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if req.Mode == "" {
		req.Mode = trigger.UpdateModePreserve
	}
	// Inserted/reset triggers need per-user character defaults, same as a
	// fresh pack install; the raw definition stays pristine for baselines.
	defaulted := *pack
	defaulted.Triggers = append([]trigger.Trigger(nil), pack.Triggers...)
	h.applyDefaultCharacters(&defaulted)
	res, err := trigger.ApplyPackUpdate(h.store, *pack, defaulted, req.Mode, req.Keys)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.engine.Reload()
	writeJSON(w, http.StatusOK, res)
}

// ── Action templates ─────────────────────────────────────────────────────────
//
// Named, reusable Actions lists managed from the trigger editor's Templates
// menu. At most one is the default; its actions prefill new triggers.

func (h *triggerHandler) listActionTemplates(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.ListActionTemplates()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if list == nil {
		list = []*trigger.ActionTemplate{}
	}
	writeJSON(w, http.StatusOK, list)
}

type actionTemplateRequest struct {
	Name      string           `json:"name"`
	Actions   []trigger.Action `json:"actions"`
	IsDefault bool             `json:"is_default"`
}

func (h *triggerHandler) createActionTemplate(w http.ResponseWriter, r *http.Request) {
	var req actionTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	t := trigger.ActionTemplate{
		Name:      strings.TrimSpace(req.Name),
		Actions:   req.Actions,
		IsDefault: req.IsDefault,
	}
	if err := h.store.CreateActionTemplate(&t); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

func (h *triggerHandler) updateActionTemplate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	existing, err := h.store.GetActionTemplate(id)
	if err != nil {
		if errors.Is(err, trigger.ErrTemplateNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	var req actionTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if strings.TrimSpace(req.Name) != "" {
		existing.Name = strings.TrimSpace(req.Name)
	}
	if req.Actions != nil {
		existing.Actions = req.Actions
	}
	existing.IsDefault = req.IsDefault
	if err := h.store.UpdateActionTemplate(existing); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, existing)
}

func (h *triggerHandler) deleteActionTemplate(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if err := h.store.DeleteActionTemplate(id); err != nil {
		if errors.Is(err, trigger.ErrTemplateNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Bulk action edits ────────────────────────────────────────────────────────

type bulkActionsRequest struct {
	TriggerIDs []string `json:"trigger_ids"`
	// Operation: "apply_template" replaces each trigger's actions with
	// Actions; "tts_to_sound" converts text_to_speech actions (and,
	// optionally, TTS timer alerts) to play_sound with SoundPath/Volume.
	Operation          string           `json:"operation"`
	Actions            []trigger.Action `json:"actions"`
	SoundPath          string           `json:"sound_path"`
	Volume             float64          `json:"volume"`
	IncludeTimerAlerts bool             `json:"include_timer_alerts"`
}

func (h *triggerHandler) bulkEditActions(w http.ResponseWriter, r *http.Request) {
	var req bulkActionsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if len(req.TriggerIDs) == 0 {
		writeError(w, http.StatusBadRequest, "trigger_ids is required")
		return
	}
	// Validate user input up front so the only errors the bulk ops can return
	// are store failures (mapped to 500 below, not 400).
	if req.Operation == "tts_to_sound" && req.SoundPath == "" {
		writeError(w, http.StatusBadRequest, "sound_path is required")
		return
	}
	var res *trigger.BulkResult
	var err error
	switch req.Operation {
	case "apply_template":
		res, err = trigger.BulkApplyActions(h.store, req.TriggerIDs, req.Actions)
	case "tts_to_sound":
		res, err = trigger.BulkConvertTTSToSound(h.store, req.TriggerIDs, req.SoundPath, req.Volume, req.IncludeTimerAlerts)
	default:
		writeError(w, http.StatusBadRequest, "unknown operation")
		return
	}
	// Reload regardless of outcome: a mid-loop failure may have committed some
	// updates, so the running engine must re-sync with the DB either way. Both
	// bulk ops are idempotent, so a retry after a partial failure is safe.
	h.engine.Reload()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// resetAllPositions clears the pinned position on every trigger's
// overlay_text actions, so they all fall back to the global default set on
// the Settings page.
func (h *triggerHandler) resetAllPositions(w http.ResponseWriter, r *http.Request) {
	res, err := trigger.ClearAllPositions(h.store)
	h.engine.Reload()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// findBuiltinPack returns the named built-in pack definition, or nil.
func findBuiltinPack(name string) *trigger.TriggerPack {
	for _, p := range trigger.AllPacks() {
		if p.PackName == name {
			return &p
		}
	}
	return nil
}

// removePack removes all triggers belonging to the named pack and then
// re-installs any shared dedup_key triggers that another still-installed
// pack would have provided. Used by both built-in packs and user-imported
// packs — anything stored with a pack_name can be removed in one shot.
func (h *triggerHandler) removePack(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "pack name is required")
		return
	}
	installed, err := h.store.InstalledPackNames()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	delete(installed, name)
	if err := trigger.UninstallPack(h.store, name, installed); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.engine.Reload()
	w.WriteHeader(http.StatusNoContent)
}
