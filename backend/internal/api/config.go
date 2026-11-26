package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/jasonsoprovich/pq-companion/backend/internal/applog"
	"github.com/jasonsoprovich/pq-companion/backend/internal/backup"
	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/eqconfig"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
	"github.com/jasonsoprovich/pq-companion/backend/internal/zeal"
)

type configHandler struct {
	mgr        *config.Manager
	hub        *ws.Hub
	backupMgr  *backup.Manager
	actualPort int // port the server actually bound to (may differ from cfg.ServerAddr after fallback)
}

// eqDiagnostics is the consolidated "why aren't logs working" picture, used by
// the onboarding wizard, the Settings toggles, and the missing-log notices.
type eqDiagnostics struct {
	EQPath            string `json:"eq_path"`
	HasLogs           bool   `json:"has_logs"`
	CharacterCount    int    `json:"character_count"`
	LogFound          bool   `json:"log_found"`   // eqclient.ini had a Log= line
	LogEnabled        bool   `json:"log_enabled"` // ...and it's TRUE
	ZealInstalled     bool   `json:"zeal_installed"`
	ZealVersion       string `json:"zeal_version,omitempty"`
	ZealVersionOK     bool   `json:"zeal_version_ok"`
	ExportOnCampFound bool   `json:"export_on_camp_found"`
	ExportOnCamp      bool   `json:"export_on_camp"`
}

// buildDiagnostics gathers the log/Zeal state for an EQ directory.
func buildDiagnostics(eqPath string) eqDiagnostics {
	d := eqDiagnostics{EQPath: eqPath}
	if eqPath == "" {
		return d
	}
	chars := logparser.DiscoverCharacters(eqPath)
	d.CharacterCount = len(chars)
	d.HasLogs = len(chars) > 0

	logStatus := eqconfig.ReadLog(eqPath)
	d.LogFound = logStatus.Found
	d.LogEnabled = logStatus.Enabled

	z := zeal.DetectInstall(context.Background(), eqPath, nil)
	d.ZealInstalled = z.Installed
	d.ZealVersion = z.Version
	d.ZealVersionOK = z.VersionOK
	d.ExportOnCampFound = z.ExportOnCampFound
	d.ExportOnCamp = z.ExportOnCamp
	return d
}

// diagnostics handles GET /api/config/eq-diagnostics for the configured EQ path.
func (h *configHandler) diagnostics(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, buildDiagnostics(h.mgr.Get().EQPath))
}

// setLogging handles POST /api/config/set-logging {enabled} — writes the
// eqclient.ini Log flag after snapshotting the .ini files. Returns refreshed
// diagnostics.
func (h *configHandler) setLogging(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	eqPath := h.mgr.Get().EQPath
	if eqPath == "" {
		writeError(w, http.StatusBadRequest, "eq_path not configured")
		return
	}
	if h.backupMgr != nil {
		if _, err := h.backupMgr.Create("Before changing EQ logging", "Auto-backup before writing eqclient.ini"); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Sprintf("backup before write failed: %s", err))
			return
		}
	}
	if err := eqconfig.SetLog(eqPath, req.Enabled); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, buildDiagnostics(eqPath))
}

// setExportOnCamp handles POST /api/config/set-export-on-camp {enabled} —
// writes the zeal.ini ExportOnCamp flag after snapshotting the .ini files.
func (h *configHandler) setExportOnCamp(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	eqPath := h.mgr.Get().EQPath
	if eqPath == "" {
		writeError(w, http.StatusBadRequest, "eq_path not configured")
		return
	}
	if h.backupMgr != nil {
		// Best-effort: zeal.ini may be the only ini and Create globs *.ini, so a
		// missing-file error here shouldn't block the write.
		if _, err := h.backupMgr.Create("Before changing Zeal output-on-camp", "Auto-backup before writing zeal.ini"); err != nil {
			// Non-fatal — log via the response is overkill; proceed with the write.
			_ = err
		}
	}
	if err := eqconfig.SetExportOnCamp(eqPath, req.Enabled); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, buildDiagnostics(eqPath))
}

// get returns the current configuration as JSON.
func (h *configHandler) get(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.mgr.Get())
}

// update replaces the configuration with the request body and persists it.
func (h *configHandler) update(w http.ResponseWriter, r *http.Request) {
	// Decode over the current config, not a zero Config: a field the frontend
	// omits (older/partial client) then keeps its existing value instead of
	// decoding to the zero value and silently wiping that setting.
	// Maps the body sends replace the current ones (see config.DecodeOver).
	c := h.mgr.Get()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "failed to read body")
		return
	}
	if err := config.DecodeOver(&c, body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := h.mgr.Update(c); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save config")
		return
	}
	// Apply the verbose-logging preference live so toggling it in Settings takes
	// effect without a backend restart.
	applog.SetDebug(c.Preferences.DebugLogging)
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
	Valid bool   `json:"valid"`
	Error string `json:"error,omitempty"`
	// Note explains a valid folder that has no log files yet (a new player,
	// or logging off). It doesn't block setup: the wizard's next step turns
	// logging on.
	Note       string                          `json:"note,omitempty"`
	HasLogs    bool                            `json:"has_logs"`
	Characters []logparser.DiscoveredCharacter `json:"characters"`
	// Diagnostics is populated for a real directory so the wizard can explain
	// *why* logs are missing (logging disabled, Zeal output-on-camp off, …)
	// instead of the bare "no log files found".
	Diagnostics *eqDiagnostics `json:"diagnostics,omitempty"`
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
	diag := buildDiagnostics(req.Path)
	resp.Diagnostics = &diag
	if !resp.HasLogs {
		// No logs yet is normal for a new player or with logging off, so it
		// only blocks when the folder doesn't look like an EverQuest install
		// either. Otherwise explain the most likely cause as a note.
		if !looksLikeEQInstall(req.Path) {
			resp.Error = "This doesn't look like an EverQuest folder (no eqgame.exe, eqclient.ini or log files). Choose the folder that contains eqgame.exe."
			writeJSON(w, http.StatusOK, resp)
			return
		}
		switch {
		case diag.LogFound && !diag.LogEnabled:
			resp.Note = "No log files yet, which is normal for a new install. Logging is off in eqclient.ini; the next step can turn it on."
		case !diag.LogFound:
			resp.Note = "No log files yet, which is normal for a new install. The next step can turn logging on."
		default:
			resp.Note = "No log files yet, which is normal for a new install. They appear the first time you play with logging on."
		}
	}
	resp.Valid = true
	writeJSON(w, http.StatusOK, resp)
}

// looksLikeEQInstall reports whether dir holds an EverQuest client: eqgame.exe,
// eqgame.dll or eqclient.ini, matched case-insensitively (Windows and macOS
// file names vary in case, Linux/Wine installs keep whatever was copied).
func looksLikeEQInstall(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		switch strings.ToLower(e.Name()) {
		case "eqgame.exe", "eqgame.dll", "eqclient.ini":
			return true
		}
	}
	return false
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
