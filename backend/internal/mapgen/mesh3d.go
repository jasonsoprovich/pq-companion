package mapgen

import (
	"encoding/binary"
	"fmt"
	"log/slog"
	"math"
	"path/filepath"
	"strings"
)

// mesh3DMagic opens an encoded 3D mesh ("PQM2").
var mesh3DMagic = [4]byte{'P', 'Q', 'M', '2'}

const (
	fragBitmapName = 0x03 // texture file names
	fragBitmapInfo = 0x04 // texture frames
	fragBitmapRef  = 0x05 // reference to a 0x04
	fragMaterial   = 0x30
	fragMatList    = 0x31
)

// Material kinds for the 3D view, from a 0x30 material's render method.
const (
	MatSolid    = 0 // opaque
	MatMasked   = 1 // colour-keyed cutouts: foliage, fences, grates
	MatBlend    = 2 // translucent: water, glass
	MatAdditive = 3 // glowing: lava, magic
)

// Material3D is one material of a 3D mesh. Texture is the bitmap's file name
// in the zone or _obj archive ("" for none; animated textures use their
// first frame), Opacity applies to MatBlend, and Object marks a placed
// object's material rather than the zone's own.
type Material3D struct {
	Texture string
	Kind    uint8
	Opacity float32
	Object  bool
}

// Group3D is a run of indices drawn with one material.
type Group3D struct {
	Start, Count uint32
	Material     uint32
}

// Mesh3D is a zone's visible geometry for the 3D view: map-space positions
// (x, y, z per vertex), texture coords (u, v per vertex) and triangle
// indices grouped by material. Faces with invisible materials (zone
// boundaries, collision-only walls, sky) are left out.
type Mesh3D struct {
	Pos       []float32
	UV        []float32
	Idx       []uint32
	Groups    []Group3D
	Materials []Material3D
}

// LoadMesh3D reads <clientDir>/<shortName>.s3d and returns its 3D mesh,
// with the zone's placed objects (trees, buildings, furniture) from its
// _obj archives when the client has them. Unlike LoadZone, vertices aren't
// welded: neighbouring fragments that meet at a point usually give it
// different texture coords.
func LoadMesh3D(clientDir, shortName string) (*Mesh3D, error) {
	w, err := loadZoneWLD(clientDir, shortName)
	if err != nil {
		return nil, err
	}
	b := newMeshBuilder()
	for _, m := range w.Meshes() {
		b.add(w, m, nil, false)
	}
	if err := b.addObjects(clientDir, shortName); err != nil {
		// Objects are extra detail; the zone still draws without them.
		slog.Warn("mapgen: zone objects skipped", "zone", shortName, "err", err)
	}
	return b.build(), nil
}

// Mesh3D builds the 3D mesh from the WLD's own geometry fragments.
func (w *WLD) Mesh3D() *Mesh3D {
	b := newMeshBuilder()
	for _, m := range w.Meshes() {
		b.add(w, m, nil, false)
	}
	return b.build()
}

// matKey identifies a material: a 0x30 fragment of one WLD, or a WLD's
// stand-in for faces without one (frag -1).
type matKey struct {
	w    *WLD
	frag int
}

// meshBuilder collects geometry from one or more WLDs into a Mesh3D,
// deduplicating materials and grouping triangles by material.
type meshBuilder struct {
	out   *Mesh3D
	byMat [][]uint32
	// matID maps a material to its place in out.Materials, or -1 when it's
	// invisible.
	matID map[matKey]int
}

func newMeshBuilder() *meshBuilder {
	return &meshBuilder{out: &Mesh3D{}, matID: map[matKey]int{}}
}

func (b *meshBuilder) material(w *WLD, frag int, object bool) int {
	k := matKey{w, frag}
	if id, ok := b.matID[k]; ok {
		return id
	}
	id := -1
	m, ok := Material3D{Kind: MatSolid, Opacity: 1}, true
	if frag >= 0 {
		m, ok = w.material(frag)
	}
	if ok {
		m.Object = object
		id = len(b.out.Materials)
		b.out.Materials = append(b.out.Materials, m)
		b.byMat = append(b.byMat, nil)
	}
	b.matID[k] = id
	return id
}

