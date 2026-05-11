package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jasonsoprovich/pq-companion/backend/internal/rolltracker"
)

type rollsHandler struct {
	tracker *rolltracker.Tracker
}

func (h *rollsHandler) state(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, h.tracker.State())
}

func (h *rollsHandler) stop(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id == 0 {
		writeError(w, http.StatusBadRequest, "invalid session id")
		return
	}
	if !h.tracker.Stop(id) {
		writeError(w, http.StatusNotFound, "no active session")
		return
	}
	writeJSON(w, http.StatusOK, h.tracker.State())
}

func (h *rollsHandler) remove(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id == 0 {
		writeError(w, http.StatusBadRequest, "invalid session id")
		return
	}
	if !h.tracker.Remove(id) {
		writeError(w, http.StatusNotFound, "session not found")
		return
	}
	writeJSON(w, http.StatusOK, h.tracker.State())
}

func (h *rollsHandler) clear(w http.ResponseWriter, _ *http.Request) {
	h.tracker.Clear()
	writeJSON(w, http.StatusOK, h.tracker.State())
}

// rollsSettingsRequest carries any subset of the tracker's settings —
// callers may update one field without resending the others.
type rollsSettingsRequest struct {
	WinnerRule      *rolltracker.WinnerRule `json:"winner_rule,omitempty"`
	Mode            *rolltracker.Mode       `json:"mode,omitempty"`
	AutoStopSeconds *int                    `json:"auto_stop_seconds,omitempty"`
}

func (h *rollsHandler) updateSettings(w http.ResponseWriter, r *http.Request) {
	var req rollsSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.WinnerRule != nil {
		if *req.WinnerRule != rolltracker.WinnerHighest && *req.WinnerRule != rolltracker.WinnerLowest {
			writeError(w, http.StatusBadRequest, `winner_rule must be "highest" or "lowest"`)
			return
		}
		h.tracker.SetWinnerRule(*req.WinnerRule)
	}
	if req.Mode != nil || req.AutoStopSeconds != nil {
		mode := h.tracker.State().Mode
		if req.Mode != nil {
			if *req.Mode != rolltracker.ModeManual && *req.Mode != rolltracker.ModeTimer {
				writeError(w, http.StatusBadRequest, `mode must be "manual" or "timer"`)
				return
			}
			mode = *req.Mode
		}
		secs := 0
		if req.AutoStopSeconds != nil {
			if *req.AutoStopSeconds < 5 || *req.AutoStopSeconds > 600 {
				writeError(w, http.StatusBadRequest, "auto_stop_seconds must be between 5 and 600")
				return
			}
			secs = *req.AutoStopSeconds
		}
		h.tracker.SetMode(mode, secs)
	}
	writeJSON(w, http.StatusOK, h.tracker.State())
}
