package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/jasonsoprovich/pq-companion/backend/internal/overlay"
	"github.com/jasonsoprovich/pq-companion/backend/internal/petoverlay"
)

type overlayHandler struct {
	npcTracker *overlay.NPCTracker
	petTracker *petoverlay.Tracker
}

// petState handles GET /api/overlay/pet.
// Returns the active character's pet state (mode "none" when there is no pet).
func (h *overlayHandler) petState(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.petTracker.GetState())
}

// npcTarget handles GET /api/overlay/npc/target.
// Returns the current inferred combat target and its NPC data (or has_target=false).
func (h *overlayHandler) npcTarget(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.npcTracker.GetState())
}

// npcDistance handles GET /api/overlay/npc/distance.
// Returns the latest player→target distance (distance null when unknown).
func (h *overlayHandler) npcDistance(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.npcTracker.GetDistance())
}

// npcPreview handles GET /api/overlay/npc/preview/{id}.
// Returns a target state for any NPC id, for the Settings overlay preview.
func (h *overlayHandler) npcPreview(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	st, err := h.npcTracker.PreviewState(id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "npc not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, st)
}
