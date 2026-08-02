package api

import (
	"encoding/binary"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/mapexport"
	"github.com/jasonsoprovich/pq-companion/backend/internal/mapfiles"
	"github.com/jasonsoprovich/pq-companion/backend/internal/maps"
)

type mapsHandler struct {
	store *maps.Store
	// annotations is the user's own markers, in user.db. Separate store because
	// maps.db is a shipped read-only artifact replaced by every app update —
	// anything written there would be destroyed on the next release.
	annotations *maps.AnnotationStore
	// cfg supplies the EQ install path for the in-game map export.
	cfg *config.Manager
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

// listAnnotations returns the user's own markers for one zone.
func (h *mapsHandler) listAnnotations(w http.ResponseWriter, r *http.Request) {
	rows, err := h.annotations.List(chi.URLParam(r, "zone"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"annotations": rows})
}

func (h *mapsHandler) createAnnotation(w http.ResponseWriter, r *http.Request) {
	var body maps.UserAnnotation
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	body.Zone = chi.URLParam(r, "zone")
	created, err := h.annotations.Create(body)
	if err != nil {
		// Validation failures are the caller's fault, not the server's.
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (h *mapsHandler) updateAnnotation(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad annotation id")
		return
	}
	var body struct {
		Category string `json:"category"`
		Label    string `json:"label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	updated, err := h.annotations.Update(id, body.Category, body.Label)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (h *mapsHandler) deleteAnnotation(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "bad annotation id")
		return
	}
	if err := h.annotations.Delete(id); err != nil {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// exportAnnotations serves every user marker in the same shape
// internal/mapgen/annotations.json reads, so a submission needs no conversion —
// see maps.BuildExport for why that matters and why evidence comes out blank.
func (h *mapsHandler) exportAnnotations(w http.ResponseWriter, r *http.Request) {
	rows, err := h.annotations.All()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, maps.BuildExport(rows))
}

// ── In-game map export (phase 6) ──────────────────────────────────────────────

// mapExportManifest is where ownership of written files is tracked. Outside the
// EQ directory because the file format permits no comments, so the marker cannot
// live in-band; see internal/mapexport.
func (h *mapsHandler) mapExportManifest() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".pq-companion", "map_export.json")
}

// gameMapStatus reports whether an export is possible and what is already there.
// externalStatus reports whether a third-party .txt map pack is installed in
// the player's own EQ folder.
//
// Detected fresh each call rather than cached at startup: installing a map pack
// is a thing a user does while the app is open, and the render mode should
// appear when they come back to it rather than after a restart. The scan is a
// single directory listing.
func (h *mapsHandler) externalStatus(w http.ResponseWriter, r *http.Request) {
	pack := mapfiles.Detect(h.cfg.Get().EQPath)
	if pack == nil {
		writeJSON(w, http.StatusOK, map[string]any{"available": false})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"available": true,
		"name":      pack.Name,
		"dir":       pack.Dir,
		"zones":     len(pack.Zones),
	})
}

// externalGeometry serves one zone from the installed pack.
//
// Same packed-binary shape as the built-in geometry endpoint, plus the three
// colour bytes the format reserves — a pack's palette is the point of rendering
// it, since the colours are what distinguish water from wall in a drawing that
// has no other way to say so.
//
// Not cached: unlike maps.db, these files can be replaced under us at any time.
func (h *mapsHandler) externalGeometry(w http.ResponseWriter, r *http.Request) {
	pack := mapfiles.Detect(h.cfg.Get().EQPath)
	if pack == nil {
		writeError(w, http.StatusNotFound, "no map pack installed")
		return
	}
	z, err := pack.Load(chi.URLParam(r, "zone"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if z == nil {
		// The pack simply has no file for this zone, which is ordinary — no
		// pack covers every zone. Empty, not an error.
		z = &mapfiles.Zone{}
	}
	// Does this pack's map describe the same place the server does? A pack is
	// drawn for whichever EverQuest its author plays, and a zone revamped since
	// Luclin renders perfectly while showing a different building. Reported in
	// a header rather than the body, which is packed binary.
	if pois, err := h.store.POIs(chi.URLParam(r, "zone")); err == nil && len(pois) > 0 {
		known := make([]mapfiles.Named, 0, len(pois))
		for _, p := range pois {
			known = append(known, mapfiles.Named{
				Label: p.Label, X: float64(p.X), Y: float64(p.Y),
			})
		}
		a := mapfiles.CheckAlignment(z, known)
		if a.Landmarks > 0 {
			w.Header().Set("X-Map-Landmarks", strconv.Itoa(a.Landmarks))
			w.Header().Set("X-Map-Offset", strconv.Itoa(int(a.OffsetUnits)))
			if a.Mismatch {
				w.Header().Set("X-Map-Mismatch", "1")
			}
		}
	}

	buf := make([]byte, len(z.Segments)*externalSegmentBytes)
	for i, s := range z.Segments {
		o := i * externalSegmentBytes
		for j, v := range [6]int{s.X1, s.Y1, s.Z1, s.X2, s.Y2, s.Z2} {
			binary.LittleEndian.PutUint16(buf[o+j*2:], uint16(int16(v)))
		}
		buf[o+12], buf[o+13], buf[o+14] = s.R, s.G, s.B
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)
	w.Write(buf)
}

// externalSegmentBytes is 6 int16 coordinates plus one RGB triple.
const externalSegmentBytes = 15

func (h *mapsHandler) gameMapStatus(w http.ResponseWriter, r *http.Request) {
	eq := h.cfg.Get().EQPath
	out := map[string]any{
		"eq_path":            eq,
		"default_categories": mapexport.DefaultCategories,
	}
	if eq == "" {
		out["ready"] = false
		out["reason"] = "EQ folder is not set in Settings"
		writeJSON(w, http.StatusOK, out)
		return
	}
	if _, err := os.Stat(eq); err != nil {
		out["ready"] = false
		out["reason"] = "EQ folder does not exist: " + eq
		writeJSON(w, http.StatusOK, out)
		return
	}
	out["ready"] = true

	m, err := mapexport.LoadManifest(h.mapExportManifest())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out["exported_files"] = len(m.Files)

	// Whether a foreign map pack is present changes which slot we take, so say so.
	entries, _ := os.ReadDir(filepath.Join(eq, mapexport.MapFilesDir))
	foreign := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".txt") {
			continue
		}
		if _, ours := m.Files[e.Name()]; !ours {
			foreign++
		}
	}
	out["existing_files"] = len(entries)
	out["foreign_files"] = foreign
	writeJSON(w, http.StatusOK, out)
}

type gameMapExportRequest struct {
	Categories []string `json:"categories"`
}

func (h *mapsHandler) collectPoints(categories []string) (map[string][]mapexport.Point, error) {
	if len(categories) == 0 {
		categories = mapexport.DefaultCategories
	}
	byZone, err := h.store.POIsByCategory(categories)
	if err != nil {
		return nil, err
	}
	out := map[string][]mapexport.Point{}
	for zone, pois := range byZone {
		for _, p := range pois {
			out[zone] = append(out[zone], mapexport.Point{
				X: p.X, Y: p.Y, Z: p.Z, Category: p.Category, Label: p.Label,
			})
		}
	}
	// The user's own markers ride along, so a note placed in the app shows up in
	// game without a second action.
	own, err := h.annotations.All()
	if err != nil {
		return nil, err
	}
	want := map[string]bool{}
	for _, c := range categories {
		want[c] = true
	}
	for _, a := range own {
		if !want[a.Category] {
			continue
		}
		out[a.Zone] = append(out[a.Zone], mapexport.Point{
			X: a.X, Y: a.Y, Z: a.Z, Category: a.Category, Label: a.Label,
		})
	}
	return out, nil
}

// gameMapExport writes the marker files into the EQ directory.
func (h *mapsHandler) gameMapExport(w http.ResponseWriter, r *http.Request) {
	eq := h.cfg.Get().EQPath
	if eq == "" {
		writeError(w, http.StatusBadRequest, "EQ folder is not set in Settings")
		return
	}
	var body gameMapExportRequest
	// An empty body is a valid request meaning "use the defaults".
	_ = json.NewDecoder(r.Body).Decode(&body)

	points, err := h.collectPoints(body.Categories)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	res, err := mapexport.Write(eq, h.mapExportManifest(), points)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// gameMapRemove deletes the files we wrote, and only those.
func (h *mapsHandler) gameMapRemove(w http.ResponseWriter, r *http.Request) {
	eq := h.cfg.Get().EQPath
	if eq == "" {
		writeError(w, http.StatusBadRequest, "EQ folder is not set in Settings")
		return
	}
	removed, kept, err := mapexport.Remove(eq, h.mapExportManifest())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"removed": removed, "kept": kept})
}
