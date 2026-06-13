package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/jasonsoprovich/pq-companion/backend/internal/players"
)

type playersHandler struct {
	store *players.Store
}

// list handles GET /api/players?search=&class=&zone=&pvp=&limit=&offset=
// The response carries the filter-matching total alongside the page so the
// client can render an accurate count and a "show more" affordance.
func (h *playersHandler) list(w http.ResponseWriter, r *http.Request) {
	limit := queryInt(r, "limit", 200)
	if limit > 1000 {
		limit = 1000
	}
	filters := players.SearchFilters{
		NameContains: r.URL.Query().Get("search"),
		Class:        r.URL.Query().Get("class"),
		Zone:         r.URL.Query().Get("zone"),
		Guild:        r.URL.Query().Get("guild"),
		PVPOnly:      r.URL.Query().Get("pvp") == "1",
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
	total, err := h.store.Count(filters)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"players": out, "total": total})
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

// upsertNote handles PUT /api/players/{name}/note — saves the user's note
// text and PVP flag for a player.
func (h *playersHandler) upsertNote(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	var req struct {
		Note string `json:"note"`
		PVP  bool   `json:"pvp"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := h.store.UpsertNote(name, req.Note, req.PVP); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
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
