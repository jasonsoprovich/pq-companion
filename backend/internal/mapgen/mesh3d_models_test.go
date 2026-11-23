package mapgen

import (
	"encoding/binary"
	"math"
	"reflect"
	"testing"
)

// decodeMesh3D reads Encode's layout back, as the frontend's parseMesh3D
// does, checking every 4-byte block starts aligned.
func decodeMesh3D(t *testing.T, b []byte) *Mesh3D {
	t.Helper()
	le := binary.LittleEndian
	p := 24
	u32 := func() uint32 { v := le.Uint32(b[p:]); p += 4; return v }
	aligned := func(what string) {
		if p%4 != 0 {
			t.Fatalf("%s starts at %d, not 4-byte aligned", what, p)
		}
	}
	fl := func(n int) []float32 {
		aligned("float block")
		out := make([]float32, n)
		for i := range out {
			out[i] = math.Float32frombits(u32())
		}
		return out
	}
	geom := func(ni, ng int) ([]uint32, []Group3D) {
		aligned("index block")
		idx := make([]uint32, ni)
		for i := range idx {
			idx[i] = u32()
		}
		gs := make([]Group3D, ng)
		for i := range gs {
			gs[i] = Group3D{Start: u32(), Count: u32(), Material: u32()}
		}
		return idx, gs
	}
	v, ni, ng, nm, nmd := le.Uint32(b[4:]), le.Uint32(b[8:]), le.Uint32(b[12:]), le.Uint32(b[16:]), le.Uint32(b[20:])
	m := &Mesh3D{}
	m.Pos, m.UV = fl(int(v)*3), fl(int(v)*2)
	m.Idx, m.Groups = geom(int(ni), int(ng))
	for range nm {
		n := int(le.Uint16(b[p+4:]))
		m.Materials = append(m.Materials, Material3D{
			Kind: b[p], Opacity: float32(b[p+1]) / 255, Object: b[p+2] == 1, Texture: string(b[p+6 : p+6+n]),
		})
		p += 6 + n
	}
	p = (p + 3) &^ 3
	for range nmd {
		aligned("model header")
		mv, mi, mg, inst := u32(), u32(), u32(), u32()
		var md Model3D
		md.Pos, md.UV = fl(int(mv)*3), fl(int(mv)*2)
		md.Idx, md.Groups = geom(int(mi), int(mg))
		for range inst {
			var x [12]float32
			copy(x[:], fl(12))
			md.Instances = append(md.Instances, x)
		}
		m.Models = append(m.Models, md)
	}
	if p != len(b) {
		t.Fatalf("decoded %d of %d bytes", p, len(b))
	}
	return m
}

func TestMesh3DEncodeModelsRoundTrip(t *testing.T) {
	m := &Mesh3D{
		Pos: []float32{0, 0, 0, 1, 0, 0, 0, 1, 0}, UV: []float32{0, 0, 1, 0, 0, 1},
		Idx: []uint32{0, 2, 1}, Groups: []Group3D{{0, 3, 0}},
		// "abc.bmp" is 7 bytes: the table ends 1 byte past a boundary.
		Materials: []Material3D{{Kind: MatSolid, Opacity: 1}, {Texture: "abc.bmp", Kind: MatMasked, Opacity: 1, Object: true}},
		Models: []Model3D{
			{
				Pos: []float32{0, 0, 0, 2, 0, 0, 0, 2, 0}, UV: []float32{0, 0, 1, 0, 0, 1},
				Idx: []uint32{0, 2, 1}, Groups: []Group3D{{0, 3, 1}},
				Instances: [][12]float32{{1, 0, 0, 5, 0, 1, 0, 6, 0, 0, 1, 7}, {2, 0, 0, -5, 0, 2, 0, -6, 0, 0, 2, 0}},
			},
			{Pos: []float32{1, 1, 1}, UV: []float32{0, 0}, Idx: nil, Groups: nil, Instances: [][12]float32{{1}}},
		},
	}
	b := m.Encode()
	if len(b)%4 != 0 {
		t.Errorf("encoded length %d not a multiple of 4", len(b))
	}
	got := decodeMesh3D(t, b)
	for i := range m.Materials {
		got.Materials[i].Opacity = m.Materials[i].Opacity
	}
	got.Models[1].Idx, got.Models[1].Groups = nil, nil
	if !reflect.DeepEqual(got, m) {
		t.Errorf("round trip:\n got %+v\nwant %+v", got, m)
	}
	if m.Triangles() != 1+2 {
		t.Errorf("Triangles = %d, want 3", m.Triangles())
	}
}

// Character and item meshes have no models: the format still ends at the
// padded materials.
func TestMesh3DEncodeNoModels(t *testing.T) {
	m := &Mesh3D{Pos: []float32{0, 0, 0}, UV: []float32{0, 0}, Materials: []Material3D{{Texture: "a.bmp", Opacity: 1}}}
	b := m.Encode()
	got := decodeMesh3D(t, b)
	if len(got.Models) != 0 || got.Materials[0].Texture != "a.bmp" {
		t.Errorf("got %+v", got)
	}
}
