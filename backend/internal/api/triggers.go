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
	Name                 string                 `json:"name"`
	Enabled              bool                   `json:"enabled"`
	Pattern              string                 `json:"pattern"`
	Actions              []trigger.Action       `json:"actions"`
	TimerType            trigger.TimerType      `json:"timer_type"`
	TimerDurationSecs    int                    `json:"timer_duration_secs"`
	TimerDurationCapture string                 `json:"timer_duration_capture"`
	TimerKeyCapture      string                 `json:"timer_key_capture"`
	TimerTargetCapture   string                 `json:"timer_target_capture"`
	WornOffPattern       string                 `json:"worn_off_pattern"`
	SpellID              int                    `json:"spell_id"`
	DisplayThresholdSecs int                    `json:"display_threshold_secs"`
	BarColor             string                 `json:"bar_color"`
	Characters           []string               `json:"characters"`
	TimerAlerts          []trigger.TimerAlert   `json:"timer_alerts"`
	ExcludePatterns      []string               `json:"exclude_patterns"`
	ExtraPatterns        []trigger.ExtraPattern `json:"extra_patterns"`
	Source               string                 `json:"source,omitempty"`
	PipeCondition        *trigger.PipeCondition `json:"pipe_condition,omitempty"`
	// PackName is the trigger's category. Pointer so an omitted field on
	// update leaves the existing category untouched (a present value — even
	// "" for Uncategorized — replaces it). nil on create defaults to "".
	PackName *string `json:"pack_name,omitempty"`
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
	t := &trigger.Trigger{
		ID:                   id,
		Name:                 req.Name,
		Enabled:              req.Enabled,
		Pattern:              req.Pattern,
		Actions:              req.Actions,
		PackName:             packName,
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
		Characters:           req.Characters,
		TimerAlerts:          req.TimerAlerts,
		ExcludePatterns:      req.ExcludePatterns,
		ExtraPatterns:        req.ExtraPatterns,
		Source:               src,
		PipeCondition:        req.PipeCondition,
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
	existing.DisplayThresholdSecs = req.DisplayThresholdSecs
	existing.BarColor = strings.TrimSpace(req.BarColor)
	existing.Characters = req.Characters
	existing.TimerAlerts = req.TimerAlerts
	existing.ExcludePatterns = req.ExcludePatterns
	existing.ExtraPatterns = req.ExtraPatterns
	existing.Source = src
	existing.PipeCondition = req.PipeCondition
	// Only touch the category when the request carries pack_name — an
	// omitted field (older callers, edits that don't change category)
	// leaves the existing value intact. On a category change, re-append to
	// the end of the destination's manual order.
	if req.PackName != nil {
		newPack := strings.TrimSpace(*req.PackName)
		if newPack != existing.PackName {
			existing.PackName = newPack
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

// importPreview detects the source app of an uploaded trigger file (PQ
// Companion / GINA / EQNag / EQLogParser), parses it into a normalized preview,
// and returns it WITHOUT persisting anything. The wizard reviews/selects from
// the preview and then calls importCommit. The ?filename= query supplies the
// original name (used to suggest a category and as a weak format hint).
func (h *triggerHandler) importPreview(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "read body: "+err.Error())
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

	// Ensure the destination category exists. A reserved/builtin name or an
	// already-existing custom category is fine — only hard errors abort.
	if _, err := h.store.CreateCategory(category); err != nil &&
		!errors.Is(err, trigger.ErrCategoryExists) {
		writeCategoryError(w, err)
		return
	}

	// Default Characters via the shared (class-agnostic) pack logic.
	pack := trigger.TriggerPack{PackName: category, Triggers: req.Triggers}
	h.applyDefaultCharacters(&pack)

	imported := 0
	for i := range pack.Triggers {
		t := pack.Triggers[i]
		t.PackName = category
		t.SourcePack = "" // user-owned: removal is via the category, not a pack
		t.TimerType = normalizeTimerType(t.TimerType)
		id, err := trigger.NewID()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		t.ID = id
		t.CreatedAt = time.Now().UTC()
		if order, err := h.store.NextTriggerSortOrder(category); err == nil {
			t.SortOrder = order
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
		if t.Source == "" {
			t.Source = trigger.SourceLog
		}
		if err := h.store.Insert(&t); err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		imported++
	}
	h.engine.Reload()
	writeJSON(w, http.StatusOK, map[string]any{
		"status":   "ok",
		"category": category,
		"imported": imported,
	})
}

// listBuiltinPacks returns all available pre-built trigger packs.
func (h *triggerHandler) listBuiltinPacks(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, trigger.AllPacks())
}

// ── Categories ───────────────────────────────────────────────────────────────
//
// Categories are trigger groupings keyed off the pack_name column. Custom
// categories persist in the trigger_categories table so an empty, freshly-
// created group survives a restart; built-in (class) and imported packs show
// up here too (derived from in-use pack_name values) but are flagged
// IsBuiltin and stay read-only — they're managed from the Packs tab. Deleting
// a category moves its triggers to Uncategorized rather than deleting them,
// which is the key difference from uninstalling a pack (removePack).

// listCategories returns all categories surfaced to the UI.
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
	Name string `json:"name"`
}

// createCategory persists a new, empty custom category.
func (h *triggerHandler) createCategory(w http.ResponseWriter, r *http.Request) {
	var req categoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	cat, err := h.store.CreateCategory(req.Name)
	if err != nil {
		writeCategoryError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, cat)
}

type renameCategoryRequest struct {
	NewName string `json:"new_name"`
}

// renameCategory renames a custom category, cascading to its triggers.
func (h *triggerHandler) renameCategory(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	var req renameCategoryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := h.store.RenameCategory(name, req.NewName); err != nil {
		writeCategoryError(w, err)
		return
	}
	h.engine.Reload()
	w.WriteHeader(http.StatusNoContent)
}

// deleteCategory removes a custom category. The ?triggers= query selects what
// happens to its triggers: "delete" removes them outright, anything else (the
// default) moves them to Uncategorized.
func (h *triggerHandler) deleteCategory(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	deleteTriggers := r.URL.Query().Get("triggers") == "delete"
	if err := h.store.DeleteCategory(name, deleteTriggers); err != nil {
		writeCategoryError(w, err)
		return
	}
	h.engine.Reload()
	w.WriteHeader(http.StatusNoContent)
}

type reorderCategoriesRequest struct {
	Order []string `json:"order"`
}

// reorderCategories persists a new display order for the category sections.
func (h *triggerHandler) reorderCategories(w http.ResponseWriter, r *http.Request) {
	var req reorderCategoriesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := h.store.ReorderCategories(req.Order); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
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

// writeCategoryError maps category sentinel errors to HTTP status codes.
func writeCategoryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, trigger.ErrCategoryNameEmpty), errors.Is(err, trigger.ErrCategoryReserved):
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
