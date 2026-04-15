package api

import (
	"net/http"

	"github.com/jasonsoprovich/pq-companion/backend/internal/spelltimer"
)

type timerHandler struct {
	engine *spelltimer.Engine
}

// state handles GET /api/overlay/timers — returns all active spell timers.
func (h *timerHandler) state(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.engine.GetState())
}
