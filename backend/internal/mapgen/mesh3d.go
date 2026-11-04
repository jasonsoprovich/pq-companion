package mapgen

import (
	"encoding/binary"
	"math"
)

// mesh3DMagic opens an encoded 3D mesh ("PQM1").
var mesh3DMagic = [4]byte{'P', 'Q', 'M', '1'}

// Mesh3D returns the zone's solid geometry for the 3D view: map-space
// positions (x, y, z per vertex) and triangle indices. Permeable faces
// (water surfaces, decorative planes) are dropped and the vertices they
// alone used are left out.
func (z *Zone) Mesh3D() (pos []float32, idx []uint32) {
	remap := make([]int32, len(z.Vertices))
	for i := range remap {
		remap[i] = -1
	}
	use := func(v int) uint32 {
		if remap[v] < 0 {
			remap[v] = int32(len(pos) / 3)
			p := z.Vertices[v]
			pos = append(pos, float32(p.X), float32(p.Y), float32(p.Z))
		}
		return uint32(remap[v])
	}
	for _, t := range z.Triangles {
		if t.Flags&polyPermeable != 0 {
			continue
		}
		idx = append(idx, use(t.A), use(t.B), use(t.C))
	}
	return pos, idx
}

// EncodeMesh3D packs a mesh little-endian: the magic, the vertex count and
// the index count as uint32, then the float32 positions and uint32 indices.
func EncodeMesh3D(pos []float32, idx []uint32) []byte {
	out := make([]byte, 12+4*len(pos)+4*len(idx))
	copy(out, mesh3DMagic[:])
	binary.LittleEndian.PutUint32(out[4:], uint32(len(pos)/3))
	binary.LittleEndian.PutUint32(out[8:], uint32(len(idx)))
	p := 12
	for _, f := range pos {
		binary.LittleEndian.PutUint32(out[p:], math.Float32bits(f))
		p += 4
	}
	for _, i := range idx {
		binary.LittleEndian.PutUint32(out[p:], i)
		p += 4
	}
	return out
}
