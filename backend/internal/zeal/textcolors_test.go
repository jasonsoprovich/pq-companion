package zeal

import (
	"testing"
)

func TestMacroColorPaletteDefaults(t *testing.T) {
	// The full built-in 20-color palette is returned, independent of any
	// eqclient.ini (macro colors reference the fixed client palette, not the
	// user-editable chat palette).
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
	// Confirmed against real macros: 5=pink/magenta, 14=green, 15=yellow, 18=aqua.
	if colors[5].R != 240 || colors[5].G != 0 || colors[5].B != 240 {
		t.Errorf("index 5 = %+v, want magenta/pink", colors[5])
	}
	if colors[14].R != 0 || colors[14].G != 240 || colors[14].B != 0 {
		t.Errorf("index 14 = %+v, want green", colors[14])
	}
	if colors[15].R != 240 || colors[15].G != 240 || colors[15].B != 0 {
		t.Errorf("index 15 = %+v, want yellow", colors[15])
	}
	if colors[18].R != 0 || colors[18].G != 240 || colors[18].B != 240 {
		t.Errorf("index 18 = %+v, want aqua/cyan", colors[18])
	}
}

// The palette must NOT be swayed by eqclient.ini [TextColors] — that is the
// user chat palette, a different system. Even with an eqclient.ini present the
// fixed built-in table wins.
func TestMacroColorPaletteIgnoresEqclient(t *testing.T) {
	dir := t.TempDir()
	colors, err := MacroColorPalette(dir)
	if err != nil {
		t.Fatalf("palette: %v", err)
	}
	// index 0 stays white regardless of whatever eqclient.ini User_1 might hold.
	if colors[0].R != 240 || colors[0].G != 240 || colors[0].B != 240 {
		t.Errorf("index 0 = %+v, want fixed white (not eqclient override)", colors[0])
	}
}
