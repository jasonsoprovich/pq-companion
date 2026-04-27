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
