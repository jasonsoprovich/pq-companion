package api

import (
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"
	"github.com/jasonsoprovich/pq-companion/backend/internal/respawn"
)

type respawnHandler struct {
	engine *respawn.Engine
}

// state handles GET /api/overlay/respawns — returns all active respawn timers.
func (h *respawnHandler) state(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.engine.GetState())
}

// clear handles DELETE /api/overlay/respawns — removes every active timer.
func (h *respawnHandler) clear(w http.ResponseWriter, r *http.Request) {
	h.engine.Clear()
	w.WriteHeader(http.StatusNoContent)
}

// remove handles DELETE /api/overlay/respawns/{id} — removes a single active
// timer by its ID. Returns 404 if the ID isn't currently active.
func (h *respawnHandler) remove(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id is required"})
		return
	}
	// IDs contain '|' and spaces, so they arrive percent-encoded. Decode
	// before looking up against the engine's unescaped map keys.
	if decoded, err := url.PathUnescape(id); err == nil {
		id = decoded
	}
	if !h.engine.RemoveByID(id) {
		w.WriteHeader(http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
