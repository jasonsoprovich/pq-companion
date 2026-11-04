package mapgen

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestMesh3DEncode(t *testing.T) {
	m := &Mesh3D{
		Pos:       []float32{0, 0, 0, 10, 0, 0, 0, 10, 5},
		UV:        []float32{0, 0, 1, 0, 0, 1},
		Idx:       []uint32{0, 2, 1},
		Groups:    []Group3D{{Start: 0, Count: 3, Material: 1}},
		Materials: []Material3D{{Kind: MatSolid, Opacity: 1}, {Texture: "water.bmp", Kind: MatBlend, Opacity: 0.5}},
	}
	b := m.Encode()
	if string(b[:4]) != "PQM2" {
		t.Fatalf("magic = %q", b[:4])
	}
	le := binary.LittleEndian
	if v, i, g, n := le.Uint32(b[4:]), le.Uint32(b[8:]), le.Uint32(b[12:]), le.Uint32(b[16:]); v != 3 || i != 3 || g != 1 || n != 2 {
		t.Fatalf("counts = %d %d %d %d", v, i, g, n)
	}
	uv := 20 + 9*4
	if got := math.Float32frombits(le.Uint32(b[uv+2*4:])); got != 1 {
		t.Fatalf("second vertex u = %v", got)
	}
	idx := uv + 6*4
	if le.Uint32(b[idx+4:]) != 2 {
		t.Fatalf("indices out of place")
	}
	grp := idx + 3*4
	if le.Uint32(b[grp+8:]) != 1 {
		t.Fatalf("group material = %d", le.Uint32(b[grp+8:]))
	}
	mat := grp + 12
	if b[mat] != MatSolid || le.Uint16(b[mat+2:]) != 0 {
		t.Fatalf("untextured material = % x", b[mat:mat+4])
	}
	mat += 4
	if b[mat] != MatBlend || b[mat+1] != 128 || string(b[mat+4:mat+4+int(le.Uint16(b[mat+2:]))]) != "water.bmp" {
		t.Fatalf("water material = % x", b[mat:])
	}
	if mat+4+9 != len(b) {
		t.Fatalf("length %d, want %d", len(b), mat+13)
	}
}

func TestMaterialKinds(t *testing.T) {
	cases := []struct {
		method  uint32
		visible bool
		kind    uint8
	}{
		{0x00, false, 0},
		{0x80000053, false, 0},
		{0x8000000D, false, 0}, // sky dome
		{0x80000001, true, MatSolid},
		{0x80000013, true, MatMasked},
		{0x80000005, true, MatBlend},
		{0x80000017, true, MatAdditive},
	}
	for _, tc := range cases {
		p := make([]byte, 24)
		binary.LittleEndian.PutUint32(p[4:], tc.method)
		w := &WLD{Fragments: []fragment{{kind: fragMaterial, payload: p}}}
		m, ok := w.material(0)
		// Untextured diffuse materials aren't drawn either.
		if tc.kind == MatSolid && tc.visible {
			if ok {
				t.Errorf("0x%x: untextured solid material drawn", tc.method)
			}
			continue
		}
		if ok != tc.visible || (ok && m.Kind != tc.kind) {
			t.Errorf("0x%x: visible=%v kind=%d, want %v %d", tc.method, ok, m.Kind, tc.visible, tc.kind)
		}
	}
}

func TestBitmapName(t *testing.T) {
	name := "Brick.BMP\x00"
	b := make([]byte, 6+len(name))
	binary.LittleEndian.PutUint32(b, 1)
	binary.LittleEndian.PutUint16(b[4:], uint16(len(name)))
	for i := range name {
		b[6+i] = name[i] ^ stringHashKey[i%8]
	}
	if got := bitmapName(b); got != "brick.bmp" {
		t.Fatalf("bitmapName = %q", got)
	}
}
