package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/tradeskill"
)

type tradeskillHandler struct{ db *db.DB }

// modifiers lists the catalog of items that boost a given tradeskill skill,
// best bonus first. Drives the recipe success calculator's modifier picker.
func (h *tradeskillHandler) modifiers(w http.ResponseWriter, r *http.Request) {
	skillID, err := strconv.Atoi(chi.URLParam(r, "skillId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid skill id")
		return
	}
	mods, err := h.db.TradeskillModifiers(skillID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"modifiers": mods})
}

// chance computes a recipe's combine success/failure odds for a raw skill,
// optional item skill-mod percentage, and optional AA fail-reduction. Stateless:
// the caller supplies the character's skill (from /characters/{id}/tradeskills)
// and the recipe's trivial/nofail so the picker can recompute live.
func (h *tradeskillHandler) chance(w http.ResponseWriter, r *http.Request) {
	trivial := queryInt(r, "trivial", 0)
	raw := queryInt(r, "skill", 0)
	mod := queryInt(r, "mod", 0)
	aa := queryInt(r, "aa", 0)
	// The character's class/level skill cap (max attainable skill), so the panel
	// can show the honest best-case failure "even at max skill" rather than the
	// current-skill failure. 0 = unknown.
	cap := queryInt(r, "cap", 0)
	q := r.URL.Query()
	nofail := q.Get("nofail") == "1" || q.Get("nofail") == "true"
	writeJSON(w, http.StatusOK, tradeskill.Chance(raw, trivial, mod, aa, cap, nofail))
}
