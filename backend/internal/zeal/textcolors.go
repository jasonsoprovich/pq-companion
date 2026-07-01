package zeal

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// MacroColor is one resolved entry of the EQ user-color palette, used to render
// a best-effort swatch for a social macro's color index. Index is the value
// stored in the macro's Color key (0-based); R/G/B are the 0..255 components the
// player has assigned to that slot in eqclient.ini.
type MacroColor struct {
	Index int `json:"index"`
	R     int `json:"r"`
	G     int `json:"g"`
	B     int `json:"b"`
}

// EQClientIniPath returns the path to the shared EQ client config that holds the
// user color palette: <eq_path>/eqclient.ini
func EQClientIniPath(eqPath string) string {
	return filepath.Join(eqPath, "eqclient.ini")
}

// textColorKeyRe matches a [TextColors] component key: User_<N>_<Red|Green|Blue>.
var textColorKeyRe = regexp.MustCompile(`(?i)^User_(\d+)_(Red|Green|Blue)$`)

// defaultMacroColors is the client's built-in 20-entry social color palette
// (indices 0–19), used for any slot eqclient.ini doesn't override. Values match
// the classic client defaults (same table SockDrawer ships:
// https://github.com/yeroca/SockDrawer src/utils/colors.tsx).
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

// MacroColorPalette returns the social color palette for swatch rendering:
// the built-in defaults for indices 0–19, overridden per-slot by any User_N
// entries in eqclient.ini [TextColors] (which may also extend past index 19).
func MacroColorPalette(eqPath string) ([]MacroColor, error) {
	parsed, err := ParseTextColors(eqPath)
	if err != nil {
		return nil, err
	}
	byIndex := map[int]MacroColor{}
	for i, c := range defaultMacroColors {
		byIndex[i] = MacroColor{Index: i, R: c[0], G: c[1], B: c[2]}
	}
	for _, c := range parsed {
		byIndex[c.Index] = c
	}
	out := make([]MacroColor, 0, len(byIndex))
	for _, c := range byIndex {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Index < out[j].Index })
	return out, nil
}

// ParseTextColors reads the [TextColors] section of eqclient.ini and returns the
// user color palette. The palette is what social-macro Color indices reference:
// the client stores colors as User_<N>_Red/Green/Blue (N is 1-based), and a
// macro's 0-based Color index maps to slot N = index+1. The mapping is a
// best-effort convention (the value points at a user-editable slot), so callers
// should treat the swatch as approximate and still surface the raw index.
//
// Returns an empty slice (no error) when eqclient.ini or the section is absent.
func ParseTextColors(eqPath string) ([]MacroColor, error) {
	path := EQClientIniPath(eqPath)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []MacroColor{}, nil
		}
		return nil, err
	}
	defer f.Close()

	type rgb struct{ r, g, b int }
	bySlot := map[int]*rgb{}

	inSection := false
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inSection = strings.EqualFold(strings.TrimSpace(line[1:len(line)-1]), "TextColors")
			continue
		}
		if !inSection {
			continue
		}
		eq := strings.IndexByte(line, '=')
		if eq < 0 {
			continue
		}
		key := strings.TrimSpace(line[:eq])
		m := textColorKeyRe.FindStringSubmatch(key)
		if m == nil {
			continue
		}
		slot, _ := strconv.Atoi(m[1])
		val, err := strconv.Atoi(strings.TrimSpace(line[eq+1:]))
		if err != nil {
			continue
		}
		if val < 0 {
			val = 0
		} else if val > 255 {
			val = 255
		}
		c := bySlot[slot]
		if c == nil {
			c = &rgb{}
			bySlot[slot] = c
		}
		switch strings.ToLower(m[2]) {
		case "red":
			c.r = val
		case "green":
			c.g = val
		case "blue":
			c.b = val
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("parse text colors: %w", err)
	}

	out := make([]MacroColor, 0, len(bySlot))
	for slot, c := range bySlot {
		// Macro Color indices are 0-based; eqclient slots are 1-based.
		out = append(out, MacroColor{Index: slot - 1, R: c.r, G: c.g, B: c.b})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Index < out[j].Index })
	return out, nil
}
