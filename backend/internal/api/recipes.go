package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
)

type recipesHandler struct{ db *db.DB }

func (h *recipesHandler) search(w http.ResponseWriter, r *http.Request) {
	limit := queryInt(r, "limit", 50)
	if limit > 100 {
		limit = 100
	}
	// Tradeskill defaults to -1 ("any") because 0 is a real discipline value
	// (Common Combine); queryInt clamps negatives, so read it explicitly.
	tradeskill := -1
	if s := r.URL.Query().Get("tradeskill"); s != "" {
		if v, err := strconv.Atoi(s); err == nil {
			tradeskill = v
		}
	}
	f := db.RecipeFilter{
		Query:      r.URL.Query().Get("q"),
		Tradeskill: tradeskill,
		TrivialMin: queryInt(r, "trivial_min", 0),
		TrivialMax: queryInt(r, "trivial_max", 0),
		Limit:      limit,
		Offset:     queryInt(r, "offset", 0),
	}
	result, err := h.db.SearchRecipes(f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *recipesHandler) get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	recipe, err := h.db.GetRecipe(id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "recipe not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, recipe)
}

func (h *recipesHandler) tradeskills(w http.ResponseWriter, r *http.Request) {
	skills, err := h.db.GetRecipeTradeskills()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, skills)
}
