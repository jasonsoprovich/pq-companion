package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jasonsoprovich/pq-companion/backend/internal/popflag"
)

// popflagHandler serves the curated PoP flag dataset plus per-character
// progress. The store may be nil when user.db failed to open; dataset reads
// still work, but per-character reads/writes respond 503.
type popflagHandler struct {
	store *popflag.Store
}

// GET /api/popflags/dataset
// Returns the embedded curated dataset (the frontend's source of truth).
func (h *popflagHandler) dataset(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"flags": popflag.Flags()})
}

// GET /api/popflags/{character}
// Returns the resolved per-flag status + progress for one character.
func (h *popflagHandler) get(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "pop flag store unavailable")
		return
	}
	character := strings.TrimSpace(chi.URLParam(r, "character"))
	if character == "" {
		writeError(w, http.StatusBadRequest, "character required")
		return
	}
	states, err := h.store.Get(character)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, popflag.Resolve(states))
}

type popflagSetRequest struct {
	Done bool `json:"done"`
}

// POST /api/popflags/{character}/{flagID}
// Records a manual toggle (done=true confirms, done=false retracts).
func (h *popflagHandler) setManual(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "pop flag store unavailable")
		return
	}
	character := strings.TrimSpace(chi.URLParam(r, "character"))
	flagID := strings.TrimSpace(chi.URLParam(r, "flagID"))
	if character == "" || flagID == "" {
		writeError(w, http.StatusBadRequest, "character and flagID required")
		return
	}
	var req popflagSetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := h.store.SetManual(character, flagID, req.Done); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	states, err := h.store.Get(character)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, popflag.Resolve(states))
}
