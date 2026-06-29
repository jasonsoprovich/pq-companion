package api

import (
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
	"github.com/jasonsoprovich/pq-companion/backend/internal/threat"
)

type threatHandler struct {
	tracker *threat.Tracker
}

// state handles GET /api/overlay/threat.
// Returns the current personal threat estimate: per-mob hate and the
// highlighted (current-target) mob.
func (h *threatHandler) state(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.tracker.GetState())
}

// reset handles POST /api/threat/reset.
// Clears all accumulated hate (the overlay's manual reset button).
func (h *threatHandler) reset(w http.ResponseWriter, r *http.Request) {
	h.tracker.Reset()
	w.WriteHeader(http.StatusNoContent)
}

// removeMob handles DELETE /api/overlay/threat/{name}.
// Drops a single mob from the threat list (the overlay's per-row "x" button).
func (h *threatHandler) removeMob(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if decoded, err := url.PathUnescape(name); err == nil {
		name = decoded
	}
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}
	if !h.tracker.RemoveMob(name) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
