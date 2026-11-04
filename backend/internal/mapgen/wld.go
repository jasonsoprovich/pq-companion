package mapgen

import (
	"encoding/binary"
	"fmt"
	"math"
)

const (
	wldMagic = 0x54503D02

	// wldVerOld is the Trilogy/Mac format used by every TAKP zone. wldVerNew is
	// the later PC format; the only difference that reaches us is texture-coord
	// width inside a mesh, which we skip over either way.
	wldVerOld = 0x00015500
	wldVerNew = 0x1000C800

	fragMesh           = 0x36 // DmSpriteDef2 — zone geometry
	fragObjectLocation = 0x15 // placed object instance

	// polyPermeable marks a non-collidable face (water surfaces, decorative
	// planes). Excluded from every extractor: you can't stand on it.
	polyPermeable = 0x10
)

// stringHashKey de-obfuscates the WLD string table (a repeating XOR).
var stringHashKey = [8]byte{0x95, 0x3A, 0xC5, 0x2A, 0x95, 0x7A, 0x95, 0x6A}

type fragment struct {
	kind    uint32
	nameRef int32 // negated string-table offset of the fragment's name
	payload []byte
}

// WLD is a parsed .wld file: its fragments plus the decoded string table.
type WLD struct {
	Version   uint32
	Strings   []byte
	Fragments []fragment
}

// Vec3 is a point in map space (see ToMapSpace for the axis convention).
type Vec3 struct{ X, Y, Z float64 }

// Triangle indexes three vertices in the owning mesh's vertex slice. Mat
// indexes the mesh's material list (-1 when the mesh gives none).
type Triangle struct {
	Flags   uint16
	A, B, C int
	Mat     int
}

// Mesh is one 0x36 fragment's geometry. UVs holds a u, v pair per vertex
// (zeros where the fragment has fewer texture coords than vertices), and
// MaterialList is the 0-based index of its 0x31 fragment, or -1. Pieces,
// in a skeletal model's mesh, assign runs of vertices to bones in order.
type Mesh struct {
	Vertices     []Vec3
	Triangles    []Triangle
	UVs          []float32
	MaterialList int
	Pieces       []VertexPiece
}

// VertexPiece is a run of Count vertices that follow bone Bone.
type VertexPiece struct{ Count, Bone int }

// ParseWLD decodes a .wld payload into fragments.
func ParseWLD(data []byte) (*WLD, error) {
	if len(data) < 28 {
		return nil, fmt.Errorf("too short to be a WLD (%d bytes)", len(data))
	}
	if magic := binary.LittleEndian.Uint32(data); magic != wldMagic {
		return nil, fmt.Errorf("bad WLD magic 0x%08X, want 0x%08X", magic, wldMagic)
	}
	version := binary.LittleEndian.Uint32(data[4:])
	fragCount := binary.LittleEndian.Uint32(data[8:])
	hashSize := binary.LittleEndian.Uint32(data[20:])

	p := 28
	if p+int(hashSize) > len(data) {
		return nil, fmt.Errorf("string table (%d bytes) past end of file", hashSize)
	}
	strings := make([]byte, hashSize)
	for i := 0; i < int(hashSize); i++ {
		strings[i] = data[p+i] ^ stringHashKey[i%8]
	}
	p += int(hashSize)

	frags := make([]fragment, 0, fragCount)
	for i := uint32(0); i < fragCount; i++ {
		if p+8 > len(data) {
			return nil, fmt.Errorf("fragment %d header past end of file", i)
		}
		size := binary.LittleEndian.Uint32(data[p:])
		kind := binary.LittleEndian.Uint32(data[p+4:])
		// size counts the 4-byte name reference plus the payload.
		if size < 4 || p+8+int(size) > len(data) {
			return nil, fmt.Errorf("fragment %d size %d past end of file", i, size)
		}
		frags = append(frags, fragment{
			kind:    kind,
			nameRef: int32(binary.LittleEndian.Uint32(data[p+8:])),
			payload: data[p+12 : p+8+int(size)],
		})
		p += 8 + int(size)
	}
	return &WLD{Version: version, Strings: strings, Fragments: frags}, nil
}

