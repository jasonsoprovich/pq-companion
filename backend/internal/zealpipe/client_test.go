package zealpipe

import "testing"

func TestParsePID(t *testing.T) {
	tests := map[string]uint32{
		`\\.\pipe\zeal_12345`: 12345,
		`zeal_67890`:          67890,
		`zeal_`:               0,
		`unrelated`:           0,
		`zeal_abc`:            0,
		`\\.\pipe\zeal_1`:     1,
	}
	for in, want := range tests {
		if got := parsePID(in); got != want {
			t.Errorf("parsePID(%q) = %d, want %d", in, got, want)
		}
	}
}
