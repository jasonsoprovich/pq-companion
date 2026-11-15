package mapgen

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Character models (players and NPCs) for the 3D views, read from the
// user's own client like the zones are. A model is an actor in one of the
// client's character archives (*_chr*.s3d): the original player models in
// global_chr.s3d, the Luclin-era ones in global<code>_chr.s3d (globalhum_
// chr.s3d), and NPCs in their zones' archives and the numbered globals.
// Which actor a race draws is ModelCodes; which archive holds it is the
// ModelIndex.

// idleAnim is the animation models are posed in: "P01", standing idle.
// Luclin models name their tracks with a variant letter after it
// ("P01A...").
const idleAnim = "P01"

// ErrNoModel means no character archive holds a model for the request.
var ErrNoModel = errors.New("no model for this race")

// ModelIndex lists the character archives that hold each actor. Building
// it reads every character archive's .wld (seconds, on a slow drive), so
// it's cached on disk and rebuilt only when the archives change.
type ModelIndex struct {
	// Fingerprint identifies the set of archives indexed: names, sizes
	// and modification times.
	Fingerprint string `json:"fingerprint"`
	// Actors maps an actor code ("GNN") to the archive file names holding
	// it, lower-cased and sorted.
	Actors map[string][]string `json:"actors"`
}

// chrArchives lists the client's character archives with a fingerprint of
// the set.
func chrArchives(clientDir string) (names []string, fingerprint string, err error) {
	entries, err := os.ReadDir(clientDir)
	if err != nil {
		return nil, "", err
	}
	h := sha1.New()
	for _, e := range entries {
		n := strings.ToLower(e.Name())
		if e.IsDir() || !strings.HasSuffix(n, ".s3d") || !strings.Contains(n, "_chr") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		names = append(names, e.Name())
		fmt.Fprintf(h, "%s|%d|%d\n", n, info.Size(), info.ModTime().Unix())
	}
	return names, hex.EncodeToString(h.Sum(nil)), nil
}

// BuildModelIndex reads every character archive in clientDir.
func BuildModelIndex(clientDir string) (*ModelIndex, error) {
	names, fp, err := chrArchives(clientDir)
	if err != nil {
		return nil, err
	}
	ix := &ModelIndex{Fingerprint: fp, Actors: map[string][]string{}}
	for _, name := range names {
		w, err := loadArchiveWLD(filepath.Join(clientDir, name))
		if err != nil {
			continue // a damaged or geometry-free archive just isn't indexed
		}
		lower := strings.ToLower(name)
		for _, f := range w.Fragments {
			if f.kind != fragActor {
				continue
			}
			code := strings.TrimSuffix(strings.ToUpper(w.str(f.nameRef)), "_ACTORDEF")
			if code != "" {
				ix.Actors[code] = append(ix.Actors[code], lower)
			}
		}
	}
	for _, list := range ix.Actors {
		sort.Strings(list)
	}
	return ix, nil
}

// LoadModelIndex returns the index for clientDir from cacheFile when it
// still matches the client's archives, and otherwise builds it and saves
// it there. A cache that can't be written isn't an error.
func LoadModelIndex(clientDir, cacheFile string) (*ModelIndex, error) {
	_, fp, err := chrArchives(clientDir)
	if err != nil {
		return nil, err
	}
	if raw, err := os.ReadFile(cacheFile); err == nil {
		var ix ModelIndex
		if json.Unmarshal(raw, &ix) == nil && ix.Fingerprint == fp && len(ix.Actors) > 0 {
			return &ix, nil
		}
	}
	ix, err := BuildModelIndex(clientDir)
	if err != nil {
		return nil, err
	}
	if raw, err := json.Marshal(ix); err == nil {
		if os.MkdirAll(filepath.Dir(cacheFile), 0o755) == nil {
			_ = os.WriteFile(cacheFile, raw, 0o644)
		}
	}
	return ix, nil
}

