package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
)

type zonesHandler struct{ db *db.DB }

func (h *zonesHandler) get(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	zone, err := h.db.GetZone(id)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "zone not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, zone)
}

func (h *zonesHandler) getByShortName(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	zone, err := h.db.GetZoneByShortName(name)
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "zone not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, zone)
}

func (h *zonesHandler) getNPCsByShortName(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	limit := queryInt(r, "limit", 100)
	offset := queryInt(r, "offset", 0)
	if limit > 200 {
		limit = 200
	}
	result, err := h.db.GetNPCsByZone(name, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *zonesHandler) search(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")
	limit := queryInt(r, "limit", 20)
	offset := queryInt(r, "offset", 0)
	if limit > 100 {
		limit = 100
	}
	result, err := h.db.SearchZones(q, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}
