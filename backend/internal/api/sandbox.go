package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/sandbox"
)

// sandboxHandler exposes the developer-mode SQL sandbox. Both endpoints
// 403 unless Preferences.DeveloperMode is enabled, so the renderer can
// rely on the same gate the UI uses without leaking the surface.
type sandboxHandler struct {
	sb     *sandbox.Sandbox
	cfgMgr *config.Manager
}

type sandboxQueryRequest struct {
	SQL string `json:"sql"`
}

func (h *sandboxHandler) guarded(w http.ResponseWriter) bool {
	if h.sb == nil {
		writeError(w, http.StatusServiceUnavailable, "sandbox not initialized")
		return false
	}
	if !h.cfgMgr.Get().Preferences.DeveloperMode {
		writeError(w, http.StatusForbidden, "developer mode is disabled")
		return false
	}
	return true
}

func (h *sandboxHandler) query(w http.ResponseWriter, r *http.Request) {
	if !h.guarded(w) {
		return
	}
	var req sandboxQueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	res, err := h.sb.Query(r.Context(), req.SQL)
	if err != nil {
		switch {
		case errors.Is(err, sandbox.ErrStatementNotAllowed),
			errors.Is(err, sandbox.ErrEmpty):
			writeError(w, http.StatusBadRequest, err.Error())
		default:
			// SQL syntax / runtime errors are user-driven, not server bugs —
			// surface them with 400 so the UI can render the message verbatim
			// in the results pane instead of a generic "server error".
			writeError(w, http.StatusBadRequest, err.Error())
		}
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (h *sandboxHandler) schema(w http.ResponseWriter, r *http.Request) {
	if !h.guarded(w) {
		return
	}
	tables, err := h.sb.Schema(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tables": tables})
}
