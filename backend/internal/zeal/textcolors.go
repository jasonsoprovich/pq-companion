package zeal

// MacroColor is one entry of the fixed built-in social-macro color palette, used
// to render the swatch for a social macro's color index. Index is the value
// stored in the macro's Color key (0-based); R/G/B are the 0..255 components of
// that palette slot.
type MacroColor struct {
	Index int `json:"index"`
	R     int `json:"r"`
	G     int `json:"g"`
	B     int `json:"b"`
}

// defaultMacroColors is the client's built-in 20-entry social color palette
// (indices 0–19) — the fixed set of "A" swatches offered in the in-game social
// color picker. It is NOT the user-editable chat palette (eqclient.ini
// [TextColors] User_N): a macro's Color index references this fixed table, so
// swatches must come straight from it. Values match the classic client defaults
// (same table SockDrawer ships:
// https://github.com/yeroca/SockDrawer src/utils/colors.tsx) and are confirmed
// against real macros: 0=white, 5=pink/magenta, 13=red, 14=green, 15=yellow,
// 18=aqua.
var defaultMacroColors = [20][3]int{
	{240, 240, 240}, // 0 white
	{240, 180, 0},   // 1 gold
	{0, 128, 0},     // 2 green
	{180, 100, 0},   // 3 brown
	{0, 0, 128},     // 4 navy
	{240, 0, 240},   // 5 magenta
	{128, 128, 128}, // 6 gray
	{200, 200, 200}, // 7 light gray
	{130, 130, 0},   // 8 olive
	{96, 96, 96},    // 9 dark gray
	{0, 0, 0},       // 10 black
	{140, 0, 0},     // 11 dark red
	{160, 160, 160}, // 12 silver
	{240, 0, 0},     // 13 red
	{0, 240, 0},     // 14 bright green
	{240, 240, 0},   // 15 yellow
	{0, 0, 240},     // 16 blue
	{0, 100, 240},   // 17 sky blue
	{0, 240, 240},   // 18 cyan
	{128, 0, 128},   // 19 purple
}

// MacroColorPalette returns the fixed built-in social color palette (indices
// 0–19) used for swatch rendering. The eqPath argument is unused — the palette
// is hard-coded in the client and does NOT vary per install — but is kept so the
// call site stays symmetric with the other per-eq-path zeal scanners.
func MacroColorPalette(_ string) ([]MacroColor, error) {
	out := make([]MacroColor, len(defaultMacroColors))
	for i, c := range defaultMacroColors {
		out[i] = MacroColor{Index: i, R: c[0], G: c[1], B: c[2]}
	}
	return out, nil
}
