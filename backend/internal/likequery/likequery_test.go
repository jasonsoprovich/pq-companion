package likequery

import "testing"

func TestContains(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Mold", `%Mold%`},
		{"Elemental * Mold", `%Elemental%Mold%`},
		{"Elemental*Mold", `%Elemental%Mold%`},
		{"Fl?me", `%Fl_me%`},
		{"100%", `%100\%%`}, // literal % stays literal
		{"a_b", `%a\_b%`},   // literal _ stays literal
		{`back\slash`, `%back\\slash%`},
		{"Backpack*", `%Backpack%%`},
	}
	for _, tt := range tests {
		if got := Contains(tt.in); got != tt.want {
			t.Errorf("Contains(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestMatcher(t *testing.T) {
	tests := []struct {
		q, s string
		want bool
	}{
		{"mold", "Elemental Helm Mold", true},
		{"Elemental * Mold", "Elemental Helm Mold", true},
		{"elemental*mold", "ELEMENTAL BOOT MOLD", true},
		{"Spirit * Wolf", "Illusion: Spirit Wolf", true},
		{"Hel? Mold", "Elemental Helm Mold", true},
		{"Hel? Mold", "Elemental Hel Mold", false},
		{"a.b", "axb", false}, // regex metacharacters are literal
		{"(x)", "a (x) b", true},
		{"Elemental * Mold", "Elemental Helm", false},
	}
	for _, tt := range tests {
		if got := Matcher(tt.q)(tt.s); got != tt.want {
			t.Errorf("Matcher(%q)(%q) = %v, want %v", tt.q, tt.s, got, tt.want)
		}
	}
}
