package mapgen

import (
	"encoding/binary"
	"math"
	"testing"
)

// trackWLD builds a WLD holding one int16 track: fragment 1 is the 0x12
// frame data, fragment 2 the 0x13 reference to it.
func trackWLD(frame [8]int16) *WLD {
	le := binary.LittleEndian
	def := make([]byte, 8+16)
	le.PutUint32(def, 8)
	le.PutUint32(def[4:], 1)
	for i, v := range frame {
		le.PutUint16(def[8+2*i:], uint16(v))
	}
	ref := make([]byte, 8)
	le.PutUint32(ref, 1)
	return &WLD{Fragments: []fragment{{kind: fragTrackDef, payload: def}, {kind: fragTrack, payload: ref}}}
}

func TestTrackPose(t *testing.T) {
	// A quarter turn about z (w = x... cos/sin 45 degrees), moved (1, 2, 3)
	// at scale 2.
	q := int16(math.Round(16384 * math.Sqrt2 / 2))
	w := trackWLD([8]int16{q, 0, 0, q, 256, 512, 768, 512})
	m := w.trackPose(2)
	x, y, z := m.apply(1, 0, 0)
	if math.Abs(x-1) > 1e-3 || math.Abs(y-4) > 1e-3 || math.Abs(z-3) > 1e-3 {
		t.Fatalf("pose of (1,0,0) = (%v, %v, %v), want (1, 4, 3)", x, y, z)
	}
	if got := w.trackPose(99); got != identity {
		t.Fatalf("bad reference gave %v, want identity", got)
	}
}

func TestParseSkeleton(t *testing.T) {
	// flags bit 1 (bounding radius) and bit 9 (meshes); two bones, the
	// root with one child and a mesh, then one skeleton mesh.
	le := binary.LittleEndian
	var b []byte
	put := func(v uint32) { b = le.AppendUint32(b, v) }
	put(2 | 1<<9)
	put(2)
	put(0)
	put(0) // radius
	put(0)
	put(0)
	put(5)
	put(7)
	put(1)
	put(1) // root: track 5, mesh 7, child 1
	put(0)
	put(0)
	put(6)
	put(0)
	put(0) // bone 1: track 6
	put(1)
	put(9)
	put(0) // one mesh, 9
	bones, meshes := parseSkeleton(b)
	if len(bones) != 2 || bones[0].track != 5 || bones[0].mesh != 7 || len(bones[0].children) != 1 ||
		bones[0].children[0] != 1 || bones[1].track != 6 {
		t.Fatalf("bones = %+v", bones)
	}
	if len(meshes) != 1 || meshes[0] != 9 {
		t.Fatalf("meshes = %v", meshes)
	}
}