// add appends one mesh fragment. xform, when set, places a map-space vertex
// (an object's model space) into the zone.
func (b *meshBuilder) add(w *WLD, m Mesh, xform func(Vec3) Vec3, object bool) {
	list := w.materialList(m.MaterialList)
	remap := make([]int32, len(m.Vertices))
	for i := range remap {
		remap[i] = -1
	}
	out := b.out
	use := func(v int) uint32 {
		if remap[v] < 0 {
			remap[v] = int32(len(out.Pos) / 3)
			p := m.Vertices[v]
			if xform != nil {
				p = xform(p)
			}
			out.Pos = append(out.Pos, float32(p.X), float32(p.Y), float32(p.Z))
			out.UV = append(out.UV, m.UVs[2*v], m.UVs[2*v+1])
		}
		return uint32(remap[v])
	}
	for _, t := range m.Triangles {
		// Faces without a material to go by are drawn plain, as before
		// textures.
		frag := -1
		if t.Mat >= 0 && t.Mat < len(list) {
			frag = list[t.Mat]
		}
		id := b.material(w, frag, object)
		if id < 0 {
			continue
		}
		// Reversed: after the map-space swap and the 3D view's y flip,
		// the client's front faces wind clockwise (floors face down
		// otherwise), and three.js takes counter-clockwise as the front.
		b.byMat[id] = append(b.byMat[id], use(t.A), use(t.C), use(t.B))
	}
}

func (b *meshBuilder) build() *Mesh3D {
	out := b.out
	for id, idx := range b.byMat {
		if len(idx) == 0 {
			continue
		}
		out.Groups = append(out.Groups, Group3D{
			Start: uint32(len(out.Idx)), Count: uint32(len(idx)), Material: uint32(id),
		})
		out.Idx = append(out.Idx, idx...)
	}
	return out
}

// ref turns a fragment reference (1-based, 0 for none) into an index into
// Fragments, checking its kind.
func (w *WLD) ref(r int32, kind uint32) (int, bool) {
	i := int(r) - 1
	if i < 0 || i >= len(w.Fragments) || w.Fragments[i].kind != kind {
		return 0, false
	}
	return i, true
}

// materialList returns the 0x30 fragment indices of a 0x31 list.
func (w *WLD) materialList(frag int) []int {
	if frag < 0 || frag >= len(w.Fragments) || w.Fragments[frag].kind != fragMatList {
		return nil
	}
	b := w.Fragments[frag].payload
	if len(b) < 8 {
		return nil
	}
	n := int(binary.LittleEndian.Uint32(b[4:]))
	out := make([]int, 0, n)
	for i := 0; i < n && 8+4*i+4 <= len(b); i++ {
		r := int32(binary.LittleEndian.Uint32(b[8+4*i:]))
		m, ok := w.ref(r, fragMaterial)
		if !ok {
			m = -1
		}
		out = append(out, m)
	}
	return out
}

// material reads a 0x30 fragment: flags, render method, pen colour,
// brightness, scaled ambient, then a 0x05 reference. ok is false for
// materials that aren't drawn.
func (w *WLD) material(frag int) (Material3D, bool) {
	if frag < 0 || frag >= len(w.Fragments) {
		return Material3D{}, false
	}
	b := w.Fragments[frag].payload
	if len(b) < 24 {
		return Material3D{}, false
	}
	tex := w.bitmapRefName(int32(binary.LittleEndian.Uint32(b[20:])))
	m := Material3D{Texture: tex, Kind: MatSolid, Opacity: 1}
	switch binary.LittleEndian.Uint32(b[4:]) &^ 0x80000000 {
	case 0x00, 0x03, 0x4B, 0x53:
		return m, false // boundary or invisible
	case 0x0D, 0x0F, 0x10:
		return m, false // sky dome
	case 0x05:
		m.Kind, m.Opacity = MatBlend, 0.5
	case 0x09:
		m.Kind, m.Opacity = MatBlend, 0.25
	case 0x0A:
		m.Kind, m.Opacity = MatBlend, 0.75
	case 0x0B, 0x17:
		m.Kind = MatAdditive
	case 0x13, 0x19:
		m.Kind = MatMasked
	default:
		if tex == "" {
			return m, false
		}
	}
	return m, true
}

