package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/raidtrack"
)

type raidReplayHandler struct {
	store *raidtrack.Store
	db    *db.DB
}

func (h *raidReplayHandler) zoneName(short string, cache map[string]string) string {
	if n, ok := cache[short]; ok {
		return n
	}
	name := ""
	if h.db != nil {
		if z, err := h.db.GetZoneByShortName(short); err == nil && z != nil {
			name = z.LongName
		}
	}
	cache[short] = name
	return name
}

// sessions handles GET /api/raidreplay/sessions: every recording, newest
// first.
func (h *raidReplayHandler) sessions(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "raid replay unavailable")
		return
	}
	ss, err := h.store.ListSessions()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	names := map[string]string{}
	for i := range ss {
		ss[i].ZoneName = h.zoneName(ss[i].Zone, names)
	}
	writeJSON(w, http.StatusOK, ss)
}

// recording handles GET /api/raidreplay/sessions/{id}: the session's
// roster, frames and events for playback.
func (h *raidReplayHandler) recording(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "raid replay unavailable")
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	rec, ok, err := h.store.GetRecording(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "recording not found")
		return
	}
	rec.Session.ZoneName = h.zoneName(rec.Session.Zone, map[string]string{})
	writeJSON(w, http.StatusOK, rec)
}

// deleteSession handles DELETE /api/raidreplay/sessions/{id}.
func (h *raidReplayHandler) deleteSession(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "raid replay unavailable")
		return
	}
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.store.DeleteSession(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// deleteAll handles DELETE /api/raidreplay/sessions.
func (h *raidReplayHandler) deleteAll(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "raid replay unavailable")
		return
	}
	if err := h.store.DeleteAll(); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
