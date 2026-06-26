package api

import (
	"net/http"

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
