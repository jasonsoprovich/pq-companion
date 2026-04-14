package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
)

// GET /api/spells/class/{classIndex}
// Returns all spells castable by the given class (0=Warrior … 14=Beastlord),
// ordered by that class's required level. Supports ?limit= and ?offset= for
// pagination; limit defaults to 500 and is capped at 1000.
func (h *spellsHandler) byClass(w http.ResponseWriter, r *http.Request) {
	classIndex, err := strconv.Atoi(chi.URLParam(r, "classIndex"))
	if err != nil || classIndex < 0 || classIndex > 14 {
		writeError(w, http.StatusBadRequest, "invalid class index: must be 0–14")
		return
	}
	limit := queryInt(r, "limit", 500)
	if limit > 1000 {
		limit = 1000
	}
	offset := queryInt(r, "offset", 0)
	result, err := h.db.GetSpellsByClass(classIndex, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

type spellsHandler struct{ db *db.DB }

func (h *spellsHandler) get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	spell, err := h.db.GetSpell(id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "spell not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, spell)
}

func (h *spellsHandler) search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	limit := queryInt(r, "limit", 20)
	offset := queryInt(r, "offset", 0)
	if limit > 100 {
		limit = 100
	}
	result, err := h.db.SearchSpells(q, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}
