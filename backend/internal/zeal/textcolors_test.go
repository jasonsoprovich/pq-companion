package zeal

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseTextColors(t *testing.T) {
	// Mixed spacing around '=' as seen in real eqclient.ini files, plus a
	// non-TextColors section that must be ignored.
	content := "[Defaults]\nFoo=1\n[TextColors]\n" +
		"User_1_Red = 0\nUser_1_Green = 0\nUser_1_Blue = 0\n" +
		"User_2_Red =173\nUser_2_Green =0\nUser_2_Blue =183\n" +
		"User_3_Red = 0\nUser_3_Green = 255\nUser_3_Blue = 255\n"
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "eqclient.ini"), []byte(content), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	colors, err := ParseTextColors(dir)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(colors) != 3 {
		t.Fatalf("colors = %d, want 3", len(colors))
	}
	// User_1 -> macro index 0.
	if colors[0].Index != 0 || colors[0].R != 0 || colors[0].G != 0 || colors[0].B != 0 {
		t.Errorf("index 0 = %+v, want black", colors[0])
	}
	// User_2 -> macro index 1 (magenta-ish).
	if colors[1].Index != 1 || colors[1].R != 173 || colors[1].G != 0 || colors[1].B != 183 {
		t.Errorf("index 1 = %+v", colors[1])
	}
	if colors[2].Index != 2 || colors[2].B != 255 {
		t.Errorf("index 2 = %+v", colors[2])
	}
}

func TestParseTextColorsMissingFile(t *testing.T) {
	colors, err := ParseTextColors(t.TempDir())
	if err != nil {
		t.Fatalf("missing file should not error: %v", err)
	}
	if len(colors) != 0 {
		t.Errorf("expected empty palette, got %d", len(colors))
	}
}

func TestParseTextColorsFixture(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "testdata", "TAKPv22")
	if _, err := os.Stat(filepath.Join(dir, "eqclient.ini")); err != nil {
		t.Skipf("fixture not found: %v", err)
	}
	colors, err := ParseTextColors(dir)
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	if len(colors) == 0 {
		t.Fatal("expected a palette from the fixture")
	}
}
