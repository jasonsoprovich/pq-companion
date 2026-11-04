package api

import (
	"bytes"
	"errors"
	"image/png"
	"io/fs"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
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

// mesh3D handles GET /api/maps/zone/{zone}/mesh: the zone's visible geometry
// and materials from the user's client files, packed by mapgen.Mesh3D.Encode.
// 404 when the client has no s3d for the zone.
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
		m, err := mapgen.LoadMesh3D(eqPath, short)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				writeError(w, http.StatusNotFound, "no "+short+".s3d in the EverQuest folder")
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		buf = m.Encode()
		h.meshes.put(key, buf)
		slog.Info("maps: 3D mesh loaded", "zone", short, "triangles", len(m.Idx)/3,
			"materials", len(m.Materials), "bytes", len(buf), "took", time.Since(start))
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(buf) //nolint:errcheck
}

// textureSize is the largest side, in pixels, a 3D-view texture is served
// at. Client textures are 1024 px; a zone has dozens, and at full size they
// would take hundreds of megabytes of GPU memory for a map view.
const textureSize = 256

// textureNameRe guards a texture name taken from the URL. Client names
// include spaces, apostrophes and ampersands ("stained glass.bmp").
var textureNameRe = regexp.MustCompile(`^[a-z0-9_&' \-]{1,64}\.(bmp|dds)$`)

// textureCache holds one zone's raw client textures and the PNGs made from
// them so far. The 3D view asks for a zone's textures together, so only the
// latest zone is kept; the mutex also makes those parallel requests share
// one read of the archive.
type textureCache struct {
	mu  sync.Mutex
	key string
	raw map[string][]byte
	png map[string][]byte
}

// get returns the PNG for one texture, reading the zone archive if it isn't
// the cached zone. ok is false when the archive has no such texture.
func (c *textureCache) get(eqPath, zone, name string, masked bool) (buf []byte, ok bool, err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	key := eqPath + "|" + zone
	if c.key != key {
		raw, err := mapgen.LoadTextures(eqPath, zone)
		if err != nil {
			return nil, false, err
		}
		c.key, c.raw, c.png = key, raw, map[string][]byte{}
	}
	pk := name
	if masked {
		pk += "|mask"
	}
	if b, ok := c.png[pk]; ok {
		return b, true, nil
	}
	raw, ok := c.raw[name]
	if !ok {
		return nil, false, nil
	}
	img, err := mapgen.DecodeTexture(raw, textureSize, masked)
	if err != nil {
		return nil, true, err
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil, true, err
	}
	c.png[pk] = out.Bytes()
	return out.Bytes(), true, nil
}

// texture3D handles GET /api/maps/zone/{zone}/texture/{name}: one of the
// zone's client textures as a PNG, for the 3D view. ?mask=1 keeps the colour
// key as transparency, for cutout materials like foliage.
func (h *mapsHandler) texture3D(w http.ResponseWriter, r *http.Request) {
	short := chi.URLParam(r, "zone")
	name := strings.ToLower(chi.URLParam(r, "name"))
	if !zoneShortRe.MatchString(short) || !textureNameRe.MatchString(name) {
		writeError(w, http.StatusBadRequest, "invalid texture")
		return
	}
	eqPath := h.cfg.Get().EQPath
	if eqPath == "" {
		writeError(w, http.StatusServiceUnavailable, "EverQuest folder not set")
		return
	}
	buf, ok, err := h.textures.get(eqPath, short, name, r.URL.Query().Get("mask") == "1")
	switch {
	case errors.Is(err, fs.ErrNotExist) || (err == nil && !ok):
		writeError(w, http.StatusNotFound, "no texture "+name+" in "+short+".s3d")
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(buf) //nolint:errcheck
}
