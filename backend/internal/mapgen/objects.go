package mapgen

import (
	"encoding/binary"
	"errors"
	"io/fs"
	"math"
	"path/filepath"
	"strings"
)

// A zone's placed objects (trees, buildings, lamps, furniture) aren't part of
// its own geometry. objects.wld inside <zone>.s3d places them by actor name;
// the models are in <zone>_obj.s3d (and, for some Velious zones,
// <zone>_2_obj.s3d): a 0x14 actor names a 0x2D reference to a 0x36 mesh, or
// a skeleton for animated objects (skeleton.go).

const (
	fragActor   = 0x14
	fragMeshRef = 0x2D
)

// objYawSign is the direction a placement's yaw turns in raw WLD space.
// Measured on wall-mounted models across ten zones (torches, sconces,
// bookcases): with +1 they sit flush against their walls (Kael's torches,
// 603 vertices touching a wall against 18 the other way). Tools that report
// it negated have converted to a left-handed engine's axes.
const objYawSign = 1.0

// Door is a door the server places (the doors table), in game
// coordinates. Model names an actor in the zone's _obj archive without its
// _ACTORDEF suffix; Heading and Incline are in 1/512ths of a turn, and Size
// is a percentage.
type Door struct {
	Model            string
	X, Y, Z          float64
	Heading, Incline float64
	Size             int
}

// doorHeadingSign turns a door's heading into a placement yaw: the same
// direction as objects.wld yaw. Checked on plans of door headings 128 and
// 384, where the sign swings a door 180 degrees about its hinge, in four
// zones: this way every slab closes its doorway, the other way it lies
// along the wall beside it.
const doorHeadingSign = 1.0

// placement returns the door as an object placement. Game x and y are raw
// WLD y and x (see ToMapSpace).
func (d Door) placement() Placement {
	raw := Vec3{d.Y, d.X, d.Z}
	scale := float64(d.Size) / 100
	if scale <= 0 {
		scale = 1
	}
	return Placement{
		Pos:   ToMapSpace(raw.X, raw.Y, raw.Z),
		Raw:   raw,
		Actor: d.Model + "_ACTORDEF",
		Yaw:   doorHeadingSign * d.Heading,
		Tilt:  d.Incline,
		Scale: scale,
	}
}

// objArchives are the archives that can hold a zone's object models, in
// lookup order.
func objArchives(shortName string) []string {
	return []string{shortName + "_obj.s3d", shortName + "_2_obj.s3d"}
}

// sharedArchives hold models any zone can use, which doors mostly come from
// when a zone's own archives lack them: Kael's crates, the Plane of
// Knowledge books. They're read only for models a zone is missing, since
// gequip.s3d alone is 13 MB.
var sharedArchives = []string{"global_obj.s3d", "gequip.s3d"}

// objMargin is how far, in world units, a placement may sit outside the
// zone's geometry and still be drawn. Some zones park unused objects far
// below the world (gfaydark has them at z = -32768); left in, they'd
// stretch the 3D view's framing to nothing.
const objMargin = 500

// addObjects places every object from objects.wld, and the given doors,
// using models from the zone's _obj archives. Call it after the zone's own
// geometry is added: its bounds decide which placements are kept. A zone
// with no objects or no _obj archive adds nothing.
func (b *meshBuilder) addObjects(clientDir, shortName string, doors []Door) error {
	arc, err := OpenArchiveFiles(filepath.Join(clientDir, shortName+".s3d"), func(name string) bool {
		return strings.EqualFold(name, "objects.wld")
	})
	if err != nil {
		return err
	}
	var places []Placement
	if raw, ok := arc["objects.wld"]; ok {
		ow, err := ParseWLD(raw)
		if err != nil {
			return err
		}
		places = ow.Placements()
	}
	for _, d := range doors {
		places = append(places, d.placement())
	}
	if len(places) == 0 {
		return nil
	}

	type model struct {
		w      *WLD
		meshes []Mesh
	}
	// Only the actors placed are parsed; missing counts those not found yet.
	missing := map[string]bool{}
	for _, p := range places {
		missing[strings.ToUpper(p.Actor)] = true
	}
	models := map[string]model{}
	for i, name := range append(objArchives(shortName), sharedArchives...) {
		if len(missing) == 0 {
			break
		}
		w, err := loadArchiveWLD(filepath.Join(clientDir, name))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			if i >= len(objArchives(shortName)) {
				continue // a shared archive is only a fallback
			}
			return err
		}
		for actor, meshes := range w.actorMeshes(missing) {
			models[actor] = model{w, meshes}
			delete(missing, actor)
		}
	}
	lo, hi := b.bounds()
	inside := func(v Vec3) bool {
		return v.X >= lo.X-objMargin && v.X <= hi.X+objMargin &&
			v.Y >= lo.Y-objMargin && v.Y <= hi.Y+objMargin &&
			v.Z >= lo.Z-objMargin && v.Z <= hi.Z+objMargin
	}
	for _, p := range places {
		m, ok := models[strings.ToUpper(p.Actor)]
		if !ok || !inside(p.Pos) {
			continue
		}
		x := placeTransform(p)
		for _, mesh := range m.meshes {
			b.add(m.w, mesh, x, true)
		}
	}
	return nil
}

