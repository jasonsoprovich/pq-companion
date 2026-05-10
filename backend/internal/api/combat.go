package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jasonsoprovich/pq-companion/backend/internal/combat"
)

type combatHandler struct {
	tracker      *combat.Tracker
	historyStore *combat.HistoryStore // optional; nil disables history endpoints
}

// state handles GET /api/overlay/combat.
// Returns the current combat state: active fight, recent fights, and session DPS.
func (h *combatHandler) state(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.tracker.GetState())
}

// reset handles POST /api/combat/reset.
// Clears all fight history, session totals, and deaths from the tracker.
func (h *combatHandler) reset(w http.ResponseWriter, r *http.Request) {
	h.tracker.Reset()
	w.WriteHeader(http.StatusNoContent)
}

// historyList handles GET /api/combat/history.
// Query params (all optional): start, end (RFC3339), npc (substring),
// character, zone, limit (default 100, max 1000), offset.
// Response includes the matched fights plus a total count for pagination.
func (h *combatHandler) historyList(w http.ResponseWriter, r *http.Request) {
	if h.historyStore == nil {
		writeError(w, http.StatusServiceUnavailable, "combat history disabled")
		return
	}
	filter, err := parseFightFilter(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	fights, err := h.historyStore.ListFights(filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	total, err := h.historyStore.Count(filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"fights": fights,
		"total":  total,
		"limit":  filter.Limit,
		"offset": filter.Offset,
	})
}

// historyGet handles GET /api/combat/history/{id}.
func (h *combatHandler) historyGet(w http.ResponseWriter, r *http.Request) {
	if h.historyStore == nil {
		writeError(w, http.StatusServiceUnavailable, "combat history disabled")
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	f, err := h.historyStore.GetFight(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if f == nil {
		writeError(w, http.StatusNotFound, "fight not found")
		return
	}
	writeJSON(w, http.StatusOK, f)
}

// historyDelete handles DELETE /api/combat/history/{id}.
func (h *combatHandler) historyDelete(w http.ResponseWriter, r *http.Request) {
	if h.historyStore == nil {
		writeError(w, http.StatusServiceUnavailable, "combat history disabled")
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if _, err := h.historyStore.DeleteFight(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// historyClear handles DELETE /api/combat/history.
// Wipes the entire saved fight history. Intended for the "Clear History"
// button on the history page; the tracker's in-memory state is unaffected.
func (h *combatHandler) historyClear(w http.ResponseWriter, r *http.Request) {
	if h.historyStore == nil {
		writeError(w, http.StatusServiceUnavailable, "combat history disabled")
		return
	}
	n, err := h.historyStore.DeleteAll()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"removed": n})
}

// parseFightFilter pulls a FightFilter out of query parameters. Returns an
// error only when a provided value is malformed; missing fields are zero
// values, which the store treats as "no filter".
func parseFightFilter(r *http.Request) (combat.FightFilter, error) {
	q := r.URL.Query()
	f := combat.FightFilter{
		NPCName:       q.Get("npc"),
		CharacterName: q.Get("character"),
		Zone:          q.Get("zone"),
	}
	if v := q.Get("start"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return f, errBadParam("start", err)
		}
		f.StartTime = t
	}
	if v := q.Get("end"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return f, errBadParam("end", err)
		}
		f.EndTime = t
	}
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return f, errBadParam("limit", err)
		}
		f.Limit = n
	}
	if v := q.Get("offset"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return f, errBadParam("offset", err)
		}
		f.Offset = n
	}
	return f, nil
}

type paramError struct{ field, detail string }

func (e *paramError) Error() string { return "invalid " + e.field + ": " + e.detail }

func errBadParam(field string, err error) *paramError {
	d := "must be valid"
	if err != nil {
		d = err.Error()
	}
	return &paramError{field: field, detail: d}
}
