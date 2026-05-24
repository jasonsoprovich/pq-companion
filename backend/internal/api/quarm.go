package api

import (
	"net/http"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/quarm"
)

// quarmHandler exposes Project Quarm client-file status to the Settings UI.
// It reads the configured EQ directory, inspects eqgame.dll / eqw.dll, and
// compares the former against the public Quarm patch manifest.
type quarmHandler struct {
	cfgMgr  *config.Manager
	fetcher *quarm.ManifestFetcher
}

// GET /api/quarm/client-status
// Returns per-file status for the EQ client DLLs we care about. No
// authentication or write side effects — pure read of disk + cached HTTP
// fetch of the upstream YAML manifest.
func (h *quarmHandler) clientStatus(w http.ResponseWriter, r *http.Request) {
	eqPath := h.cfgMgr.Get().EQPath
	status := quarm.Status(r.Context(), eqPath, h.fetcher)
	writeJSON(w, http.StatusOK, status)
}