// bounds returns the extent of the geometry added so far.
func (b *meshBuilder) bounds() (lo, hi Vec3) {
	pos := b.out.Pos
	if len(pos) == 0 {
		return
	}
	lo = Vec3{math.Inf(1), math.Inf(1), math.Inf(1)}
	hi = Vec3{math.Inf(-1), math.Inf(-1), math.Inf(-1)}
	for i := 0; i+2 < len(pos); i += 3 {
		x, y, z := float64(pos[i]), float64(pos[i+1]), float64(pos[i+2])
		lo = Vec3{math.Min(lo.X, x), math.Min(lo.Y, y), math.Min(lo.Z, z)}
		hi = Vec3{math.Max(hi.X, x), math.Max(hi.Y, y), math.Max(hi.Z, z)}
	}
	return lo, hi
}

// placeTransform returns the map-space transform for one placement: scale,
// tilt, then yaw about the vertical, then move to the placement, all in raw
// WLD space (map space is a swap-and-negate of it; see ToMapSpace).
func placeTransform(p Placement) func(Vec3) Vec3 {
	const turn = 2 * math.Pi / 512
	sy, cy := math.Sincos(objYawSign * p.Yaw * turn)
	st, ct := math.Sincos(p.Tilt * turn)
	s := p.Scale
	return func(v Vec3) Vec3 {
		// Back to raw model space.
		x, y, z := -v.Y*s, -v.X*s, v.Z*s
		// Tilt about the raw y axis.
		x, z = x*ct+z*st, -x*st+z*ct
		// Yaw about the vertical.
		x, y = x*cy-y*sy, x*sy+y*cy
		return ToMapSpace(p.Raw.X+x, p.Raw.Y+y, p.Raw.Z+z)
	}
}

// actorMeshes maps each actor's name ("TREE1_ACTORDEF", uppercased) to its
// meshes: a static actor's own, or an animated one's posed at its first
// frame (see skeletonMeshes). With want set, only those actors are read.
func (w *WLD) actorMeshes(want map[string]bool) map[string][]Mesh {
	out := map[string][]Mesh{}
	for _, f := range w.Fragments {
		if f.kind != fragActor {
			continue
		}
		name := strings.ToUpper(w.str(f.nameRef))
		if want != nil && !want[name] {
			continue
		}
		var meshes []Mesh
		for _, r := range actorComponents(f.payload) {
			if m, ok := w.meshRef(r); ok {
				meshes = append(meshes, m)
			} else {
				meshes = append(meshes, w.skeletonMeshes(r)...)
			}
		}
		if len(meshes) > 0 {
			out[name] = meshes
		}
	}
	return out
}

// actorComponents reads a 0x14 actor's component references: after flags,
// a callback name, an entry count, a component count and a reference, two
// optional parameter blocks (flag bits 0 and 1), then the entries, each a
// count of 8-byte pairs.
func actorComponents(b []byte) []int32 {
	le := binary.LittleEndian
	if len(b) < 20 {
		return nil
	}
	flags := le.Uint32(b)
	entries := int(le.Uint32(b[8:]))
	components := int(le.Uint32(b[12:]))
	p := 20
	if flags&1 != 0 {
		p += 4
	}
	if flags&2 != 0 {
		p += 28
	}
	for i := 0; i < entries; i++ {
		if p+4 > len(b) {
			return nil
		}
		p += 4 + 8*int(le.Uint32(b[p:]))
	}
	var out []int32
	for i := 0; i < components && p+4 <= len(b); i++ {
		out = append(out, int32(le.Uint32(b[p:])))
		p += 4
	}
	return out
}

// loadArchiveWLD reads an archive whose only geometry is one .wld, like an
// _obj archive, and parses it.
func loadArchiveWLD(path string) (*WLD, error) {
	arc, err := OpenArchiveFiles(path, func(name string) bool {
		return strings.HasSuffix(strings.ToLower(name), ".wld")
	})
	if err != nil {
		return nil, err
	}
	for _, raw := range arc {
		return ParseWLD(raw)
	}
	return nil, errors.New(filepath.Base(path) + ": no .wld inside archive")
}
