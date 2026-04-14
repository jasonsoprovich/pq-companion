package api

import (
	"net/http"

	"github.com/jasonsoprovich/pq-companion/backend/internal/combat"
)

type combatHandler struct {
	tracker *combat.Tracker
}

// state handles GET /api/overlay/combat.
// Returns the current combat state: active fight, recent fights, and session DPS.
func (h *combatHandler) state(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.tracker.GetState())
}
