package api

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jasonsoprovich/pq-companion/backend/internal/spelltimer"
)

type timerHandler struct {
	engine *spelltimer.Engine
}

// state handles GET /api/overlay/timers — returns all active spell timers.
func (h *timerHandler) state(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.engine.GetState())
}

// clear handles POST /api/overlay/timers/clear — removes active timers in the
// given category group. The ?category= query parameter accepts "buff",
// "detrimental", "custom", "ch_chain", "ch_chain_2", "all", or empty
// (treated as "all").
func (h *timerHandler) clear(w http.ResponseWriter, r *http.Request) {
	group := r.URL.Query().Get("category")
	switch group {
	case "", "all", "buff", "detrimental", "custom", "ch_chain", "ch_chain_2":
		// accepted
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "category must be one of: buff, detrimental, custom, ch_chain, ch_chain_2, all",
		})
		return
	}
	h.engine.ClearCategory(group)
	w.WriteHeader(http.StatusNoContent)
}

// startCustom handles POST /api/overlay/timers/custom — starts a manual
// countdown timer on the Custom Timers overlay without needing a trigger.
// Body: {"name": "Break over", "duration_secs": 300, "alerts": [...]}.
// The optional "alerts" array carries the same fading-soon notification shape
// the trigger engine emits (the frontend builds it from the user's global
// Custom-timer alert preference); it is stored opaquely on the timer and fired
// client-side by useTimerAlerts. Omit or pass null for a silent timer.
func (h *timerHandler) startCustom(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name         string          `json:"name"`
		DurationSecs float64         `json:"duration_secs"`
		Alerts       json.RawMessage `json:"alerts,omitempty"`
		Color        string          `json:"color,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return
	}
	if req.DurationSecs <= 0 {
		writeError(w, http.StatusBadRequest, "duration_secs must be > 0")
		return
	}
	// spellID 0 — manual timers have no spell, so no duration focuses apply.
	// targetName "" — a manually-added timer has no captured target.
	// Alerts pass straight through to the timer's TimerAlerts (nil = silent).
	// Color "" (default) keeps the overlay's automatic bar color.
	h.engine.StartExternal(req.Name, string(spelltimer.CategoryCustom),
		req.DurationSecs, 0, time.Now(), req.Alerts, 0, "", req.Color, false)
	w.WriteHeader(http.StatusNoContent)
}

// remove handles DELETE /api/overlay/timers/{id} — removes a single active
// timer by its composite ID. Returns 404 if the ID isn't currently active.
func (h *timerHandler) remove(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
		return
	}
	// chi prefers r.URL.RawPath when the URL is percent-encoded, so the id
	// arrives still encoded (timer ids contain '@' and spaces). Decode before
	// looking up against the engine's unescaped map keys.
	if decoded, err := url.PathUnescape(id); err == nil {
		id = decoded
	}
	if !h.engine.RemoveByID(id) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
