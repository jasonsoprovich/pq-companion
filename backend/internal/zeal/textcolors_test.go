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

func TestMacroColorPaletteDefaults(t *testing.T) {
	// No eqclient.ini at all: the full built-in 20-color palette is returned.
	colors, err := MacroColorPalette(t.TempDir())
	if err != nil {
		t.Fatalf("palette: %v", err)
	}
	if len(colors) != 20 {
		t.Fatalf("colors = %d, want 20 defaults", len(colors))
	}
	if colors[0].Index != 0 || colors[0].R != 240 || colors[0].G != 240 || colors[0].B != 240 {
		t.Errorf("index 0 = %+v, want default white", colors[0])
	}
	if colors[13].Index != 13 || colors[13].R != 240 || colors[13].G != 0 || colors[13].B != 0 {
		t.Errorf("index 13 = %+v, want default red", colors[13])
	}
}

func TestMacroColorPaletteOverrides(t *testing.T) {
	// eqclient.ini overrides a slot; remaining slots keep their defaults, and
	// slots past index 19 extend the palette.
	content := "[TextColors]\n" +
		"User_1_Red = 10\nUser_1_Green = 20\nUser_1_Blue = 30\n" +
		"User_25_Red = 1\nUser_25_Green = 2\nUser_25_Blue = 3\n"
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "eqclient.ini"), []byte(content), 0o644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	colors, err := MacroColorPalette(dir)
	if err != nil {
		t.Fatalf("palette: %v", err)
	}
	if len(colors) != 21 {
		t.Fatalf("colors = %d, want 20 defaults + 1 extension", len(colors))
	}
	if colors[0].R != 10 || colors[0].G != 20 || colors[0].B != 30 {
		t.Errorf("index 0 = %+v, want eqclient override", colors[0])
	}
	if colors[1].R != 240 || colors[1].G != 180 || colors[1].B != 0 {
		t.Errorf("index 1 = %+v, want default gold", colors[1])
	}
	if colors[20].Index != 24 || colors[20].R != 1 {
		t.Errorf("last = %+v, want User_25 as index 24", colors[20])
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
