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
	Name                 string               `json:"name"`
	Enabled              bool                 `json:"enabled"`
	Pattern              string               `json:"pattern"`
	Actions              []trigger.Action     `json:"actions"`
	TimerType            trigger.TimerType    `json:"timer_type"`
	TimerDurationSecs    int                  `json:"timer_duration_secs"`
	WornOffPattern       string               `json:"worn_off_pattern"`
	SpellID              int                  `json:"spell_id"`
	DisplayThresholdSecs int                  `json:"display_threshold_secs"`
	Characters           []string             `json:"characters"`
	TimerAlerts          []trigger.TimerAlert `json:"timer_alerts"`
	ExcludePatterns      []string             `json:"exclude_patterns"`
}

// normalizeTimerType coerces an incoming timer_type into one of the valid
// values, defaulting to "none" for anything else (including blank).
func normalizeTimerType(t trigger.TimerType) trigger.TimerType {
	switch t {
	case trigger.TimerTypeBuff, trigger.TimerTypeDetrimental:
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
	if req.Name == "" || req.Pattern == "" {
		writeError(w, http.StatusBadRequest, "name and pattern are required")
		return
	}

	id, err := trigger.NewID()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	t := &trigger.Trigger{
		ID:                   id,
		Name:                 req.Name,
		Enabled:              req.Enabled,
		Pattern:              req.Pattern,
		Actions:              req.Actions,
		CreatedAt:            time.Now().UTC(),
		TimerType:            normalizeTimerType(req.TimerType),
		TimerDurationSecs:    req.TimerDurationSecs,
		WornOffPattern:       req.WornOffPattern,
		SpellID:              req.SpellID,
		DisplayThresholdSecs: req.DisplayThresholdSecs,
		Characters:           req.Characters,
		TimerAlerts:          req.TimerAlerts,
		ExcludePatterns:      req.ExcludePatterns,
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
	if req.Name == "" || req.Pattern == "" {
		writeError(w, http.StatusBadRequest, "name and pattern are required")
		return
	}

	existing.Name = req.Name
	existing.Enabled = req.Enabled
	existing.Pattern = req.Pattern
	existing.Actions = req.Actions
	existing.TimerType = normalizeTimerType(req.TimerType)
	existing.TimerDurationSecs = req.TimerDurationSecs
	existing.WornOffPattern = req.WornOffPattern
	existing.SpellID = req.SpellID
	existing.DisplayThresholdSecs = req.DisplayThresholdSecs
	existing.Characters = req.Characters
	existing.TimerAlerts = req.TimerAlerts
	existing.ExcludePatterns = req.ExcludePatterns
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
// or class-agnostic (e.g. "Group Awareness"):
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

// importGINA imports triggers from a GINA share XML document in the request
// body. The pack_name is taken from the ?pack_name= query param or a default.
func (h *triggerHandler) importGINA(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "read body: "+err.Error())
		return
	}
	if len(body) == 0 {
		writeError(w, http.StatusBadRequest, "empty body")
		return
	}
	packName := r.URL.Query().Get("pack_name")
	pack, err := trigger.ParseGINA(body, packName)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if len(pack.Triggers) == 0 {
		writeError(w, http.StatusBadRequest, "no triggers found in GINA document")
		return
	}
	h.applyDefaultCharacters(&pack)
	if err := trigger.InstallPack(h.store, pack); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.engine.Reload()
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "ok",
		"pack_name": pack.PackName,
		"imported":  len(pack.Triggers),
	})
}

// listBuiltinPacks returns all available pre-built trigger packs.
func (h *triggerHandler) listBuiltinPacks(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, trigger.AllPacks())
}

// ── Test overlay (positioning) ───────────────────────────────────────────────
//
// The Test/Position button in the overlay-text editor uses these endpoints to
// preview an alert in the live trigger overlay window and round-trip the new
// position back when the user drags the test card. No persistence happens
// here — the editor owns the unsaved form state and writes back via the
// regular update endpoint.

type testOverlayRequest struct {
	TestID       string                  `json:"test_id"`
	Text         string                  `json:"text"`
	Color        string                  `json:"color"`
	DurationSecs int                     `json:"duration_secs"`
	FontSize     int                     `json:"font_size,omitempty"`
	Position     *trigger.ActionPosition `json:"position,omitempty"`
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

// removePack removes all triggers belonging to the named pack. Used by both
// built-in packs and user-imported packs — anything stored with a pack_name
// can be removed in one shot.
func (h *triggerHandler) removePack(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "pack name is required")
		return
	}
	if err := h.store.DeleteByPack(name); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.engine.Reload()
	w.WriteHeader(http.StatusNoContent)
}
