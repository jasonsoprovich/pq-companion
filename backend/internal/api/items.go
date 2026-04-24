package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
)

type itemsHandler struct{ db *db.DB }

func (h *itemsHandler) get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	item, err := h.db.GetItem(id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "item not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (h *itemsHandler) sources(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	sources, err := h.db.GetItemSources(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, sources)
}

func (h *itemsHandler) search(w http.ResponseWriter, r *http.Request) {
	limit := queryInt(r, "limit", 20)
	if limit > 100 {
		limit = 100
	}
	f := db.ItemFilter{
		Query:    r.URL.Query().Get("q"),
		BaneBody: queryInt(r, "bane_body", 0),
		Race:     queryInt(r, "race", 0),
		Class:    queryInt(r, "class", 0),
		MinLevel: queryInt(r, "min_level", 0),
		MaxLevel: queryInt(r, "max_level", 0),
		Slot:     queryInt(r, "slot", 0),
		ItemType: queryInt(r, "item_type", -1),
		MinSTR:   queryInt(r, "min_str", 0),
		MinSTA:   queryInt(r, "min_sta", 0),
		MinAGI:   queryInt(r, "min_agi", 0),
		MinDEX:   queryInt(r, "min_dex", 0),
		MinWIS:   queryInt(r, "min_wis", 0),
		MinINT:   queryInt(r, "min_int", 0),
		MinCHA:   queryInt(r, "min_cha", 0),
		MinHP:    queryInt(r, "min_hp", 0),
		MinMana:  queryInt(r, "min_mana", 0),
		MinAC:    queryInt(r, "min_ac", 0),
		MinMR:    queryInt(r, "min_mr", 0),
		MinCR:    queryInt(r, "min_cr", 0),
		MinDR:    queryInt(r, "min_dr", 0),
		MinFR:    queryInt(r, "min_fr", 0),
		MinPR:    queryInt(r, "min_pr", 0),
		Limit:    limit,
		Offset:   queryInt(r, "offset", 0),
	}
	result, err := h.db.SearchItems(f)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}
