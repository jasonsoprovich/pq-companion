package api

import (
	"bytes"
	"errors"
	"image/png"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/mapgen"
)

// Character models for the 3D views and the dressing room, read from the
// user's own client (<eq_path>/*_chr*.s3d) like the zones are. Nothing from
// the client is shipped; the only thing stored is the index of which
// archive holds which model (~/.pq-companion/cache/model-index.json), so it
// isn't rebuilt on every launch.
//
// A caller resolves a race and gender to an actor and archive, then fetches
// that actor's mesh and its textures from the archive.

var (
	// chrArchiveRe guards an archive name from the URL: a character
	// archive in the EQ folder, no paths.
	chrArchiveRe = regexp.MustCompile(`^[a-z0-9_]{1,40}_chr[0-9]?\.s3d$`)
	// modelCodeRe guards an actor code ("HUM", "DR2", "LAUNCH").
	modelCodeRe = regexp.MustCompile(`^[A-Z0-9]{2,8}$`)
)

// modelMeshCacheSize is how many encoded model meshes are kept. A raid or a
// zone of NPCs uses a few dozen distinct models at most.
const modelMeshCacheSize = 48

// modelTextureArchives is how many character archives' directories are
// kept open for texture reads.
const modelTextureArchives = 6

type modelsHandler struct {
	cfg *config.Manager
	// indexPath is where the model index is cached on disk.
	indexPath string

	indexMu  sync.Mutex
	indexFor string // EQ path the index was built for
	index    *mapgen.ModelIndex

	meshes meshCache

	texMu    sync.Mutex
	texFor   string // EQ path the open archives belong to
	archives map[string]*mapgen.LazyArchive
	archOrd  []string
	pngs     meshCache
}

func newModelsHandler(cfg *config.Manager) *modelsHandler {
	h := &modelsHandler{cfg: cfg}
	h.meshes.size = modelMeshCacheSize
	h.pngs.size = 512
	if home, err := os.UserHomeDir(); err == nil {
		h.indexPath = filepath.Join(home, ".pq-companion", "cache", "model-index.json")
	}
	return h
}

func (h *modelsHandler) eqPath(w http.ResponseWriter) (string, bool) {
	p := h.cfg.Get().EQPath
	if p == "" {
		writeError(w, http.StatusServiceUnavailable, "EverQuest folder not set")
		return "", false
	}
	return p, true
}

// modelIndex returns the index for eqPath, loading or building it on first
// use. Building reads every character archive, so concurrent first
// requests wait for one build.
func (h *modelsHandler) modelIndex(eqPath string) (*mapgen.ModelIndex, error) {
	h.indexMu.Lock()
	defer h.indexMu.Unlock()
	if h.index != nil && h.indexFor == eqPath {
		return h.index, nil
	}
	start := time.Now()
	cache := h.indexPath
	if cache == "" {
		cache = filepath.Join(os.TempDir(), "pq-companion-model-index.json")
	}
	ix, err := mapgen.LoadModelIndex(eqPath, cache)
	if err != nil {
		return nil, err
	}
	slog.Info("models: index ready", "actors", len(ix.Actors), "took", time.Since(start))
	h.index, h.indexFor = ix, eqPath
	return ix, nil
}

// modelRef is a resolved model: the actor and the archive it's read from.
type modelRef struct {
	Code    string `json:"code"`
	Archive string `json:"archive"`
}

