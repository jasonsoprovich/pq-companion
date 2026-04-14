package api

import (
	"net/http"

	"github.com/jasonsoprovich/pq-companion/backend/internal/overlay"
)

type overlayHandler struct {
	npcTracker *overlay.NPCTracker
}

// npcTarget handles GET /api/overlay/npc/target.
// Returns the current inferred combat target and its NPC data (or has_target=false).
func (h *overlayHandler) npcTarget(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.npcTracker.GetState())
}
