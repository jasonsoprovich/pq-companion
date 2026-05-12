package api

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

type configHandler struct {
	mgr        *config.Manager
	hub        *ws.Hub
	actualPort int // port the server actually bound to (may differ from cfg.ServerAddr after fallback)
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

// serverInfo reports the port the backend is actually listening on (which may
// differ from cfg.ServerAddr when the preferred port was busy and the server
// fell back to an OS-assigned port). The Settings UI uses this to show the
// current port alongside the user-configured preference.
func (h *configHandler) serverInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"actual_port":    h.actualPort,
		"preferred_addr": h.mgr.Get().ServerAddr,
	})
}

// testPort probes whether a given TCP port can be bound on localhost. Used by
// the Settings UI's "Test availability" affordance before the user commits a
// preferred-port change. Binds and immediately closes a listener — this is a
// local-only availability check, not a network scan.
func (h *configHandler) testPort(w http.ResponseWriter, r *http.Request) {
	portStr := r.URL.Query().Get("port")
	if portStr == "" {
		writeError(w, http.StatusBadRequest, "port is required")
		return
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port < 1 || port > 65535 {
		writeError(w, http.StatusBadRequest, "port must be an integer 1-65535")
		return
	}
	// A port the server itself is currently bound to is "in use by us" —
	// don't make the user think they need to change anything.
	if port == h.actualPort {
		writeJSON(w, http.StatusOK, map[string]any{
			"available": true,
			"in_use_by": "pq-companion",
		})
		return
	}
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", port))
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"available": false,
			"error":     err.Error(),
		})
		return
	}
	_ = ln.Close()
	writeJSON(w, http.StatusOK, map[string]any{"available": true})
}
