package mapfiles

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// writePack builds a map-pack directory with enough zones to be detected.
func writePack(t *testing.T, rel string, extra map[string]string) string {
	t.Helper()
	eq := t.TempDir()
	dir := filepath.Join(eq, rel)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < packMinZones; i++ {
		name := filepath.Join(dir, fmt.Sprintf("zone%02d.txt", i))
		if err := os.WriteFile(name, []byte("L 1, 2, 3, 4, 5, 6, 0, 0, 0\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for name, body := range extra {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return eq
}

func TestDetectFindsBrewallFolder(t *testing.T) {
	eq := writePack(t, filepath.Join("maps", "Brewall"), nil)
	p := Detect(eq)
	if p == nil {
		t.Fatal("Detect returned nil for a populated maps/Brewall")
	}
	// The folder name is the only self-description these packs carry, so it is
	// what the UI credits.
	if p.Name != "Brewall" {
		t.Errorf("Name = %q, want Brewall", p.Name)
	}
}

func TestDetectIgnoresOurOwnExportDir(t *testing.T) {
	// map_files is where this app writes its marker exports. A handful of files
	// there must not be mistaken for an installed pack, or the UI would offer a
	// render mode backed by our own two-line marker files.
	eq := t.TempDir()
	dir := filepath.Join(eq, "map_files")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"akheva.txt", "oasis.txt", "nexus.txt"} {
		if err := os.WriteFile(filepath.Join(dir, n),
			[]byte("P 1, 2, 3, 0, 0, 0, 2, trap\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if p := Detect(eq); p != nil {
		t.Errorf("Detect found a pack in our own export dir: %+v", p)
	}
}

func TestDetectNilWithoutEQPath(t *testing.T) {
	if p := Detect(""); p != nil {
		t.Errorf("Detect(\"\") = %+v, want nil", p)
	}
}

func TestLoadMergesLayersAndKeepsColour(t *testing.T) {
	eq := writePack(t, filepath.Join("maps", "Brewall"), map[string]string{
		// Base geometry plus a blue line — Brewall draws water in blue, which is
		// the whole reason this mode shows things our own extraction cannot.
		"oasis.txt": "L -1041.1, -246.7, 1.9, -1069.1, -267.8, 5.8, 0, 0, 0\n" +
			"L 10, 20, 1, 30, 40, 2, 0, 0, 255\n",
		"oasis_1.txt": "P 261.0000, 1321.0000, 5.0, 255, 255, 0, 2, Marnan_(Merchant)\n",
		"oasis_2.txt": "L 0, 2200, 0, 100, 2200, 0, 255, 0, 0\n",
	})
	p := Detect(eq)
	if p == nil {
		t.Fatal("no pack detected")
	}
	z, err := p.Load("oasis")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(z.Segments) != 3 {
		t.Fatalf("segments = %d, want 3 (all layers merged)", len(z.Segments))
	}
	if got := z.Segments[1]; got.B != 255 || got.R != 0 {
		t.Errorf("segment 1 colour = %v,%v,%v, want the file's blue", got.R, got.G, got.B)
	}
	// Coordinates pass through untouched: the file format's space is already
	// the renderer's map space.
	if got := z.Segments[0]; got.X1 != -1041 || got.Y1 != -246 {
		t.Errorf("segment 0 = (%d,%d), want the file's own coordinates", got.X1, got.Y1)
	}
	if len(z.Points) != 1 || z.Points[0].Label != "Marnan (Merchant)" {
		t.Errorf("points = %+v, want one label with underscores expanded", z.Points)
	}
}

func TestLoadSkipsMalformedLinesNotWholeFile(t *testing.T) {
	eq := writePack(t, "maps", map[string]string{
		"oasis.txt": "L 1, 2, 3, 4, 5, 6, 0, 0, 0\n" +
			"L broken line\n" +
			"L 1, 2, 3, 4, 5, 6, 999, 0, 0\n" + // colour out of range
			"# a comment the format does not actually allow\n" +
			"L 7, 8, 9, 10, 11, 12, 1, 2, 3\n",
	})
	z, err := Detect(eq).Load("oasis")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	// Two decades of hand-edited third-party files: one bad line costs that
	// line, never the zone.
	if len(z.Segments) != 2 {
		t.Errorf("segments = %d, want 2 good ones kept", len(z.Segments))
	}
}

func TestLoadUnknownZoneIsNotAnError(t *testing.T) {
	eq := writePack(t, "maps", nil)
	z, err := Detect(eq).Load("nosuchzone")
	if err != nil || z != nil {
		t.Errorf("Load(unknown) = %v, %v; want nil, nil", z, err)
	}
}

func TestCheckAlignmentMatchesTheSameZone(t *testing.T) {
	pack := &Zone{Points: []Point{
		{X: -235, Y: -6, Label: "Priest of Discord"},
		{X: 100, Y: 200, Label: "Guard Hargrin"},
		{X: -40, Y: 90, Label: "Banker Fusberg"},
	}}
	known := []Named{
		{Label: "Priest_of_Discord", X: -235, Y: -6},
		{Label: "Guard_Hargrin", X: 104, Y: 197},
		{Label: "Banker_Fusberg", X: -40, Y: 90},
	}
	a := CheckAlignment(pack, known)
	if a.Landmarks != 3 || a.Mismatch {
		t.Errorf("got %+v, want 3 landmarks and no mismatch", a)
	}
}

func TestCheckAlignmentCatchesADifferentZoneOfTheSameName(t *testing.T) {
	// The real case: a modern pack's Bazaar is the revamped one, so its zone
	// lines sit hundreds of units from where this server puts them. The lines
	// render perfectly and describe a different building, which is why this has
	// to be detected rather than left to look right.
	pack := &Zone{Points: []Point{
		{X: 384, Y: 104, Label: "to_Shadow_Haven"},
		{X: -260, Y: 115, Label: "to_The_Nexus"},
		{X: 435, Y: 0, Label: "to_The_Plane_of_Knowledge"},
	}}
	known := []Named{
		{Label: "to Shadow Haven", X: 215, Y: 875},
		{Label: "to The Nexus", X: -140, Y: 821},
		{Label: "to Shadow Haven", X: 135, Y: 875},
	}
	a := CheckAlignment(pack, known)
	if !a.Mismatch {
		t.Errorf("got %+v, want a mismatch", a)
	}
}

func TestCheckAlignmentDeclinesToJudgeWithoutEnoughLandmarks(t *testing.T) {
	// Unknown must never read as fine: two names can agree by chance, and a
	// wrong pass leaves the user trusting a wrong map.
	pack := &Zone{Points: []Point{{X: 0, Y: 0, Label: "Banker Smith"}}}
	known := []Named{{Label: "Banker_Smith", X: 0, Y: 0}}
	if a := CheckAlignment(pack, known); a.Landmarks != 0 || a.Mismatch {
		t.Errorf("got %+v, want a no-opinion result", a)
	}
}

func TestNormalizeLabelIgnoresSeparators(t *testing.T) {
	if normalizeLabel("to_Shadow_Haven") != normalizeLabel("to Shadow Haven") {
		t.Error("separator styles must match; the two sources punctuate differently")
	}
	if normalizeLabel("a_b") != "" {
		t.Error("short labels identify nothing and must not be matched on")
	}
}
