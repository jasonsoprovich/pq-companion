package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jasonsoprovich/pq-companion/backend/internal/keyring"
)

// keyringHandler serves the per-character keyring tracker — the new feature
// driven by parsing /keys log output. Distinct from keysHandler, which
// handles the older multi-component zone-key progression tracker.
type keyringHandler struct {
	store  *keyring.Store
	master []keyring.MasterEntry
}

// listMaster handles GET /api/keyring/master and returns the deduplicated
// master list from quarm.db keyring_data. The list is loaded once at
// startup since the underlying table is read-only.
func (h *keyringHandler) listMaster(w http.ResponseWriter, _ *http.Request) {
	if h.master == nil {
		writeJSON(w, http.StatusOK, map[string]any{"keys": []keyring.MasterEntry{}})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"keys": h.master})
}

// listCharacters handles GET /api/keyring/characters and returns the names
// of every character that has at least one keyring entry.
func (h *keyringHandler) listCharacters(w http.ResponseWriter, _ *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "keyring store unavailable")
		return
	}
	names, err := h.store.Characters()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"characters": names})
}

// getCharacter handles GET /api/keyring/characters/{name} and returns the
// character's owned keys (key_item IDs plus first/last_seen timestamps).
func (h *keyringHandler) getCharacter(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "keyring store unavailable")
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
