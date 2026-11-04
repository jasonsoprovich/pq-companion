package api

import (
	"net/http"

	"github.com/jasonsoprovich/pq-companion/backend/internal/regen"
)

type regenHandler struct {
	tracker *regen.Tracker
}

// state handles GET /api/overlay/regen?character=<name>.
// Returns the character's HP/mana regen per tick measured this session.
func (h *regenHandler) state(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.tracker.Snapshot(r.URL.Query().Get("character")))
}
