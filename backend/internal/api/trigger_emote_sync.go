package api

import (
	"encoding/json"
	"net/http"

	"github.com/jasonsoprovich/pq-companion/backend/internal/trigger"
)

// POST /api/triggers/emote-sync/suggestions
// Body: { "spell_id": 123, "changes": [{"field","old","new"}, ...] }
// Returns every trigger linked to spell_id (via Trigger.SpellID) whose
// Pattern/WornOffPattern/ExtraPatterns contains one of changes' old emote
// text, with the suggested replacement — never applied automatically. Used
// by the Spell Emote Customizer to flag triggers that may need updating
// after an emote edit.
func (h *triggerHandler) emoteSyncSuggestions(w http.ResponseWriter, r *http.Request) {
	var body struct {
		SpellID int                   `json:"spell_id"`
		Changes []trigger.EmoteChange `json:"changes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	suggestions, err := trigger.SuggestPatternUpdates(h.store, body.SpellID, body.Changes)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, suggestions)
}

// POST /api/triggers/emote-sync/apply
// Body: { "trigger_id", "location", "extra_index", "old", "new" }
// Applies one suggested pattern replacement and records an audit entry so
// it can be reverted. Fails (without changing anything) if the trigger's
// pattern no longer literally contains "old" — e.g. it was hand-edited
// again since the suggestion was computed.
func (h *triggerHandler) emoteSyncApply(w http.ResponseWriter, r *http.Request) {
	var body struct {
		TriggerID  string                  `json:"trigger_id"`
		Location   trigger.PatternLocation `json:"location"`
		ExtraIndex int                     `json:"extra_index"`
		Old        string                  `json:"old"`
		New        string                  `json:"new"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if body.TriggerID == "" {
		writeError(w, http.StatusBadRequest, "trigger_id required")
		return
	}
	auditID, err := h.store.ApplyPatternUpdate(body.TriggerID, body.Location, body.ExtraIndex, body.Old, body.New)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"audit_id": auditID})
}

// POST /api/triggers/emote-sync/revert
// Body: { "audit_id" }
// Restores the pattern text an earlier emoteSyncApply changed.
func (h *triggerHandler) emoteSyncRevert(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AuditID string `json:"audit_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.store.RevertPatternUpdate(body.AuditID); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
