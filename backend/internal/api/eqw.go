package api

import (
	"net/http"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/eqw"
)

// eqwHandler exposes EQW-TAKP (eqw.dll) version status to the Settings UI.
// eqw.dll sits next to eqgame.dll but has no PE version resource and isn't in
// the Quarm manifest, so it gets its own string-scan + GitHub-tag check rather
// than riding along in quarmHandler's manifest comparison.
type eqwHandler struct {
	cfgMgr *config.Manager
	latest *eqw.LatestFetcher
}

// GET /api/eqw/status
// Pure read of disk (eqw.dll build stamp) plus a cached HEAD of the upstream
// GitHub releases redirect. No auth or write side effects.
func (h *eqwHandler) status(w http.ResponseWriter, r *http.Request) {
	eqPath := h.cfgMgr.Get().EQPath
	status := eqw.DetectStatus(r.Context(), eqPath, h.latest)
	writeJSON(w, http.StatusOK, status)
}
