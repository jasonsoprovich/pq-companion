package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jasonsoprovich/pq-companion/backend/internal/lockout"
)

// lockoutsHandler serves the per-character loot/legacy lockout tracker driven
// by parsing the in-game /sll command from the log file.
type lockoutsHandler struct {
	store *lockout.Store
}

// listCharacters handles GET /api/lockouts/characters and returns the names of
// every character that has at least one captured lockout snapshot.
func (h *lockoutsHandler) listCharacters(w http.ResponseWriter, _ *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "lockout store unavailable")
		return
	}
	names, err := h.store.Characters()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"characters": names})
}

// getCharacter handles GET /api/lockouts/characters/{name} and returns the
// character's lockout entries (both sections) in snapshot order.
func (h *lockoutsHandler) getCharacter(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "lockout store unavailable")
		return
	}
	name := chi.URLParam(r, "name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	entries, err := h.store.ListByCharacter(name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"character": name, "entries": entries})
}
