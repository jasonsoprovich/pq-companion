package api

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

type logProbeRequest struct {
	EQPath    string `json:"eq_path"`
	Character string `json:"character"`
}

type logProbeResponse struct {
	// Character is the character probed: the one given, or the most recently
	// logged character when the request leaves it blank.
	Character string `json:"character"`
	Path      string `json:"path"`
	Exists    bool   `json:"exists"`
	SizeBytes int64  `json:"size_bytes"`
	// LastWriteAt is the log file's mtime (RFC 3339), "" when absent.
	LastWriteAt string `json:"last_write_at"`
	// LogEnabled reports eqclient.ini's Log=TRUE, which turns logging on
	// at every login without typing /log on.
	LogEnabled bool `json:"log_enabled"`
}

// logProbe handles POST /api/setup/log-probe {eq_path, character}. It is
// stateless: the onboarding wizard calls it before anything is saved to
// config, to show whether the chosen character's log is being written.
func (h *configHandler) logProbe(w http.ResponseWriter, r *http.Request) {
	var req logProbeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	writeJSON(w, http.StatusOK, probeLog(strings.TrimSpace(req.EQPath), strings.TrimSpace(req.Character)))
}

func probeLog(eqPath, character string) logProbeResponse {
	var resp logProbeResponse
	if eqPath == "" {
		return resp
	}
	if fi, err := os.Stat(eqPath); err != nil || !fi.IsDir() {
		return resp
	}
	if character == "" {
		character = logparser.ResolveActiveCharacter(eqPath)
	}
	resp.Character = character
	resp.LogEnabled = buildDiagnostics(eqPath).LogEnabled
	resp.Path = logparser.LogFilePath(eqPath, character)
	if resp.Path == "" {
		return resp
	}
	if fi, err := os.Stat(resp.Path); err == nil && !fi.IsDir() {
		resp.Exists = true
		resp.SizeBytes = fi.Size()
		resp.LastWriteAt = fi.ModTime().UTC().Format(time.RFC3339)
	}
	return resp
}
