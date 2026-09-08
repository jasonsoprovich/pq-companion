package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/jasonsoprovich/pq-companion/backend/internal/character"
	"github.com/jasonsoprovich/pq-companion/backend/internal/mystats"
)

// statSnapshotsHandler serves the per-character `/mystats` snapshot history.
// Snapshots are captured automatically from the log by mystats.Consumer; this
// handler only lists and deletes them.
type statSnapshotsHandler struct {
	charStore *character.Store
	store     *mystats.Store // nil when the snapshot store failed to open
}

type statSnapshotsResponse struct {
	Character string                   `json:"character"`
	Snapshots []mystats.StoredSnapshot `json:"snapshots"`
}

// GET /api/characters/{id}/stat-snapshots
func (h *statSnapshotsHandler) list(w http.ResponseWriter, r *http.Request) {
	name, ok := h.resolveCharacter(w, r)
	if !ok {
		return
	}
	if h.store == nil {
		writeJSON(w, http.StatusOK, statSnapshotsResponse{Character: name, Snapshots: []mystats.StoredSnapshot{}})
		return
	}
	snaps, err := h.store.List(name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, statSnapshotsResponse{Character: name, Snapshots: snaps})
}

// DELETE /api/characters/{id}/stat-snapshots/{snapID}
func (h *statSnapshotsHandler) del(w http.ResponseWriter, r *http.Request) {
	name, ok := h.resolveCharacter(w, r)
	if !ok {
		return
	}
	snapID, err := strconv.ParseInt(chi.URLParam(r, "snapID"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid snapshot id")
		return
	}
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "stat snapshots disabled")
		return
	}
	if err := h.store.Delete(name, snapID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// resolveCharacter maps the {id} path param to a stored character's name.
func (h *statSnapshotsHandler) resolveCharacter(w http.ResponseWriter, r *http.Request) (string, bool) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return "", false
	}
	char, found, err := h.charStore.Get(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return "", false
	}
	if !found {
		writeError(w, http.StatusNotFound, "character not found")
		return "", false
	}
	return char.Name, true
}
