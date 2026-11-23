package api

import (
	"bytes"
	"errors"
	"image/png"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"runtime/debug"
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

// meshCacheBytes bounds a zero-value meshCache's total size: a big zone's
// mesh is 40-50 MB, so this keeps it plus a small one, not three big ones.
const meshCacheBytes = 64 << 20

type meshCache struct {
	mu    sync.Mutex
	order []string // least recently used first
	data  map[string][]byte
	bytes int
	// size is how many entries are kept; 0 means meshCacheSize. maxBytes,
	// when set, also bounds their total size (the newest entry is always
	// kept); with both 0 it's meshCacheBytes.
	size     int
	maxBytes int
}

func (c *meshCache) get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	b, ok := c.data[key]
	if ok {
		c.touch(key)
	}
	return b, ok
}

// touch moves key to the most recently used end.
func (c *meshCache) touch(key string) {
	for i, k := range c.order {
		if k == key {
			c.order = append(append(c.order[:i:i], c.order[i+1:]...), key)
			return
		}
	}
}

func (c *meshCache) put(key string, b []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.data == nil {
		c.data = map[string][]byte{}
	}
	if old, ok := c.data[key]; ok {
		c.bytes -= len(old)
		c.touch(key)
	} else {
		c.order = append(c.order, key)
	}
	c.data[key] = b
	c.bytes += len(b)
	size, maxBytes := c.size, c.maxBytes
	if size <= 0 {
		size = meshCacheSize
		if maxBytes <= 0 {
			maxBytes = meshCacheBytes
		}
	}
	for len(c.order) > 1 && (len(c.order) > size || (maxBytes > 0 && c.bytes > maxBytes)) {
		c.bytes -= len(c.data[c.order[0]])
		delete(c.data, c.order[0])
		c.order = c.order[1:]
	}
}

// flight runs one call per key at a time: callers asking for a key that's
// already being built wait for that build and share its result. The main
// map and its popout open the same zone together, and two builds of a big
// zone would each allocate hundreds of megabytes.
type flight struct {
	mu    sync.Mutex
	calls map[string]*flightCall
}

type flightCall struct {
	done chan struct{}
	buf  []byte
	err  error
}

func (f *flight) do(key string, fn func() ([]byte, error)) ([]byte, error) {
	f.mu.Lock()
	if c, ok := f.calls[key]; ok {
		f.mu.Unlock()
		<-c.done
		return c.buf, c.err
	}
	if f.calls == nil {
		f.calls = map[string]*flightCall{}
	}
	c := &flightCall{done: make(chan struct{})}
	f.calls[key] = c
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		delete(f.calls, key)
		f.mu.Unlock()
		close(c.done)
	}()
	c.buf, c.err = fn()
	return c.buf, c.err
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
		var err error
		buf, err = h.meshBuilds.do(key, func() ([]byte, error) {
			// A build that finished while this one waited for the lock.
			if b, ok := h.meshes.get(key); ok {
				return b, nil
			}
			return h.buildMesh(eqPath, short, key)
		})
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				writeError(w, http.StatusNotFound, "no "+short+".s3d in the EverQuest folder")
				return
			}
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(buf) //nolint:errcheck
}

// buildMesh loads, encodes and caches one zone's mesh.
func (h *mapsHandler) buildMesh(eqPath, short, key string) ([]byte, error) {
	if h.loadMesh != nil {
		return h.loadMesh(eqPath, short)
	}
	start := time.Now()
	m, err := mapgen.LoadMesh3D(eqPath, short, h.zoneDoors(short))
	if err != nil {
		return nil, err
	}
	buf := m.Encode()
	h.meshes.put(key, buf)
	tris, mats, nModels := m.Triangles(), len(m.Materials), len(m.Models)
	m = nil
	took := time.Since(start)
	// The build's intermediates are now garbage. A big zone's build peaks
	// at a few hundred megabytes, which Go would otherwise keep from the
	// OS for minutes.
	freeOSMemory()
	slog.Info("maps: 3D mesh loaded", "zone", short, "triangles", tris, "models", nModels,
		"materials", mats, "bytes", len(buf), "took", took)
	return buf, nil
}

// zoneDoors returns the zone's doors for the 3D view. Doors are extra
// detail: without the game database, or on an error, the zone draws without
// them.
func (h *mapsHandler) zoneDoors(zone string) []mapgen.Door {
	if h.quarm == nil {
		return nil
	}
	rows, err := h.quarm.ZoneDoors(zone)
	if err != nil {
		slog.Warn("maps: 3D doors skipped", "zone", zone, "err", err)
		return nil
	}
	out := make([]mapgen.Door, 0, len(rows))
	for _, d := range rows {
		out = append(out, mapgen.Door{
			Model: d.Name, X: d.X, Y: d.Y, Z: d.Z,
			Heading: d.Heading, Incline: float64(d.Incline), Size: d.Size,
		})
	}
	return out
}

// textureSize is the largest side, in pixels, a 3D-view texture is served
// at. Client textures are 1024 px; a zone has dozens, and at full size they
// would take hundreds of megabytes of GPU memory for a map view.
const textureSize = 256

