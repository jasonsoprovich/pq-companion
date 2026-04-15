package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jasonsoprovich/pq-companion/backend/internal/trigger"
)

type triggerHandler struct {
	store  *trigger.Store
	engine *trigger.Engine
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

// create adds a new trigger.
func (h *triggerHandler) create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name    string           `json:"name"`
		Enabled bool             `json:"enabled"`
		Pattern string           `json:"pattern"`
		Actions []trigger.Action `json:"actions"`
	}
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
		ID:        id,
		Name:      req.Name,
		Enabled:   req.Enabled,
		Pattern:   req.Pattern,
		Actions:   req.Actions,
		CreatedAt: time.Now().UTC(),
	}
	if t.Actions == nil {
		t.Actions = []trigger.Action{}
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

	var req struct {
		Name    string           `json:"name"`
		Enabled bool             `json:"enabled"`
		Pattern string           `json:"pattern"`
		Actions []trigger.Action `json:"actions"`
	}
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
	if existing.Actions == nil {
		existing.Actions = []trigger.Action{}
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

// history returns recent trigger firing events (newest last).
func (h *triggerHandler) history(w http.ResponseWriter, r *http.Request) {
	events := h.engine.GetHistory()
	if events == nil {
		events = []trigger.TriggerFired{}
	}
	writeJSON(w, http.StatusOK, events)
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

// listBuiltinPacks returns all available pre-built trigger packs.
func (h *triggerHandler) listBuiltinPacks(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, trigger.AllPacks())
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
	if err := trigger.InstallPack(h.store, *found); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.engine.Reload()
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "pack_name": found.PackName})
}
