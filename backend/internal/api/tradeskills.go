package api

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jasonsoprovich/pq-companion/backend/internal/character"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/tradeskill"
)

type tradeskillHandler struct {
	db    *db.DB
	store *character.Store
}

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

// customRecipes lists the recipes in a tradeskill's build-your-own Custom
// leveling path (global — shared across all characters, see
// character.Store.ListCustomLevelingRecipes), enriched to full summaries (so
// the UI has a name to show) and sorted by trivial — the same order the
// leveling-plan handler builds a Custom-mode plan in.
func (h *tradeskillHandler) customRecipes(w http.ResponseWriter, r *http.Request) {
	ts, err := strconv.Atoi(chi.URLParam(r, "skillId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid tradeskill")
		return
	}
	ids, err := h.store.ListCustomLevelingRecipes(ts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	summaries, err := h.db.GetRecipeSummariesByIDs(ids)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	sort.Slice(summaries, func(i, j int) bool {
		if summaries[i].Trivial != summaries[j].Trivial {
			return summaries[i].Trivial < summaries[j].Trivial
		}
		return summaries[i].ID < summaries[j].ID
	})
	writeJSON(w, http.StatusOK, summaries)
}

// addCustomRecipe adds a recipe to a tradeskill's Custom path.
func (h *tradeskillHandler) addCustomRecipe(w http.ResponseWriter, r *http.Request) {
	ts, err := strconv.Atoi(chi.URLParam(r, "skillId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid tradeskill")
		return
	}
	var body struct {
		RecipeID int `json:"recipe_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.RecipeID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid recipe_id")
		return
	}
	if err := h.store.AddCustomLevelingRecipe(ts, body.RecipeID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// deleteCustomRecipe removes a recipe from a tradeskill's Custom path.
func (h *tradeskillHandler) deleteCustomRecipe(w http.ResponseWriter, r *http.Request) {
	ts, err := strconv.Atoi(chi.URLParam(r, "skillId"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid tradeskill")
		return
	}
	recipeID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.store.DeleteCustomLevelingRecipe(ts, recipeID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