// Locate picks the archive to read actor code from. The zone's own
// archives come first (an NPC looks as it does there), then the original
// player and shared globals, then any other archive that has it. With
// luclin, a Luclin-era model (global<code>_chr.s3d) wins over all of them.
func (ix *ModelIndex) Locate(code, zone string, luclin bool) (string, bool) {
	have := ix.Actors[strings.ToUpper(code)]
	if len(have) == 0 {
		return "", false
	}
	in := func(name string) bool {
		for _, h := range have {
			if h == name {
				return true
			}
		}
		return false
	}
	var order []string
	luclinFile := "global" + strings.ToLower(code) + "_chr.s3d"
	if luclin {
		order = append(order, luclinFile)
	}
	if zone != "" {
		order = append(order, zone+"_chr.s3d", zone+"_chr2.s3d")
	}
	order = append(order, "global_chr.s3d", "global_chr1.s3d")
	for i := 2; i <= 7; i++ {
		order = append(order, fmt.Sprintf("global%d_chr.s3d", i))
	}
	for _, name := range order {
		if in(name) {
			return name, true
		}
	}
	// Anything else, but not a Luclin model unless asked for one.
	for _, h := range have {
		if h != luclinFile {
			return h, true
		}
	}
	return have[0], true
}

// Resolve picks the actor and archive for a race and gender: the first of
// ModelCodes the client has. luclin asks for the Luclin-era model, which
// only the player races have.
func (ix *ModelIndex) Resolve(race, gender int, zone string, luclin bool) (code, archive string, err error) {
	luclin = luclin && IsPlayableModelRace(race)
	for _, c := range ModelCodes(race, gender) {
		if a, ok := ix.Locate(c, zone, luclin); ok {
			return c, a, nil
		}
	}
	return "", "", ErrNoModel
}

// LoadModel reads actor code from <clientDir>/<archive> as a 3D mesh,
// posed standing idle when the model has that animation (in its rest pose,
// arms out, otherwise). Positions are in map space with the model's own
// origin; its textures are in the same archive.
func LoadModel(clientDir, archive, code string) (*Mesh3D, error) {
	w, err := loadArchiveWLD(filepath.Join(clientDir, archive))
	if err != nil {
		return nil, err
	}
	want := strings.ToUpper(code) + "_ACTORDEF"
	for _, f := range w.Fragments {
		if f.kind != fragActor || strings.ToUpper(w.str(f.nameRef)) != want {
			continue
		}
		b := newMeshBuilder()
		for _, r := range actorComponents(f.payload) {
			if m, ok := w.meshRef(r); ok {
				b.add(w, m, nil, false)
				continue
			}
			for _, m := range w.posedSkeleton(r, idleAnim, 0) {
				b.add(w, m, nil, false)
			}
		}
		out := b.build()
		if len(out.Idx) == 0 {
			return nil, fmt.Errorf("%s in %s: %w", code, archive, ErrNoModel)
		}
		return out, nil
	}
	return nil, fmt.Errorf("%s in %s: %w", code, archive, fs.ErrNotExist)
}

