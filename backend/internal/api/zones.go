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
	// The zone catalog is small and fixed (~190 entries), so the browser
	// shows the full result set rather than paginating like items/spells/NPCs.
	limit := queryInt(r, "limit", 1000)
	offset := queryInt(r, "offset", 0)
	if limit > 1000 {
		limit = 1000
	}

	var filters db.ZoneSearchFilters
	if raw := r.URL.Query().Get("expansion"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil {
			filters.Expansion = &v
		}
	}

	result, err := h.db.SearchZones(q, filters, limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *zonesHandler) expansions(w http.ResponseWriter, r *http.Request) {
	result, err := h.db.ZoneExpansions()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if result == nil {
		result = []int{}
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *zonesHandler) getConnections(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	result, err := h.db.GetZoneConnections(name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if result == nil {
		result = []db.ZoneConnection{}
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *zonesHandler) getGroundSpawns(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	result, err := h.db.GetZoneGroundSpawns(name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if result == nil {
		result = []db.ZoneGroundSpawn{}
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *zonesHandler) getForage(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	result, err := h.db.GetZoneForage(name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if result == nil {
		result = []db.ZoneForageItem{}
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *zonesHandler) getDrops(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	result, err := h.db.GetZoneDrops(name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if result == nil {
		result = []db.ZoneDropItem{}
	}
	writeJSON(w, http.StatusOK, result)
}
