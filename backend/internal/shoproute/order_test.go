package shoproute

import (
	"reflect"
	"testing"
)

func TestAlignment(t *testing.T) {
	cases := map[string]string{
		"qeynos":      AlignmentGood,
		"felwithea":   AlignmentGood,
		"neriakb":     AlignmentEvil,
		"grobb":       AlignmentEvil,
		"poknowledge": AlignmentNeutral,
		"freportw":    AlignmentNeutral, // Freeport is deliberately neutral
		"nektulos":    AlignmentNeutral, // wilderness
		"":            AlignmentNeutral,
	}
	for zone, want := range cases {
		if got := Alignment(zone); got != want {
			t.Errorf("Alignment(%q) = %q, want %q", zone, got, want)
		}
	}
}

func zones(z ...string) []Stop {
	out := make([]Stop, len(z))
	for i, name := range z {
		out[i] = Stop{Zone: name}
	}
	return out
}

func names(stops []Stop) []string {
	out := make([]string, len(stops))
	for i, s := range stops {
		out[i] = s.Zone
	}
	return out
}

func TestOrder(t *testing.T) {
	// A linear chain: start - a - b - c - d
	adj := map[string][]string{
		"start": {"a"},
		"a":     {"start", "b"},
		"b":     {"a", "c"},
		"c":     {"b", "d"},
		"d":     {"c"},
	}

	tests := []struct {
		name  string
		stops []Stop
		start string
		want  []string
	}{
		{
			name:  "empty start returns input unchanged",
			stops: zones("d", "b", "a"),
			start: "",
			want:  []string{"d", "b", "a"},
		},
		{
			name:  "single stop returned as-is",
			stops: zones("d"),
			start: "start",
			want:  []string{"d"},
		},
		{
			name:  "orders nearest-first along the chain",
			stops: zones("d", "a", "c", "b"),
			start: "start",
			want:  []string{"a", "b", "c", "d"},
		},
		{
			name:  "start adjacent to far end reverses order",
			stops: zones("a", "b", "c", "d"),
			start: "d",
			want:  []string{"d", "c", "b", "a"},
		},
		{
			name:  "unreachable zones appended last, deterministically",
			stops: zones("b", "zzz", "a", "aaa"),
			start: "start",
			// a,b reachable (dist 1,2); aaa,zzz unreachable -> lexicographic.
			want: []string{"a", "b", "aaa", "zzz"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := names(Order(tt.stops, tt.start, adj))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Order = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestOrderHubShortcut(t *testing.T) {
	// A star graph: hub connects to every leaf, leaves only connect via hub.
	adj := map[string][]string{
		"hub": {"x", "y", "z"},
		"x":   {"hub"},
		"y":   {"hub"},
		"z":   {"hub"},
	}
	// From the hub every leaf is one hop; tie-break makes the order lexicographic
	// and stable.
	got := names(Order(zones("z", "x", "y"), "hub", adj))
	want := []string{"x", "y", "z"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Order via hub = %v, want %v", got, want)
	}
}