// str reads a NUL-terminated name from the string table by its negated
// offset (the form fragments store), or "" when out of range.
func (w *WLD) str(ref int32) string {
	off := -int(ref)
	if off < 0 || off >= len(w.Strings) {
		return ""
	}
	end := off
	for end < len(w.Strings) && w.Strings[end] != 0 {
		end++
	}
	return string(w.Strings[off:end])
}

// ToMapSpace converts raw WLD mesh coordinates into map space.
//
// Verified empirically (docs/maps-feasibility.md 5b.2) by matching objects.wld
// placements against the doors and spawn2 tables across all 8 axis
// permutations on three zones:
//
//	game_x = mesh_Y      game_y = mesh_X      game_z = mesh_Z
//	map_f1 = -game_x     map_f2 = -game_y
//
// which composes to a swap-and-negate. Bounding-box comparison cannot verify
// this — near-symmetric zones hide a sign flip — so it was confirmed by
// rendering and comparing against a known-good reference map.
func ToMapSpace(meshX, meshY, meshZ float64) Vec3 {
	return Vec3{X: -meshY, Y: -meshX, Z: meshZ}
}

// Meshes extracts every 0x36 geometry fragment, already in map space.
func (w *WLD) Meshes() []Mesh {
	var out []Mesh
	for _, f := range w.Fragments {
		if f.kind != fragMesh {
			continue
		}
		m, err := parseMesh(f.payload, w.Version)
		if err != nil {
			// A malformed fragment costs detail, not correctness; the rest of
			// the zone still renders. Skipping matches the reference pipeline.
			continue
		}
		out = append(out, m)
	}
	return out
}

func parseMesh(b []byte, version uint32) (Mesh, error) {
	// flags, 4 fragment refs, center xyz, params2[3], maxDistance, min/max xyz
	const headerBytes = 20 + 12 + 12 + 4 + 24
	if len(b) < headerBytes+20 {
		return Mesh{}, fmt.Errorf("mesh payload too short (%d bytes)", len(b))
	}
	matList := int(int32(binary.LittleEndian.Uint32(b[4:]))) - 1
	cx := float64(math.Float32frombits(binary.LittleEndian.Uint32(b[20:])))
	cy := float64(math.Float32frombits(binary.LittleEndian.Uint32(b[24:])))
	cz := float64(math.Float32frombits(binary.LittleEndian.Uint32(b[28:])))

	p := headerBytes
	u16 := func(off int) int { return int(int16(binary.LittleEndian.Uint16(b[off:]))) }
	vertexCount := u16(p)
	texCoordCount := u16(p + 2)
	normalCount := u16(p + 4)
	colorCount := u16(p + 6)
	polyCount := u16(p + 8)
	pieceCount := u16(p + 10)
	polyTexCount := u16(p + 12)
	scale := u16(p + 18)
	p += 20

	if vertexCount < 0 || polyCount < 0 || scale < 0 || scale > 30 {
		return Mesh{}, fmt.Errorf("implausible mesh header")
	}
	inv := 1.0 / float64(int(1)<<uint(scale))

	if p+vertexCount*6 > len(b) {
		return Mesh{}, fmt.Errorf("vertex block past end of fragment")
	}
	verts := make([]Vec3, 0, vertexCount)
	for i := 0; i < vertexCount; i++ {
		x := float64(int16(binary.LittleEndian.Uint16(b[p:]))) * inv
		y := float64(int16(binary.LittleEndian.Uint16(b[p+2:]))) * inv
		z := float64(int16(binary.LittleEndian.Uint16(b[p+4:]))) * inv
		verts = append(verts, ToMapSpace(cx+x, cy+y, cz+z))
		p += 6
	}

	// Texture coords: int16 pairs over 256 in the old format, float32 pairs
	// in the new. Normals and vertex colors are skipped.
	texWidth := 4
	if version == wldVerNew {
		texWidth = 8
	}
	if texCoordCount < 0 || p+texCoordCount*texWidth > len(b) {
		return Mesh{}, fmt.Errorf("texture coords past end of fragment")
	}
	uvs := make([]float32, 2*vertexCount)
	for i := 0; i < texCoordCount && i < vertexCount; i++ {
		q := p + i*texWidth
		if version == wldVerNew {
			uvs[2*i] = math.Float32frombits(binary.LittleEndian.Uint32(b[q:]))
			uvs[2*i+1] = math.Float32frombits(binary.LittleEndian.Uint32(b[q+4:]))
		} else {
			uvs[2*i] = float32(int16(binary.LittleEndian.Uint16(b[q:]))) / 256
			uvs[2*i+1] = float32(int16(binary.LittleEndian.Uint16(b[q+2:]))) / 256
		}
	}
	p += texCoordCount*texWidth + normalCount*3 + colorCount*4
	if p < 0 || p > len(b) {
		return Mesh{}, fmt.Errorf("attribute blocks past end of fragment")
	}

	if p+polyCount*8 > len(b) {
		return Mesh{}, fmt.Errorf("polygon block past end of fragment")
	}
	polys := p
	p += polyCount * 8

	// Polygon materials: runs of (polygon count, material index) covering the
	// polygons in order, after the vertex pieces. Missing or short runs leave
	// the remaining polygons without a material.
	mats := make([]int, polyCount)
	for i := range mats {
		mats[i] = -1
	}
	var pieces []VertexPiece
	for i := 0; i < pieceCount && p+4 <= len(b); i++ {
		pieces = append(pieces, VertexPiece{
			Count: int(binary.LittleEndian.Uint16(b[p:])),
			Bone:  int(binary.LittleEndian.Uint16(b[p+2:])),
		})
		p += 4
	}
	next := 0
	for i := 0; i < polyTexCount && p+4 <= len(b); i++ {
		n := int(binary.LittleEndian.Uint16(b[p:]))
		m := int(binary.LittleEndian.Uint16(b[p+2:]))
		p += 4
		for j := 0; j < n && next < polyCount; j++ {
			mats[next] = m
			next++
		}
	}

	tris := make([]Triangle, 0, polyCount)
	for i := 0; i < polyCount; i++ {
		q := polys + i*8
		t := Triangle{
			Flags: binary.LittleEndian.Uint16(b[q:]),
			A:     int(binary.LittleEndian.Uint16(b[q+2:])),
			B:     int(binary.LittleEndian.Uint16(b[q+4:])),
			C:     int(binary.LittleEndian.Uint16(b[q+6:])),
			Mat:   mats[i],
		}
		if t.A >= vertexCount || t.B >= vertexCount || t.C >= vertexCount {
			continue // corrupt index; drop the face rather than the mesh
		}
		tris = append(tris, t)
	}
	return Mesh{Vertices: verts, Triangles: tris, UVs: uvs, MaterialList: matList, Pieces: pieces}, nil
}