// resolve handles GET /api/models/resolve?race=&gender=&zone=&luclin=1:
// which actor the client draws for the race and gender, and the archive to
// read it from. zone prefers that zone's version of an NPC model; luclin
// asks for the Luclin-era player model. 404 when the client has none.
func (h *modelsHandler) resolve(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	race, err := strconv.Atoi(q.Get("race"))
	if err != nil || race < 0 || race > 1000 {
		writeError(w, http.StatusBadRequest, "invalid race")
		return
	}
	gender, err := strconv.Atoi(q.Get("gender"))
	if err != nil || gender < 0 || gender > 2 {
		gender = 0
	}
	zone := strings.ToLower(q.Get("zone"))
	if zone != "" && !zoneShortRe.MatchString(zone) {
		writeError(w, http.StatusBadRequest, "invalid zone")
		return
	}
	eqPath, ok := h.eqPath(w)
	if !ok {
		return
	}
	ix, err := h.modelIndex(eqPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			writeError(w, http.StatusNotFound, "EverQuest folder not found")
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	code, archive, err := ix.Resolve(race, gender, zone, q.Get("luclin") == "1")
	if err != nil {
		writeError(w, http.StatusNotFound, "no model for race "+strconv.Itoa(race))
		return
	}
	writeJSON(w, http.StatusOK, modelRef{Code: code, Archive: archive})
}

// mesh handles GET /api/models/mesh/{archive}/{code}: the actor posed
// standing, packed like a zone mesh (mapgen.Mesh3D.Encode), in map space
// around the model's own origin.
func (h *modelsHandler) mesh(w http.ResponseWriter, r *http.Request) {
	archive := strings.ToLower(chi.URLParam(r, "archive"))
	code := strings.ToUpper(chi.URLParam(r, "code"))
	if !chrArchiveRe.MatchString(archive) || !modelCodeRe.MatchString(code) {
		writeError(w, http.StatusBadRequest, "invalid model")
		return
	}
	eqPath, ok := h.eqPath(w)
	if !ok {
		return
	}
	key := eqPath + "|" + archive + "|" + code
	buf, ok := h.meshes.get(key)
	if !ok {
		m, err := mapgen.LoadModel(eqPath, archive, code)
		switch {
		case errors.Is(err, fs.ErrNotExist), errors.Is(err, mapgen.ErrNoModel):
			writeError(w, http.StatusNotFound, "no "+code+" in "+archive)
			return
		case err != nil:
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		buf = m.Encode()
		h.meshes.put(key, buf)
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(buf) //nolint:errcheck
}

// archive returns the open archive, reading its directory on first use.
// Caller holds texMu.
func (h *modelsHandler) archive(eqPath, name string) (*mapgen.LazyArchive, error) {
	if h.texFor != eqPath {
		h.texFor, h.archives, h.archOrd = eqPath, map[string]*mapgen.LazyArchive{}, nil
	}
	if a, ok := h.archives[name]; ok {
		return a, nil
	}
	a, err := mapgen.OpenArchiveLazy(filepath.Join(eqPath, name))
	if err != nil {
		return nil, err
	}
	h.archives[name] = a
	h.archOrd = append(h.archOrd, name)
	for len(h.archOrd) > modelTextureArchives {
		delete(h.archives, h.archOrd[0])
		h.archOrd = h.archOrd[1:]
	}
	return a, nil
}

// texture handles GET /api/models/texture/{archive}/{name}[?mask=1]: one of
// a character archive's textures as a PNG, at the 3D view's texture size.
func (h *modelsHandler) texture(w http.ResponseWriter, r *http.Request) {
	archive := strings.ToLower(chi.URLParam(r, "archive"))
	name, err := url.PathUnescape(chi.URLParam(r, "name"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid texture")
		return
	}
	name = strings.ToLower(name)
	if !chrArchiveRe.MatchString(archive) || !textureNameRe.MatchString(name) {
		writeError(w, http.StatusBadRequest, "invalid texture")
		return
	}
	eqPath, ok := h.eqPath(w)
	if !ok {
		return
	}
	masked := r.URL.Query().Get("mask") == "1"
	key := eqPath + "|" + archive + "|" + name
	if masked {
		key += "|mask"
	}
	if buf, ok := h.pngs.get(key); ok {
		writePNG(w, buf)
		return
	}
	h.texMu.Lock()
	a, err := h.archive(eqPath, archive)
	var raw []byte
	found := false
	if err == nil {
		raw, found, err = a.Get(name)
	}
	h.texMu.Unlock()
	switch {
	case errors.Is(err, fs.ErrNotExist) || (err == nil && !found):
		writeError(w, http.StatusNotFound, "no texture "+name+" in "+archive)
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	img, err := mapgen.DecodeTexture(raw, textureSize, masked)
	if err != nil {
		writeError(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.pngs.put(key, out.Bytes())
	writePNG(w, out.Bytes())
}

func writePNG(w http.ResponseWriter, buf []byte) {
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(buf) //nolint:errcheck
}
