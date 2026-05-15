package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jasonsoprovich/pq-companion/backend/internal/players"
)

type playersHandler struct {
	store *players.Store
}

// list handles GET /api/players?search=&class=&zone=&limit=&offset=
func (h *playersHandler) list(w http.ResponseWriter, r *http.Request) {
	limit := queryInt(r, "limit", 200)
	if limit > 1000 {
		limit = 1000
	}
	filters := players.SearchFilters{
		NameContains: r.URL.Query().Get("search"),
		Class:        r.URL.Query().Get("class"),
		Zone:         r.URL.Query().Get("zone"),
		Limit:        limit,
		Offset:       queryInt(r, "offset", 0),
	}
	out, err := h.store.Search(filters)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if out == nil {
		out = []players.Sighting{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"players": out})
}

// get handles GET /api/players/{name}
func (h *playersHandler) get(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	s, err := h.store.Get(name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if s == nil {
		writeError(w, http.StatusNotFound, "player not found")
		return
	}
	writeJSON(w, http.StatusOK, s)
}

// history handles GET /api/players/{name}/history
func (h *playersHandler) history(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	rows, err := h.store.LevelHistory(name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if rows == nil {
		rows = []players.LevelHistoryEntry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"history": rows})
}

// delete handles DELETE /api/players/{name}
func (h *playersHandler) delete(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	if err := h.store.Delete(name); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// clear handles POST /api/players/clear
func (h *playersHandler) clear(w http.ResponseWriter, r *http.Request) {
	n, err := h.store.Clear()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": n})
}
