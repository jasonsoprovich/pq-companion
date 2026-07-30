package mapexport

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSanitizeLabel(t *testing.T) {
	cases := []struct{ in, want string }{
		// Spaces MUST become underscores: Zeal parses the label with %s, which
		// stops at whitespace, so a space would silently truncate the label to
		// its first word.
		{"Locked door", "Locked_door"},
		// Em dash is three bytes and renders as mojibake in the client font.
		{"Locked — needs Key", "Locked_-_needs_Key"},
		// Commas read as field separators to anything else parsing the line.
		{"Trap: A rock, Shadows", "Trap:_A_rock;_Shadows"},
		{"", "marker"},
	}
	for _, tc := range cases {
		if got := sanitizeLabel(tc.in); got != tc.want {
			t.Errorf("sanitizeLabel(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSanitizeLabelTruncatesToZealBuffer(t *testing.T) {
	got := sanitizeLabel(strings.Repeat("a", 200))
	if len(got) != labelMaxBytes {
		t.Errorf("len = %d, want %d (Zeal's char[64] minus the terminator)", len(got), labelMaxBytes)
	}
}

func TestFormatFileMatchesZealsScanf(t *testing.T) {
	out := FormatFile([]Point{{X: -100, Y: 200, Z: 12, Category: "trap", Label: "Trap: A rock"}})
	line := strings.TrimSpace(out)
	// Zeal: "P %f, %f, %f, %u, %u, %u, %i, %s"
	if !strings.HasPrefix(line, "P -100.0000, 200.0000, 12.0000, 245, 158, 11, 2, ") {
		t.Errorf("line = %q, does not match Zeal's P format", line)
	}
	if !strings.HasSuffix(line, "Trap:_A_rock") {
		t.Errorf("line = %q, label not underscored", line)
	}
}

func TestFormatFileEmptyForNoPoints(t *testing.T) {
	// An empty base file parses to nothing, and a base file that fails to load
	// disables external data for the zone — so callers must be able to skip.
	if got := FormatFile(nil); got != "" {
		t.Errorf("FormatFile(nil) = %q, want empty", got)
	}
}

// planFixture builds a map_files dir with the given files already present.
func planFixture(t *testing.T, files map[string]string) (mapDir, manifestPath string) {
	t.Helper()
	root := t.TempDir()
	mapDir = filepath.Join(root, MapFilesDir)
	if err := os.MkdirAll(mapDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(mapDir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return mapDir, filepath.Join(root, "manifest.json")
}

func TestPlanZoneCreatesBaseWhenAbsent(t *testing.T) {
	// The normal Quarm install: no map_files at all.
	mapDir, mp := planFixture(t, nil)
	m, _ := LoadManifest(mp)
	p := PlanZone(mapDir, "akheva", 5, m)
	if p.Action != "create" || p.File != "akheva.txt" {
		t.Errorf("got %+v, want create akheva.txt", p)
	}
}

func TestPlanZoneAppendsBesideForeignPack(t *testing.T) {
	// A Brewall-style pack: base plus two overlays, none of them ours.
	mapDir, mp := planFixture(t, map[string]string{
		"akheva.txt":   "L 0,0,0,1,1,1,255,255,255\n",
		"akheva_1.txt": "P 1,1,1,255,255,255,2,thing\n",
		"akheva_2.txt": "P 2,2,2,255,255,255,2,thing\n",
	})
	m, _ := LoadManifest(mp)
	p := PlanZone(mapDir, "akheva", 5, m)
	// _3 is the first free contiguous slot. Anything higher would never be read,
	// and the base or _1/_2 would destroy their data.
	if p.Action != "create" || p.File != "akheva_3.txt" {
		t.Errorf("got %+v, want create akheva_3.txt", p)
	}
}

func TestPlanZoneReplacesOnlyItsOwnFile(t *testing.T) {
	mapDir, mp := planFixture(t, nil)
	pts := []Point{{X: 1, Y: 1, Z: 0, Category: "trap", Label: "t"}}
	if _, err := Write(filepath.Dir(mapDir), mp, map[string][]Point{"akheva": pts}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	m, _ := LoadManifest(mp)
	if p := PlanZone(mapDir, "akheva", 1, m); p.Action != "replace" || p.File != "akheva.txt" {
		t.Fatalf("got %+v, want replace akheva.txt", p)
	}

	// Now something else takes the file over. We must not reclaim it.
	if err := os.WriteFile(filepath.Join(mapDir, "akheva.txt"),
		[]byte("L 0,0,0,1,1,1,1,1,1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if p := PlanZone(mapDir, "akheva", 1, m); p.File == "akheva.txt" {
		t.Errorf("got %+v — reclaimed a file whose contents changed under us", p)
	}
}

func TestPlanZoneSkipsWhenChainIsFull(t *testing.T) {
	files := map[string]string{"akheva.txt": "x"}
	for i := 1; i <= maxOptionalSlot; i++ {
		files[fmt.Sprintf("akheva_%d.txt", i)] = "x"
	}
	mapDir, mp := planFixture(t, files)
	m, _ := LoadManifest(mp)
	// Nowhere left to go, and silently doing nothing would be worse than saying so.
	if p := PlanZone(mapDir, "akheva", 1, m); p.Action != "skip" {
		t.Errorf("got %+v, want skip", p)
	}
}

func TestWriteThenRemoveRoundTrip(t *testing.T) {
	mapDir, mp := planFixture(t, nil)
	eq := filepath.Dir(mapDir)
	res, err := Write(eq, mp, map[string][]Point{
		"akheva":     {{X: 1, Y: 1, Z: 0, Category: "trap", Label: "a"}},
		"necropolis": {{X: 2, Y: 2, Z: 0, Category: "locked", Label: "b"}},
	})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if res.Written != 2 || res.Points != 2 {
		t.Errorf("res = %+v, want 2 written / 2 points", res)
	}
	for _, f := range []string{"akheva.txt", "necropolis.txt"} {
		if _, err := os.Stat(filepath.Join(mapDir, f)); err != nil {
			t.Errorf("%s not written: %v", f, err)
		}
	}

	removed, kept, err := Remove(eq, mp)
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if removed != 2 || kept != 0 {
		t.Errorf("removed=%d kept=%d, want 2/0", removed, kept)
	}
	if _, err := os.Stat(filepath.Join(mapDir, "akheva.txt")); !os.IsNotExist(err) {
		t.Error("akheva.txt still present after Remove")
	}
}

func TestRemoveLeavesForeignFilesAlone(t *testing.T) {
	mapDir, mp := planFixture(t, nil)
	eq := filepath.Dir(mapDir)
	if _, err := Write(eq, mp, map[string][]Point{
		"akheva": {{X: 1, Y: 1, Z: 0, Category: "trap", Label: "a"}},
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	// Something replaces our file — a map pack installed afterwards, say.
	foreign := []byte("L 0,0,0,1,1,1,1,1,1\n")
	if err := os.WriteFile(filepath.Join(mapDir, "akheva.txt"), foreign, 0o644); err != nil {
		t.Fatal(err)
	}

	removed, kept, err := Remove(eq, mp)
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if removed != 0 || kept != 1 {
		t.Errorf("removed=%d kept=%d, want 0/1", removed, kept)
	}
	got, err := os.ReadFile(filepath.Join(mapDir, "akheva.txt"))
	if err != nil || string(got) != string(foreign) {
		t.Error("Remove destroyed a file it did not write — the one thing it must never do")
	}
}

func TestCorruptManifestDoesNotAuthoriseDeletion(t *testing.T) {
	mapDir, mp := planFixture(t, map[string]string{"akheva.txt": "someone elses data"})
	if err := os.WriteFile(mp, []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	removed, _, err := Remove(filepath.Dir(mapDir), mp)
	if err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if removed != 0 {
		t.Errorf("removed %d files from a corrupt manifest, want 0", removed)
	}
	if _, err := os.Stat(filepath.Join(mapDir, "akheva.txt")); err != nil {
		t.Error("a corrupt manifest led to deleting a file we never wrote")
	}
}
