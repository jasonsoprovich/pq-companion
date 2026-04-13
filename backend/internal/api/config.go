package api

import (
	"encoding/json"
	"net/http"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
)

type configHandler struct {
	mgr *config.Manager
}

// get returns the current configuration as JSON.
func (h *configHandler) get(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.mgr.Get())
}

// update replaces the configuration with the request body and persists it.
func (h *configHandler) update(w http.ResponseWriter, r *http.Request) {
	var c config.Config
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := h.mgr.Update(c); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save config")
		return
	}
	writeJSON(w, http.StatusOK, h.mgr.Get())
}
