package api

import (
	"net/http"

	"github.com/jasonsoprovich/pq-companion/backend/internal/raidthreat"
)

type raidThreatHandler struct {
	assembler *raidthreat.Assembler
}

// state handles GET /api/overlay/raidthreat.
// Returns the ESTIMATED raid-wide per-mob, per-player hate view. Empty while
// the feature is disabled. There is no reset endpoint — the view follows the
// combat tracker's fight lifecycle (kill / zone / staleness).
func (h *raidThreatHandler) state(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.assembler.GetState())
}
