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

func TestLinkHub(t *testing.T) {
	// A line a—b—c—d (d is three hops from a), plus teleport dests x and y.
	base := map[string][]string{
		"a": {"b"}, "b": {"a", "c"}, "c": {"b", "d"}, "d": {"c"},
	}
	adj := LinkHub(base, "a", []string{"x", "y", "a" /* self, ignored */})

	// Hub gains both dests; dests point back to the hub (one hop).
	if d := Distances("a", adj); d["x"] != 1 || d["y"] != 1 {
		t.Errorf("dests should be one hop from hub: x=%d y=%d", d["x"], d["y"])
	}
	// Dest-to-dest is two hops (via the hub), not one — it's a star, not a mesh.
	if d := Distances("x", adj); d["y"] != 2 {
		t.Errorf("dest-to-dest should be 2 via hub, got %d", d["y"])
	}
	// Original edges are untouched.
	if d := Distances("a", adj); d["d"] != 3 {
		t.Errorf("original path a..d should be 3 hops, got %d", d["d"])
	}
	// Input map isn't mutated.
	if len(base["a"]) != 1 {
		t.Errorf("LinkHub mutated its input: base[a]=%v", base["a"])
	}
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
