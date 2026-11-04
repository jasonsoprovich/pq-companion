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
// <zone>_2_obj.s3d): a 0x14 actor names a 0x2D reference to a 0x36 mesh.

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

// objArchives are the archives that can hold a zone's object models, in
// lookup order.
func objArchives(shortName string) []string {
	return []string{shortName + "_obj.s3d", shortName + "_2_obj.s3d"}
}

// objMargin is how far, in world units, a placement may sit outside the
// zone's geometry and still be drawn. Some zones park unused objects far
// below the world (gfaydark has them at z = -32768); left in, they'd
// stretch the 3D view's framing to nothing.
const objMargin = 500

// addObjects places every object from objects.wld using models from the
// zone's _obj archives. Call it after the zone's own geometry is added: its
// bounds decide which placements are kept. A zone with no objects or no _obj
// archive adds nothing.
func (b *meshBuilder) addObjects(clientDir, shortName string) error {
	arc, err := OpenArchiveFiles(filepath.Join(clientDir, shortName+".s3d"), func(name string) bool {
		return strings.EqualFold(name, "objects.wld")
	})
	if err != nil {
		return err
	}
	raw, ok := arc["objects.wld"]
	if !ok {
		return nil
	}
	ow, err := ParseWLD(raw)
	if err != nil {
		return err
	}
	places := ow.Placements()
	if len(places) == 0 {
		return nil
	}

	type model struct {
		w      *WLD
		meshes []Mesh
	}
	models := map[string]model{}
	for _, name := range objArchives(shortName) {
		w, err := loadArchiveWLD(filepath.Join(clientDir, name))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		for actor, meshes := range w.actorMeshes() {
			if _, ok := models[actor]; !ok {
				models[actor] = model{w, meshes}
			}
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

// actorMeshes maps each static actor's name ("TREE1_ACTORDEF", uppercased)
// to its meshes. Animated actors, which point at a skeleton instead, are
// left out.
func (w *WLD) actorMeshes() map[string][]Mesh {
	out := map[string][]Mesh{}
	for _, f := range w.Fragments {
		if f.kind != fragActor {
			continue
		}
		refs := actorComponents(f.payload)
		var meshes []Mesh
		for _, r := range refs {
			i, ok := w.ref(r, fragMeshRef)
			if !ok || len(w.Fragments[i].payload) < 4 {
				continue
			}
			j, ok := w.ref(int32(binary.LittleEndian.Uint32(w.Fragments[i].payload)), fragMesh)
			if !ok {
				continue
			}
			m, err := parseMesh(w.Fragments[j].payload, w.Version)
			if err != nil {
				continue
			}
			meshes = append(meshes, m)
		}
		if len(meshes) > 0 {
			out[strings.ToUpper(w.str(f.nameRef))] = meshes
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
