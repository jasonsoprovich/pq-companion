package api

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jasonsoprovich/pq-companion/backend/internal/rolltracker"
)

// maxItemNameLen bounds the user-supplied loot-item label. EQ item names
// top out well under this; the cap just stops a pathological paste from
// bloating the broadcast payload.
const maxItemNameLen = 128

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

type rollsItemNameRequest struct {
	ItemName string `json:"item_name"`
}

// setItemName labels a session with the loot item it's rolling for. An
// empty/whitespace name clears the label.
func (h *rollsHandler) setItemName(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseUint(chi.URLParam(r, "id"), 10, 64)
	if err != nil || id == 0 {
		writeError(w, http.StatusBadRequest, "invalid session id")
		return
	}
	var req rollsItemNameRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	name := strings.TrimSpace(req.ItemName)
	if len(name) > maxItemNameLen {
		name = name[:maxItemNameLen]
	}
	if !h.tracker.SetItemName(id, name) {
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
	WinnerRule      *rolltracker.WinnerRule  `json:"winner_rule,omitempty"`
	Mode            *rolltracker.Mode        `json:"mode,omitempty"`
	AutoStopSeconds *int                     `json:"auto_stop_seconds,omitempty"`
	Profile         *rolltracker.RollProfile `json:"profile,omitempty"`
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
	if req.Profile != nil {
		profile, err := req.Profile.Validate()
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid profile: "+err.Error())
			return
		}
		h.tracker.SetProfile(profile)
	}
	writeJSON(w, http.StatusOK, h.tracker.State())
}
