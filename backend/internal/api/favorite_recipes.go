package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jasonsoprovich/pq-companion/backend/internal/character"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
)

// favoriteRecipesHandler serves the global (not per-character) starred-recipe
// list. It joins the user.db favorites against the game db to return enriched
// recipe summaries the UI can render directly.
type favoriteRecipesHandler struct {
	store *character.Store
	db    *db.DB
}

func (h *favoriteRecipesHandler) list(w http.ResponseWriter, r *http.Request) {
	ids, err := h.store.ListFavoriteRecipes()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	summaries, err := h.db.GetRecipeSummariesByIDs(ids)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Preserve the favorites order; GetRecipeSummariesByIDs doesn't guarantee it.
	byID := make(map[int]db.RecipeSummary, len(summaries))
	for _, s := range summaries {
		byID[s.ID] = s
	}
	ordered := make([]db.RecipeSummary, 0, len(ids))
	for _, id := range ids {
		if s, ok := byID[id]; ok {
			ordered = append(ordered, s)
		}
	}
	writeJSON(w, http.StatusOK, ordered)
}

func (h *favoriteRecipesHandler) add(w http.ResponseWriter, r *http.Request) {
	var body struct {
		RecipeID int `json:"recipe_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.RecipeID <= 0 {
		writeError(w, http.StatusBadRequest, "invalid recipe_id")
		return
	}
	if err := h.store.AddFavoriteRecipe(body.RecipeID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *favoriteRecipesHandler) del(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if err := h.store.DeleteFavoriteRecipe(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
