package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jasonsoprovich/pq-companion/backend/internal/spelltimer"
)

type timerHandler struct {
	engine *spelltimer.Engine
}

// state handles GET /api/overlay/timers — returns all active spell timers.
func (h *timerHandler) state(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.engine.GetState())
}

// clear handles POST /api/overlay/timers/clear — removes active timers in the
// given category group. The ?category= query parameter accepts "buff",
// "detrimental", "all", or empty (treated as "all").
func (h *timerHandler) clear(w http.ResponseWriter, r *http.Request) {
	group := r.URL.Query().Get("category")
	switch group {
	case "", "all", "buff", "detrimental":
		// accepted
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "category must be one of: buff, detrimental, all",
		})
		return
	}
	h.engine.ClearCategory(group)
	w.WriteHeader(http.StatusNoContent)
}

// remove handles DELETE /api/overlay/timers/{id} — removes a single active
// timer by its composite ID. Returns 404 if the ID isn't currently active.
func (h *timerHandler) remove(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
		return
	}
	if !h.engine.RemoveByID(id) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
