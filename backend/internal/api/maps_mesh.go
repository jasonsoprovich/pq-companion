package api

import (
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jasonsoprovich/pq-companion/backend/internal/mapgen"
)

// The 3D view's zone geometry is read from the user's own installed client
// (<eq_path>/<zone>.s3d) at request time; nothing from the client is shipped
// or stored.

// zoneShortRe guards the zone name before it becomes a file path.
var zoneShortRe = regexp.MustCompile(`^[a-z0-9_]{1,32}$`)

// meshCacheSize is how many encoded zone meshes are kept. Loading a zone
// parses its whole s3d, so flipping between 2D and 3D shouldn't redo it.
const meshCacheSize = 3

type meshCache struct {
	mu    sync.Mutex
	order []string
	data  map[string][]byte
}

func (c *meshCache) get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	b, ok := c.data[key]
	return b, ok
}

func (c *meshCache) put(key string, b []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.data == nil {
		c.data = map[string][]byte{}
	}
	if _, ok := c.data[key]; !ok {
		c.order = append(c.order, key)
	}
	c.data[key] = b
	for len(c.order) > meshCacheSize {
		delete(c.data, c.order[0])
		c.order = c.order[1:]
	}
}

// mesh3D handles GET /api/maps/zone/{zone}/mesh: the zone's solid geometry
// from the user's client files, packed by mapgen.EncodeMesh3D. 404 when the
// client has no s3d for the zone.
func (h *mapsHandler) mesh3D(w http.ResponseWriter, r *http.Request) {
	short := chi.URLParam(r, "zone")
	if !zoneShortRe.MatchString(short) {
		writeError(w, http.StatusBadRequest, "invalid zone")
		return
	}
	eqPath := h.cfg.Get().EQPath
	if eqPath == "" {
		writeError(w, http.StatusServiceUnavailable, "EverQuest folder not set")
		return
	}
	key := eqPath + "|" + short
	buf, ok := h.meshes.get(key)
	if !ok {
		start := time.Now()
		z, err := mapgen.LoadZone(eqPath, short)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				writeError(w, http.StatusNotFound, "no "+short+".s3d in the EverQuest folder")
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		pos, idx := z.Mesh3D()
		buf = mapgen.EncodeMesh3D(pos, idx)
		h.meshes.put(key, buf)
		slog.Info("maps: 3D mesh loaded", "zone", short, "triangles", len(idx)/3,
			"bytes", len(buf), "took", time.Since(start))
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(buf) //nolint:errcheck
}