// bitmapRefName follows a 0x05 reference through its 0x04 to the first
// 0x03 file name.
func (w *WLD) bitmapRefName(r int32) string {
	i, ok := w.ref(r, fragBitmapRef)
	if !ok || len(w.Fragments[i].payload) < 4 {
		return ""
	}
	i, ok = w.ref(int32(binary.LittleEndian.Uint32(w.Fragments[i].payload)), fragBitmapInfo)
	if !ok {
		return ""
	}
	b := w.Fragments[i].payload
	if len(b) < 8 {
		return ""
	}
	flags := binary.LittleEndian.Uint32(b)
	p := 8
	if flags&0x4 != 0 {
		p += 4 // current frame
	}
	if flags&0x8 != 0 {
		p += 4 // frame delay, for animated textures
	}
	if p+4 > len(b) {
		return ""
	}
	i, ok = w.ref(int32(binary.LittleEndian.Uint32(b[p:])), fragBitmapName)
	if !ok {
		return ""
	}
	return bitmapName(w.Fragments[i].payload)
}

// bitmapName decodes a 0x03 fragment's first file name: after a count, a
// uint16 length (with the NUL) and the name, obfuscated like the string
// table.
func bitmapName(b []byte) string {
	if len(b) < 6 {
		return ""
	}
	n := int(binary.LittleEndian.Uint16(b[4:]))
	if n < 2 || 6+n > len(b) {
		return ""
	}
	name := make([]byte, n-1)
	for i := range name {
		name[i] = b[6+i] ^ stringHashKey[i%8]
	}
	return strings.ToLower(string(name))
}

// loadZoneWLD reads and parses the zone .wld from <shortName>.s3d.
func loadZoneWLD(clientDir, shortName string) (*WLD, error) {
	path := filepath.Join(clientDir, shortName+".s3d")
	// Only the .wld entries are used; the textures are skipped.
	arc, err := OpenArchiveFiles(path, func(name string) bool {
		return strings.HasSuffix(strings.ToLower(name), ".wld")
	})
	if err != nil {
		return nil, err
	}
	raw, ok := arc[shortName+".wld"]
	if !ok {
		// A few archives name the main WLD differently; fall back to the only
		// .wld that isn't one of the known auxiliary files.
		for name, b := range arc {
			l := strings.ToLower(name)
			if strings.HasSuffix(l, ".wld") && l != "objects.wld" && l != "lights.wld" {
				raw, ok = b, true
				break
			}
		}
	}
	if !ok {
		return nil, fmt.Errorf("%s: no zone .wld inside archive", shortName)
	}
	w, err := ParseWLD(raw)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", shortName, err)
	}
	return w, nil
}

// Encode packs the mesh little-endian: the magic; vertex, index, group and
// material counts as uint32; float32 positions, float32 texture coords,
// uint32 indices, then each group's start, count and material as uint32.
// Materials follow as a kind byte, an opacity byte (0-255), a flags byte
// (1: object), a zero byte, and a uint16 length and texture name. Every block before the materials is 4-byte
// aligned, so the reader can view them without copying.
func (m *Mesh3D) Encode() []byte {
	size := 20 + 4*len(m.Pos) + 4*len(m.UV) + 4*len(m.Idx) + 12*len(m.Groups)
	for _, mat := range m.Materials {
		size += 6 + len(mat.Texture)
	}
	out := make([]byte, size)
	copy(out, mesh3DMagic[:])
	le := binary.LittleEndian
	le.PutUint32(out[4:], uint32(len(m.Pos)/3))
	le.PutUint32(out[8:], uint32(len(m.Idx)))
	le.PutUint32(out[12:], uint32(len(m.Groups)))
	le.PutUint32(out[16:], uint32(len(m.Materials)))
	p := 20
	for _, f := range m.Pos {
		le.PutUint32(out[p:], math.Float32bits(f))
		p += 4
	}
	for _, f := range m.UV {
		le.PutUint32(out[p:], math.Float32bits(f))
		p += 4
	}
	for _, i := range m.Idx {
		le.PutUint32(out[p:], i)
		p += 4
	}
	for _, g := range m.Groups {
		le.PutUint32(out[p:], g.Start)
		le.PutUint32(out[p+4:], g.Count)
		le.PutUint32(out[p+8:], g.Material)
		p += 12
	}
	for _, mat := range m.Materials {
		out[p] = mat.Kind
		out[p+1] = uint8(math.Round(float64(mat.Opacity) * 255))
		if mat.Object {
			out[p+2] = 1
		}
		le.PutUint16(out[p+4:], uint16(len(mat.Texture)))
		copy(out[p+6:], mat.Texture)
		p += 6 + len(mat.Texture)
	}
	return out
}
