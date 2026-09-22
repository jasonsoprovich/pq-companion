package era

import "testing"

func TestPoPMaxLevel(t *testing.T) {
	if PoPMaxLevel != 65 {
		t.Errorf("PoPMaxLevel = %d, want 65", PoPMaxLevel)
	}
}