// textureNameRe guards a texture name taken from the URL. Client names
// include spaces, apostrophes and ampersands ("stained glass.bmp").
var textureNameRe = regexp.MustCompile(`^[a-z0-9_&' \-]{1,64}\.(bmp|dds)$`)

// textureCache serves one zone's client textures as PNGs. The 3D view asks
// for a zone's textures together, so only the latest zone's archives are
// kept open; textures are read from them one at a time, not held in
// memory. shared holds the shared model archives, opened the first time a
// zone asks for a texture its own archives lack. Everything is closed and
// dropped after textureIdle with no request.
//
// The archives are only read while holding mu, which is what makes closing
// them from the idle timer safe.
type textureCache struct {
	mu        sync.Mutex
	key       string
	zone      []*mapgen.LazyArchive
	sharedKey string
	shared    []*mapgen.LazyArchive
	pngs      *meshCache
	last      time.Time
	timer     *time.Timer
	// idle overrides textureIdle (tests).
	idle time.Duration
}

// textureIdle is how long the texture cache waits after its last request
// before closing the archives and dropping the PNGs.
const textureIdle = 2 * time.Minute

// texturePNGBytes bounds the PNGs kept per zone. A big zone's textures at
// textureSize come to around 20-40 MB.
const texturePNGBytes = 48 << 20

// get returns the PNG for one texture, opening the zone's archives if it
// isn't the cached zone. ok is false when no archive has the texture.
func (c *textureCache) get(eqPath, zone, name string, masked bool) (buf []byte, ok bool, err error) {
	pk := name
	if masked {
		pk += "|mask"
	}
	c.mu.Lock()
	c.touchLocked()
	key := eqPath + "|" + zone
	if c.key != key {
		mapgen.CloseArchives(c.zone)
		c.key, c.zone = "", nil
		as, err := mapgen.OpenTextureArchives(eqPath, zone)
		if err != nil {
			c.mu.Unlock()
			return nil, false, err
		}
		c.key, c.zone = key, as
		c.pngs = &meshCache{size: 1 << 20, maxBytes: texturePNGBytes}
	}
	pngs := c.pngs
	if b, ok := pngs.get(pk); ok {
		c.mu.Unlock()
		return b, true, nil
	}
	raw, ok, err := mapgen.FindTexture(c.zone, name)
	if err == nil && !ok {
		if c.sharedKey != eqPath {
			mapgen.CloseArchives(c.shared)
			c.sharedKey, c.shared = "", nil
			as, serr := mapgen.OpenSharedTextureArchives(eqPath)
			if serr != nil {
				c.mu.Unlock()
				return nil, false, serr
			}
			c.sharedKey, c.shared = eqPath, as
		}
		raw, ok, err = mapgen.FindTexture(c.shared, name)
	}
	c.mu.Unlock()
	if err != nil || !ok {
		return nil, ok, err
	}
	// Decoding needs no archive, so parallel requests decode in parallel.
	img, err := mapgen.DecodeTexture(raw, textureSize, masked)
	if err != nil {
		return nil, true, err
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil, true, err
	}
	pngs.put(pk, out.Bytes())
	return out.Bytes(), true, nil
}

func (c *textureCache) idleFor() time.Duration {
	if c.idle > 0 {
		return c.idle
	}
	return textureIdle
}

// touchLocked records a request and (re)arms the idle timer. Caller holds
// mu.
func (c *textureCache) touchLocked() {
	c.last = time.Now()
	if c.timer == nil {
		c.timer = time.AfterFunc(c.idleFor(), c.expire)
	} else {
		c.timer.Reset(c.idleFor())
	}
}

// expire is the idle timer's callback. A callback that was already waiting
// on mu when a request re-armed the timer finds the request recent and
// does nothing; the re-armed timer fires again later.
func (c *textureCache) expire() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.timer == nil || time.Since(c.last) < c.idleFor() {
		return
	}
	c.releaseLocked()
}

// releaseLocked closes the archives and drops everything, so the next
// request reopens. Caller holds mu.
func (c *textureCache) releaseLocked() {
	mapgen.CloseArchives(c.zone)
	mapgen.CloseArchives(c.shared)
	c.key, c.zone, c.sharedKey, c.shared, c.pngs = "", nil, "", nil, nil
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
	slog.Debug("maps: 3D textures released")
	go freeOSMemory()
}

// freeOSMemory returns freed heap to the OS. It runs a full collection, so
// it's for once-per-event moments like a finished zone build, never per
// request. A var so tests can count calls.
var freeOSMemory = debug.FreeOSMemory

// texture3D handles GET /api/maps/zone/{zone}/texture/{name}: one of the
// zone's client textures as a PNG, for the 3D view. ?mask=1 keeps the colour
// key as transparency, for cutout materials like foliage.
func (h *mapsHandler) texture3D(w http.ResponseWriter, r *http.Request) {
	short := chi.URLParam(r, "zone")
	// chi routes on the escaped path when the request has one, as it does
	// for "&" (%26), so the name can arrive still escaped.
	name, err := url.PathUnescape(chi.URLParam(r, "name"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid texture")
		return
	}
	name = strings.ToLower(name)
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