// posedSkeleton poses a skeletal actor (0x11 reference r) at frame frame
// of animation anim. A bone's animated track is named for its rest track
// with the animation's prefix in front ("P01" + "HUMPE_TRACK"), or the
// prefix and a variant letter on Luclin models ("P01A...").
//
// Many original models have no animations of their own: the client lends
// them another model's. Bones are named alike across humanoids (the
// model code, then PE, CH, NE, BI_L...), so a model without the animation
// borrows it from the model in the same file whose tracks cover the most of
// its bones, preferring the same gender. A borrowed track gives the bone
// its rotation only; the bone keeps its own length, or a dark elf posed with
// a dwarf's walk would come out dwarf-shaped. With no good match, the model
// is left in its rest pose: half-animated looks worse than arms out.
func (w *WLD) posedSkeleton(r int32, anim string, frame int) []Mesh {
	tracks := map[string]int32{}
	for i, f := range w.Fragments {
		if f.kind == fragTrack {
			tracks[strings.ToUpper(w.str(f.nameRef))] = int32(i + 1)
		}
	}
	var restNames []string
	w.skeletonMeshesPosed(r, func(track int32) bonePose {
		if i, ok := w.ref(track, fragTrack); ok {
			restNames = append(restNames, strings.ToUpper(w.str(w.Fragments[i].nameRef)))
		}
		return bonePose{track: track}
	})
	if len(restNames) == 0 {
		return w.skeletonMeshes(r)
	}
	// The model's own code is its root bone's track name ("HUM_TRACK").
	own := strings.TrimSuffix(restNames[0], "_TRACK")
	find := func(code, rest string) (int32, bool) {
		name := code + strings.TrimPrefix(rest, own)
		for _, p := range []string{anim, anim + "A", anim + "B"} {
			if t, ok := tracks[p+name]; ok {
				return t, true
			}
		}
		return 0, false
	}
	// Attachment points (weapon, shield, guild and head points) and the
	// root never move, so only the real bones count toward coverage.
	moving := 0
	for _, rest := range restNames {
		if isMovingBone(own, rest) {
			moving++
		}
	}
	coverage := func(code string) int {
		n := 0
		for _, rest := range restNames {
			if !isMovingBone(own, rest) {
				continue
			}
			if _, ok := find(code, rest); ok {
				n++
			}
		}
		return n
	}

	source, keepRest := own, false
	if coverage(own)*2 < moving {
		// Best coverage, then the same gender, then the closest skeleton.
		best, bestN, bestShape := "", 0, 0.0
		for _, c := range animatedCodes(tracks, anim) {
			n := coverage(c)
			if n == 0 {
				continue
			}
			shape := w.shapeDistance(tracks, own, c, restNames)
			sameGender := len(c) == 3 && len(own) == 3 && c[2] == own[2]
			bestSame := len(best) == 3 && len(own) == 3 && best[2] == own[2]
			better := n > bestN ||
				(n == bestN && sameGender && !bestSame) ||
				(n == bestN && sameGender == bestSame && shape < bestShape)
			if better {
				best, bestN, bestShape = c, n, shape
			}
		}
		if moving == 0 || bestN*5 < moving*4 {
			return w.skeletonMeshes(r)
		}
		source, keepRest = best, true
	}
	return w.skeletonMeshesPosed(r, func(track int32) bonePose {
		i, ok := w.ref(track, fragTrack)
		if !ok {
			return bonePose{track: track}
		}
		if t, ok := find(source, strings.ToUpper(w.str(w.Fragments[i].nameRef))); ok {
			return bonePose{track: t, frame: frame, keepRest: keepRest}
		}
		return bonePose{track: track}
	})
}

// isMovingBone reports whether a bone's rest track (named for model own)
// is a real bone rather than the root or an attachment point.
func isMovingBone(own, rest string) bool {
	suffix := strings.TrimPrefix(rest, own)
	return suffix != "_TRACK" && !strings.Contains(suffix, "POINT")
}

// shapeDistance compares two models' skeletons: how far apart their bones'
// rest offsets are, each scaled by its model's overall size. Smaller is a
// closer build (a gnome is nearer a dwarf than a barbarian).
func (w *WLD) shapeDistance(tracks map[string]int32, own, other string, restNames []string) float64 {
	var a, b [][3]float64
	var sizeA, sizeB float64
	for _, rest := range restNames {
		suffix := strings.TrimPrefix(rest, own)
		ta, okA := tracks[own+suffix]
		tb, okB := tracks[other+suffix]
		if !okA || !okB {
			continue
		}
		pa, pb := w.trackPose(ta), w.trackPose(tb)
		va := [3]float64{pa[3], pa[7], pa[11]}
		vb := [3]float64{pb[3], pb[7], pb[11]}
		a, b = append(a, va), append(b, vb)
		sizeA += math.Sqrt(va[0]*va[0] + va[1]*va[1] + va[2]*va[2])
		sizeB += math.Sqrt(vb[0]*vb[0] + vb[1]*vb[1] + vb[2]*vb[2])
	}
	if len(a) == 0 || sizeA == 0 || sizeB == 0 {
		return math.Inf(1)
	}
	var d float64
	for i := range a {
		for k := 0; k < 3; k++ {
			d += math.Abs(a[i][k]/sizeA - b[i][k]/sizeB)
		}
	}
	return d
}

// animatedCodes lists the model codes that may have tracks for anim in a
// WLD: the three letters after the prefix of each track name, and, since
// Luclin tracks put a variant letter there first ("P01A"), the three after
// that too. A wrong reading names no tracks, so coverage rules it out.
func animatedCodes(tracks map[string]int32, anim string) []string {
	seen := map[string]bool{}
	for name := range tracks {
		rest, ok := strings.CutPrefix(name, anim)
		if !ok {
			continue
		}
		if len(rest) >= 3 {
			seen[rest[:3]] = true
		}
		if len(rest) >= 4 && (rest[0] == 'A' || rest[0] == 'B') {
			seen[rest[1:4]] = true
		}
	}
	out := make([]string, 0, len(seen))
	for c := range seen {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}
