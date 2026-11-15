package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
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

	"github.com/jasonsoprovich/pq-companion/backend/internal/character"
	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
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
	// modelArchiveRe guards an archive name from the URL: a character
	// archive in the EQ folder, an item model archive (gequip) or a
	// Velious armor texture archive (global17_amr), no paths.
	modelArchiveRe = regexp.MustCompile(`^([a-z0-9_]{1,40}_chr[0-9]?|gequip[0-9]?|global[0-9]{2}_amr)\.s3d$`)
	// attachPoints are the points items can be attached at.
	attachPoints = map[string]bool{"R_POINT": true, "L_POINT": true, "SHIELD_POINT": true}
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
	cfg   *config.Manager
	db    *db.DB
	chars *character.Store
	// indexPath is where the model index is cached on disk.
	indexPath string

	indexMu  sync.Mutex
	indexFor string // EQ path the index was built for
	index    *mapgen.ModelIndex

	meshes meshCache

	posedMu  sync.Mutex
	posed    map[string]*mapgen.PosedModel
	posedOrd []string

	texMu    sync.Mutex
	texFor   string // EQ path the open archives belong to
	archives map[string]*mapgen.LazyArchive
	archOrd  []string
	pngs     meshCache
}

func newModelsHandler(cfg *config.Manager, database *db.DB, chars *character.Store) *modelsHandler {
	h := &modelsHandler{cfg: cfg, db: database, chars: chars}
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

// modelOptions reads the helm and robe swaps from a query.
func modelOptions(q url.Values) mapgen.ModelOptions {
	helm, _ := strconv.Atoi(q.Get("helm"))
	if helm < 0 || helm > 3 {
		helm = 0
	}
	return mapgen.ModelOptions{Helm: helm, Robe: q.Get("robe") == "1"}
}

// modelPosedCacheSize is how many loaded models (with their bone poses)
// are kept for outfits and attachments.
const modelPosedCacheSize = 8

// posedModel loads a model, or returns it from the cache.
func (h *modelsHandler) posedModel(eqPath, archive, code string, opts mapgen.ModelOptions) (*mapgen.PosedModel, error) {
	key := fmt.Sprintf("%s|%s|%s|%d|%v", eqPath, archive, code, opts.Helm, opts.Robe)
	h.posedMu.Lock()
	if p, ok := h.posed[key]; ok {
		h.posedMu.Unlock()
		return p, nil
	}
	h.posedMu.Unlock()
	p, err := mapgen.LoadModelWith(eqPath, archive, code, opts)
	if err != nil {
		return nil, err
	}
	h.posedMu.Lock()
	defer h.posedMu.Unlock()
	if h.posed == nil {
		h.posed = map[string]*mapgen.PosedModel{}
	}
	if _, ok := h.posed[key]; !ok {
		h.posedOrd = append(h.posedOrd, key)
	}
	h.posed[key] = p
	for len(h.posedOrd) > modelPosedCacheSize {
		delete(h.posed, h.posedOrd[0])
		h.posedOrd = h.posedOrd[1:]
	}
	return p, nil
}

// writeModelError answers a failed model load.
func writeModelError(w http.ResponseWriter, err error, what string) {
	switch {
	case errors.Is(err, fs.ErrNotExist), errors.Is(err, mapgen.ErrNoModel):
		writeError(w, http.StatusNotFound, "no "+what)
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

func writeMesh(w http.ResponseWriter, buf []byte) {
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(buf) //nolint:errcheck
}

// mesh handles GET /api/models/mesh/{archive}/{code}[?helm=&robe=1]: the
// actor posed standing, packed like a zone mesh (mapgen.Mesh3D.Encode), in
// map space around the model's own origin. helm and robe are the original
// player models' mesh swaps (see mapgen.ModelOptions).
func (h *modelsHandler) mesh(w http.ResponseWriter, r *http.Request) {
	archive := strings.ToLower(chi.URLParam(r, "archive"))
	code := strings.ToUpper(chi.URLParam(r, "code"))
	if !modelArchiveRe.MatchString(archive) || !modelCodeRe.MatchString(code) {
		writeError(w, http.StatusBadRequest, "invalid model")
		return
	}
	eqPath, ok := h.eqPath(w)
	if !ok {
		return
	}
	opts := modelOptions(r.URL.Query())
	key := fmt.Sprintf("%s|%s|%s|%d|%v", eqPath, archive, code, opts.Helm, opts.Robe)
	buf, ok := h.meshes.get(key)
	if !ok {
		p, err := h.posedModel(eqPath, archive, code, opts)
		if err != nil {
			writeModelError(w, err, code+" in "+archive)
			return
		}
		buf = p.Mesh.Encode()
		h.meshes.put(key, buf)
	}
	writeMesh(w, buf)
}

// outfitResponse is how to dress a model: the mesh swaps to load it with,
// then the texture swaps to make on it.
type outfitResponse struct {
	Helm int  `json:"helm"`
	Robe bool `json:"robe"`
	mapgen.OutfitResult
}

// outfit handles GET /api/models/outfit/{archive}/{code}?mats=h,c,a,w,g,l,f:
// how a player model looks wearing those materials (head, chest, arms,
// wrist, hands, legs, feet; 0 for nothing).
func (h *modelsHandler) outfit(w http.ResponseWriter, r *http.Request) {
	archive := strings.ToLower(chi.URLParam(r, "archive"))
	code := strings.ToUpper(chi.URLParam(r, "code"))
	if !modelArchiveRe.MatchString(archive) || !modelCodeRe.MatchString(code) {
		writeError(w, http.StatusBadRequest, "invalid model")
		return
	}
	var mats [mapgen.MaterialSlots]int
	if raw := r.URL.Query().Get("mats"); raw != "" {
		parts := strings.Split(raw, ",")
		if len(parts) != mapgen.MaterialSlots {
			writeError(w, http.StatusBadRequest, "mats needs one material per slot")
			return
		}
		for i, p := range parts {
			n, err := strconv.Atoi(p)
			if err != nil || n < 0 || n > 255 {
				writeError(w, http.StatusBadRequest, "invalid material")
				return
			}
			mats[i] = n
		}
	}
	eqPath, ok := h.eqPath(w)
	if !ok {
		return
	}
	// Only the original models swap meshes; a Luclin model is its race's
	// own archive (globalhum_chr.s3d).
	luclin := archive == "global"+strings.ToLower(code)+"_chr.s3d"
	var opts mapgen.ModelOptions
	if !luclin {
		opts = mapgen.ModelOptions{Helm: mapgen.OriginalHelm(mats[mapgen.SlotHead]), Robe: mapgen.IsRobe(mats[mapgen.SlotChest])}
	}
	p, err := h.posedModel(eqPath, archive, code, opts)
	if err != nil {
		writeModelError(w, err, code+" in "+archive)
		return
	}
	textures := make([]string, 0, len(p.Mesh.Materials))
	for _, m := range p.Mesh.Materials {
		textures = append(textures, m.Texture)
	}
	h.texMu.Lock()
	has := func(a, name string) bool {
		arc, err := h.archive(eqPath, a)
		return err == nil && arc.Has(name)
	}
	res := mapgen.DressTextures(archive, luclin, textures, mats, has)
	h.texMu.Unlock()
	if res.Swaps == nil {
		res.Swaps = []mapgen.TextureSwap{}
	}
	if res.Missing == nil {
		res.Missing = []int{}
	}
	writeJSON(w, http.StatusOK, outfitResponse{Helm: opts.Helm, Robe: opts.Robe, OutfitResult: res})
}

// locate handles GET /api/models/locate/{code}: the archive holding an
// actor, for item models ("IT63"). 404 when the client has none.
func (h *modelsHandler) locate(w http.ResponseWriter, r *http.Request) {
	code := mapgen.ItemModelCode(chi.URLParam(r, "code"))
	if code == "" || !modelCodeRe.MatchString(code) {
		writeError(w, http.StatusBadRequest, "invalid item model")
		return
	}
	eqPath, ok := h.eqPath(w)
	if !ok {
		return
	}
	ix, err := h.modelIndex(eqPath)
	if err != nil {
		writeModelError(w, err, "model index")
		return
	}
	archive, ok := ix.Locate(code, "", false)
	if !ok {
		writeError(w, http.StatusNotFound, "no model "+code)
		return
	}
	writeJSON(w, http.StatusOK, modelRef{Code: code, Archive: archive})
}

// attach handles GET /api/models/attach/{archive}/{code}/{point}/{itemArchive}/{item}:
// item model {item} placed at {point} of the standing model, in the
// model's space, packed like /mesh. Its textures are in {itemArchive}.
func (h *modelsHandler) attach(w http.ResponseWriter, r *http.Request) {
	archive := strings.ToLower(chi.URLParam(r, "archive"))
	code := strings.ToUpper(chi.URLParam(r, "code"))
	point := strings.ToUpper(chi.URLParam(r, "point"))
	itemArchive := strings.ToLower(chi.URLParam(r, "itemArchive"))
	item := strings.ToUpper(chi.URLParam(r, "item"))
	if !modelArchiveRe.MatchString(archive) || !modelCodeRe.MatchString(code) ||
		!modelArchiveRe.MatchString(itemArchive) || !modelCodeRe.MatchString(item) ||
		!attachPoints[point] {
		writeError(w, http.StatusBadRequest, "invalid attachment")
		return
	}
	eqPath, ok := h.eqPath(w)
	if !ok {
		return
	}
	key := strings.Join([]string{eqPath, "attach", archive, code, point, itemArchive, item}, "|")
	if buf, ok := h.meshes.get(key); ok {
		writeMesh(w, buf)
		return
	}
	// Bones are posed alike whatever the model wears.
	p, err := h.posedModel(eqPath, archive, code, mapgen.ModelOptions{})
	if err != nil {
		writeModelError(w, err, code+" in "+archive)
		return
	}
	m, err := mapgen.LoadModel(eqPath, itemArchive, item)
	if err != nil {
		writeModelError(w, err, item+" in "+itemArchive)
		return
	}
	placed, ok := p.Attach(m, point)
	if !ok {
		writeError(w, http.StatusNotFound, code+" has no "+point)
		return
	}
	buf := placed.Encode()
	h.meshes.put(key, buf)
	writeMesh(w, buf)
}

// items handles GET /api/models/items?ids=1,2,3: how those items look
// worn.
func (h *modelsHandler) items(w http.ResponseWriter, r *http.Request) {
	var ids []int
	for _, p := range strings.Split(r.URL.Query().Get("ids"), ",") {
		if p == "" {
			continue
		}
		id, err := strconv.Atoi(p)
		if err != nil || id <= 0 {
			writeError(w, http.StatusBadRequest, "invalid item id")
			return
		}
		ids = append(ids, id)
	}
	if len(ids) > 100 {
		writeError(w, http.StatusBadRequest, "too many items")
		return
	}
	out, err := h.db.ItemAppearances(ids)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if out == nil {
		out = []db.ItemAppearance{}
	}
	writeJSON(w, http.StatusOK, out)
}

// wearables handles GET /api/models/wearables?slots=&q=: items to try on
// in a slot (items.slots bits) whose name matches q.
func (h *modelsHandler) wearables(w http.ResponseWriter, r *http.Request) {
	slots, err := strconv.Atoi(r.URL.Query().Get("slots"))
	if err != nil || slots <= 0 {
		writeError(w, http.StatusBadRequest, "invalid slots")
		return
	}
	out, err := h.db.SearchWearables(slots, r.URL.Query().Get("q"), 60)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if out == nil {
		out = []db.ItemAppearance{}
	}
	writeJSON(w, http.StatusOK, out)
}

// appearance handles GET /api/models/appearance/{name}: the gender and
// model era saved for a character.
func (h *modelsHandler) appearance(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" || len(name) > 64 {
		writeError(w, http.StatusBadRequest, "invalid name")
		return
	}
	a, _, err := h.chars.GetAppearance(name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, a)
}

// setAppearance handles PUT /api/models/appearance/{name}.
func (h *modelsHandler) setAppearance(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" || len(name) > 64 {
		writeError(w, http.StatusBadRequest, "invalid name")
		return
	}
	var a character.Appearance
	if err := json.NewDecoder(r.Body).Decode(&a); err != nil {
		writeError(w, http.StatusBadRequest, "invalid body")
		return
	}
	a.Name = name
	if err := h.chars.SetAppearance(a); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, a)
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
	if !modelArchiveRe.MatchString(archive) || !textureNameRe.MatchString(name) {
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
