package api

import (
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
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

// dismissMob handles DELETE /api/overlay/raidthreat/{name}.
// Suppresses a single mob from the raid threat view (the overlay's per-card
// "x" button) until its fight lifecycle resets.
func (h *raidThreatHandler) dismissMob(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if decoded, err := url.PathUnescape(name); err == nil {
		name = decoded
	}
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}
	if !h.assembler.DismissMob(name) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
