package era

import "testing"

func TestMaxLevel(t *testing.T) {
	if got := MaxLevel(false); got != 60 {
		t.Errorf("MaxLevel(false) = %d, want 60", got)
	}
	if got := MaxLevel(true); got != 65 {
		t.Errorf("MaxLevel(true) = %d, want 65", got)
	}
}
