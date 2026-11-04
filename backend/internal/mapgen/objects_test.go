package mapgen

import (
	"encoding/binary"
	"math"
	"testing"
)

func TestPlaceTransform(t *testing.T) {
	// A model point one unit along raw +x (map (0, -1, 0)), placed at raw
	// (100, 200, 10) with a quarter-turn yaw and scale 2, lands two units
	// along raw +y from the placement.
	p := Placement{Raw: Vec3{100, 200, 10}, Yaw: 128, Scale: 2}
	got := placeTransform(p)(ToMapSpace(1, 0, 0))
	want := ToMapSpace(100, 202, 10)
	if math.Abs(got.X-want.X) > 1e-9 || math.Abs(got.Y-want.Y) > 1e-9 || math.Abs(got.Z-want.Z) > 1e-9 {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestActorComponents(t *testing.T) {
	// flags with both parameter blocks, one entry of two pairs, one
	// component.
	le := binary.LittleEndian
	b := make([]byte, 20+4+28+4+16+4+4)
	le.PutUint32(b, 3)
	le.PutUint32(b[8:], 1)
	le.PutUint32(b[12:], 1)
	le.PutUint32(b[52:], 2)
	le.PutUint32(b[72:], 42)
	if got := actorComponents(b); len(got) != 1 || got[0] != 42 {
		t.Fatalf("components = %v", got)
	}
}
