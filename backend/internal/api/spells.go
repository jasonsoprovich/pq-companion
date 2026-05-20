package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
)

// maxStatDeltaIDs caps a single batch request — covers a full active-buff
// list (13 slots) plus headroom for raid-buff preset queries.
const maxStatDeltaIDs = 200

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

func (h *spellsHandler) crossRefs(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	refs, err := h.db.GetSpellCrossRefs(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, refs)
}

// POST /api/spells/stat-deltas
// Body: { "ids": [123, 456, ...] }
// Returns: { "123": BuffStatDelta, "456": BuffStatDelta, ... }
//
// IDs that don't resolve to a spell are silently omitted from the response.
// Used by the character stats page to compute aggregate buff contributions
// from active or preset buff lists.
func (h *spellsHandler) statDeltas(w http.ResponseWriter, r *http.Request) {
	var body struct {
		IDs []int `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if len(body.IDs) == 0 {
		writeJSON(w, http.StatusOK, map[string]db.BuffStatDelta{})
		return
	}
	if len(body.IDs) > maxStatDeltaIDs {
		writeError(w, http.StatusBadRequest, "too many ids")
		return
	}
	out := make(map[string]db.BuffStatDelta, len(body.IDs))
	for _, id := range body.IDs {
		sp, err := h.db.GetSpell(id)
		if err != nil || sp == nil {
			continue
		}
		out[strconv.Itoa(id)] = db.ComputeBuffStatDelta(sp)
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *spellsHandler) search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	limit := queryInt(r, "limit", 20)
	offset := queryInt(r, "offset", 0)
	if limit > 1000 {
		limit = 1000
	}
	classIndex := queryInt(r, "class", -1)
	minLevel := queryInt(r, "minLevel", 0)
	maxLevel := queryInt(r, "maxLevel", 0)
	result, err := h.db.SearchSpells(q, classIndex, minLevel, maxLevel, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}