// Placement is a placed object instance from objects.wld. Pos is in map
// space (verified against the doors and spawn2 tables); Raw is the same
// point in raw WLD coordinates. Actor names the object's 0x14 definition in
// the zone's _obj archive ("TREE1_ACTORDEF"). Yaw and Tilt are rotations in
// 1/512ths of a turn: yaw about the vertical axis, set on nearly every
// object, and tilt, which a few use. Scale is uniform.
type Placement struct {
	Pos       Vec3
	Raw       Vec3
	Actor     string
	Yaw, Tilt float64
	Scale     float64
}

// Placements extracts every 0x15 object-location fragment: the actor name
// reference, flags, a fragment reference, position, three rotation values
// (the third is never used), three scale values (only the second is used),
// then a vertex-colour reference.
func (w *WLD) Placements() []Placement {
	var out []Placement
	for _, f := range w.Fragments {
		if f.kind != fragObjectLocation || len(f.payload) < 24 {
			continue
		}
		b := f.payload
		f32 := func(off int) float64 {
			if off+4 > len(b) {
				return 0
			}
			return float64(math.Float32frombits(binary.LittleEndian.Uint32(b[off:])))
		}
		x, y, z := f32(12), f32(16), f32(20)
		p := Placement{
			Pos:   ToMapSpace(x, y, z),
			Raw:   Vec3{x, y, z},
			Actor: w.str(int32(binary.LittleEndian.Uint32(b))),
			Yaw:   f32(24),
			Tilt:  f32(28),
			Scale: f32(40),
		}
		if p.Scale == 0 {
			p.Scale = 1
		}
		out = append(out, p)
	}
	return out
}
