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

type rollsSettingsRequest struct {
	WinnerRule rolltracker.WinnerRule `json:"winner_rule"`
}

func (h *rollsHandler) updateSettings(w http.ResponseWriter, r *http.Request) {
	var req rollsSettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	if req.WinnerRule != rolltracker.WinnerHighest && req.WinnerRule != rolltracker.WinnerLowest {
		writeError(w, http.StatusBadRequest, `winner_rule must be "highest" or "lowest"`)
		return
	}
	h.tracker.SetWinnerRule(req.WinnerRule)
	writeJSON(w, http.StatusOK, h.tracker.State())
}
