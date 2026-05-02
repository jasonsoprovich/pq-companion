package api

import (
	"encoding/json"
	"net/http"
	"os"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

type configHandler struct {
	mgr *config.Manager
	hub *ws.Hub
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
	// Notify connected overlay windows so they can re-read settings
	// (e.g. spell-timer display thresholds) without a page reload.
	if h.hub != nil {
		h.hub.Broadcast(ws.Event{Type: "config:updated", Data: h.mgr.Get()})
	}
	writeJSON(w, http.StatusOK, h.mgr.Get())
}

type validateEQPathRequest struct {
	Path string `json:"path"`
}

type validateEQPathResponse struct {
	Valid      bool                              `json:"valid"`
	Error      string                            `json:"error,omitempty"`
	HasLogs    bool                              `json:"has_logs"`
	Characters []logparser.DiscoveredCharacter   `json:"characters"`
}

// validateEQPath checks whether a candidate path looks like a valid EverQuest
// installation. Used by the onboarding wizard before saving the config.
func (h *configHandler) validateEQPath(w http.ResponseWriter, r *http.Request) {
	var req validateEQPathRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	resp := validateEQPathResponse{Characters: []logparser.DiscoveredCharacter{}}
	if req.Path == "" {
		resp.Error = "path is required"
		writeJSON(w, http.StatusOK, resp)
		return
	}
	info, err := os.Stat(req.Path)
	if err != nil {
		resp.Error = "folder does not exist"
		writeJSON(w, http.StatusOK, resp)
		return
	}
	if !info.IsDir() {
		resp.Error = "path is not a folder"
		writeJSON(w, http.StatusOK, resp)
		return
	}
	chars := logparser.DiscoverCharacters(req.Path)
	resp.HasLogs = len(chars) > 0
	resp.Characters = chars
	if !resp.HasLogs {
		resp.Error = "no EverQuest log files (eqlog_*_pq.proj.txt) found in this folder"
		writeJSON(w, http.StatusOK, resp)
		return
	}
	resp.Valid = true
	writeJSON(w, http.StatusOK, resp)
}
