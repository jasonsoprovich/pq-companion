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

func TestDoorPlacement(t *testing.T) {
	// Game (x, y) is raw (y, x); size is a percentage.
	p := Door{Model: "DOOR1", X: 10, Y: 20, Z: 3, Heading: 128, Incline: 0, Size: 50}.placement()
	if p.Raw != (Vec3{20, 10, 3}) || p.Actor != "DOOR1_ACTORDEF" || p.Scale != 0.5 || p.Yaw != 128 {
		t.Fatalf("placement = %+v", p)
	}
	// Map space matches the door pins (x, y negated).
	if p.Pos.X != -10 || p.Pos.Y != -20 {
		t.Fatalf("map position = %v, want (-10, -20)", p.Pos)
	}
	if (Door{Size: 0}).placement().Scale != 1 {
		t.Fatal("size 0 should draw at the model's own size")
	}
}
