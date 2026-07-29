package api

import (
	"encoding/binary"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jasonsoprovich/pq-companion/backend/internal/maps"
)

type mapsHandler struct {
	store *maps.Store
}

// status reports whether map data is present, so the UI can hide map features
// rather than showing broken ones when a build ships without maps.db.
func (h *mapsHandler) status(w http.ResponseWriter, r *http.Request) {
	zones, err := h.store.Zones()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"available": h.store.Available(),
		"zones":     len(zones),
	})
}

// list returns metadata for every zone that has a map — enough to drive a zone
// picker without fetching any geometry.
func (h *mapsHandler) list(w http.ResponseWriter, r *http.Request) {
	zones, err := h.store.Zones()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if zones == nil {
		zones = []maps.Zone{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"zones": zones})
}

// zone returns one zone's metadata plus its POIs. Geometry is fetched
// separately via geometry(), so a caller that only wants pins (the NPC page)
// doesn't pull megabytes of line data it will never draw.
func (h *mapsHandler) zone(w http.ResponseWriter, r *http.Request) {
	short := chi.URLParam(r, "zone")
	z, ok, err := h.store.Zone(short)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "no map for zone "+short)
		return
	}
	pois, err := h.store.POIs(short)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"zone": z, "pois": pois})
}

// geometry streams a zone's line segments as packed binary.
//
// Deliberately not JSON: a large zone runs to tens of thousands of segments,
// and as JSON objects that is megabytes of braces and field names for what is
// really six small integers each. The renderer reads this straight into a typed
// array. Layout is little-endian int16 x1,y1,z1,x2,y2,z2 per segment.
func (h *mapsHandler) geometry(w http.ResponseWriter, r *http.Request) {
	short := chi.URLParam(r, "zone")
	// ?layer=1 fetches the optional boundary-detail layer. An absent layer is
	// not an error — most zones have only layer 0 — so it returns empty.
	layer := 0
	if v := r.URL.Query().Get("layer"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			layer = n
		}
	}
	segs, err := h.store.Segments(short, layer)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	buf := make([]byte, len(segs)*12)
	for i, s := range segs {
		o := i * 12
		for j, v := range [6]int{s.X1, s.Y1, s.Z1, s.X2, s.Y2, s.Z2} {
			binary.LittleEndian.PutUint16(buf[o+j*2:], uint16(int16(v)))
		}
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	// maps.db is immutable for the life of an install, so the renderer can
	// cache aggressively instead of refetching on every zone switch.
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	w.WriteHeader(http.StatusOK)
	w.Write(buf)
}
