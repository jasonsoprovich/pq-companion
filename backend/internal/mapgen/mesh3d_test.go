package mapgen

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestMesh3D(t *testing.T) {
	z := &Zone{
		Vertices: []Vec3{{0, 0, 0}, {10, 0, 0}, {0, 10, 5}, {99, 99, 99}, {10, 10, 0}},
		Triangles: []Triangle{
			{A: 0, B: 1, C: 2},
			{Flags: polyPermeable, A: 1, B: 3, C: 2}, // water: dropped, and vertex 3 with it
			{A: 1, B: 4, C: 2},
		},
	}
	pos, idx := z.Mesh3D()
	if len(pos) != 4*3 {
		t.Fatalf("vertices = %d, want 4 (unused one dropped)", len(pos)/3)
	}
	want := []uint32{0, 1, 2, 1, 3, 2}
	for i := range want {
		if idx[i] != want[i] {
			t.Fatalf("indices = %v, want %v", idx, want)
		}
	}
	if pos[3*3] != 10 || pos[3*3+1] != 10 {
		t.Fatalf("remapped vertex 3 = %v", pos[9:12])
	}

	b := EncodeMesh3D(pos, idx)
	if string(b[:4]) != "PQM1" || binary.LittleEndian.Uint32(b[4:]) != 4 || binary.LittleEndian.Uint32(b[8:]) != 6 {
		t.Fatalf("header = %q %d %d", b[:4], binary.LittleEndian.Uint32(b[4:]), binary.LittleEndian.Uint32(b[8:]))
	}
	if len(b) != 12+4*12+4*6 {
		t.Fatalf("len = %d", len(b))
	}
	if f := math.Float32frombits(binary.LittleEndian.Uint32(b[12+4*8:])); f != 5 {
		t.Fatalf("vertex 2 z = %v, want 5", f)
	}
	if i := binary.LittleEndian.Uint32(b[12+4*12+4*4:]); i != 3 {
		t.Fatalf("index 4 = %d, want 3", i)
	}
}
