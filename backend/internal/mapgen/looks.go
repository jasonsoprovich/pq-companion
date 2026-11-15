package mapgen

import (
	"fmt"
	"regexp"
	"strings"
)

// A character's looks: face, hairstyle and hair colour, beard and beard
// colour, eye colour. The game picks them at character creation and keeps
// them on the spawn, but logs none of them, so the user sets them by hand.
// The rules and tables are eqgame.exe's (TAKP):
//   - Face: head textures {code}hesk{face}{piece} (Luclin) or
//     {code}he00{face}{piece} (original), face 0 being the default set
//     ("%sHE%02d%d1_MDF", 0x4a0e67).
//   - Hair and beard: worn models (see LuclinWear), tinted from a 24-colour
//     palette (0x602850, applied by 0x4a0015) within a range per race
//     (0x520863).
//   - Eyes: chr_eye{base+colour} from lgequip.s3d (0x4a0792), the base
//     per race.

// Looks is how a character's face and hair look. Each field's zero value
// is the game's default, except as noted.
type Looks struct {
	// Face is 0-7 on the player models.
	Face int `json:"face"`
	// Hair is the Luclin hairstyle (0-2 on most models), or NoHair.
	Hair int `json:"hair"`
	// HairColor is a palette index, or -1 for the race's default.
	HairColor int `json:"hair_color"`
	// Beard is the Luclin beard style, or -1 for none.
	Beard int `json:"beard"`
	// BeardColor is a palette index, or -1 to follow the hair.
	BeardColor int `json:"beard_color"`
	// Eyes is the Luclin eye colour, 1-10, or 0 for the model's own.
	Eyes int `json:"eyes"`
}

// DefaultLooks is a character nobody has set looks for.
var DefaultLooks = Looks{HairColor: -1, Beard: -1, BeardColor: -1}

// hairPalette is the client's hair and beard colours (0x602850), 0xRRGGBB.
var hairPalette = [24]uint32{
	0x2e1a0c, 0x432916, 0x4e3123, 0x7f513b, 0x650b06, 0xb93714, 0xd75532, 0x8b721e,
	0xccb361, 0xe1dd6c, 0xfbff81, 0xfdfac9, 0xffffff, 0xdedede, 0x808080, 0x6f8690,
	0x3e585a, 0x293e40, 0x121214, 0xc9e5fd, 0xc9fdfd, 0xe9c9fd, 0xcefdc9, 0x559b48,
}

// modelRaces maps a player model code's race letters to the race.
var modelRaces = map[string]int{
	"HU": 1, "BA": 2, "ER": 3, "EL": 4, "HI": 5, "DA": 6, "HA": 7, "DW": 8,
	"TR": 9, "OG": 10, "HO": 11, "GN": 12, "IK": 128, "KE": 130,
}

func modelRace(code string) int {
	if len(code) < 2 {
		return 0
	}
	return modelRaces[strings.ToUpper(code[:2])]
}

// hairColorRange is the palette range a race may use (0x520863); ok is
// false for the races with no hair colour (Iksar, Vah Shir).
func hairColorRange(code string) (lo, hi int, ok bool) {
	switch modelRace(code) {
	case 0:
		return 0, 0, false
	case 5:
		return 4, 17, true
	case 6:
		return 11, 19, true
	case 9, 10:
		return 0, 23, true
	case 12:
		return 0, 22, true
	case 128, 130:
		return 0, 0, false
	}
	return 0, 18, true
}

// hairTint is the colour for palette index idx on a model (-1 for its
// default, the bottom of its range), or 0 for none.
func hairTint(code string, idx int) uint32 {
	lo, hi, ok := hairColorRange(code)
	if !ok {
		return 0
	}
	if idx < lo || idx > hi {
		idx = lo
	}
	return hairPalette[idx]
}

// eyeBase is where a race's eye colours start among the chr_eye textures.
func eyeBase(code string) int {
	switch modelRace(code) {
	case 6:
		return 20
	case 128:
		return 40
	case 130:
		return 60
	case 10:
		return 80
	case 9:
		return 100
	}
	return 0
}

// EyeArchive holds the Luclin eye colour textures.
const EyeArchive = "lgequip.s3d"

// EyeTexture is the texture for eye colour n (1-10) on a Luclin model.
func EyeTexture(code string, n int) string {
	return fmt.Sprintf("chr_eye%03d.dds", eyeBase(code)+n)
}

var (
	luclinFaceRe   = regexp.MustCompile(`^([a-z]{3})hesk0([0-9])\.(dds|bmp)$`)
	originalFaceRe = regexp.MustCompile(`^([a-z]{3})he000([0-9])\.(bmp|dds)$`)
	eyeTextureRe   = regexp.MustCompile(`^chr_eye[0-9]{3}\.dds$`)
)

// faceTexture is the texture for face on a model's default head texture,
// or "" when tex isn't one.
func faceTexture(tex string, face int, luclin bool) string {
	lower := strings.ToLower(tex)
	if luclin {
		if m := luclinFaceRe.FindStringSubmatch(lower); m != nil {
			return fmt.Sprintf("%shesk%d%s.%s", m[1], face, m[2], m[3])
		}
		return ""
	}
	if m := originalFaceRe.FindStringSubmatch(lower); m != nil {
		return fmt.Sprintf("%she00%d%s.%s", m[1], face, m[2], m[3])
	}
	return ""
}

// LookTextures is the texture swaps for a model's face and eye colour,
// given the textures it was loaded with from archive. A face or eye colour
// the client has no texture for leaves the model's own, as the game does.
// The swaps take no tint (Slot -1).
func LookTextures(archive, code string, luclin bool, textures []string, l Looks, has func(archive, name string) bool) []TextureSwap {
	var out []TextureSwap
	done := map[string]bool{}
	for _, tex := range textures {
		lower := strings.ToLower(tex)
		if done[lower] {
			continue
		}
		done[lower] = true
		if l.Face > 0 {
			if name := faceTexture(lower, l.Face, luclin); name != "" && has(archive, name) {
				out = append(out, TextureSwap{From: tex, Archive: archive, Name: name, Slot: -1})
				continue
			}
		}
		if luclin && l.Eyes >= 1 && l.Eyes <= 10 && eyeTextureRe.MatchString(lower) {
			if name := EyeTexture(code, l.Eyes); name != lower && has(EyeArchive, name) {
				out = append(out, TextureSwap{From: tex, Archive: EyeArchive, Name: name, Slot: -1})
			}
		}
	}
	return out
}

// PaletteColor is one choice of colour: its index and what it looks like.
type PaletteColor struct {
	Index int    `json:"index"`
	RGB   uint32 `json:"rgb"`
}

// LookOptions is what a model's looks can be in this client.
type LookOptions struct {
	// Faces is how many faces there are (0 to Faces-1).
	Faces int `json:"faces"`
	// Hair and Beards are the Luclin styles the client has models for.
	Hair   []int `json:"hair"`
	Beards []int `json:"beards"`
	// HairColors is the race's palette range; DefaultHairColor its
	// default. Empty for races without hair colour.
	HairColors       []PaletteColor `json:"hair_colors"`
	DefaultHairColor int            `json:"default_hair_color"`
	// Eyes is the Luclin eye colours there's a texture for (in
	// EyeArchive).
	Eyes []EyeColor `json:"eyes"`
}

// EyeColor is one choice of eye colour and its texture.
type EyeColor struct {
	Index   int    `json:"index"`
	Texture string `json:"texture"`
}

// AvailableLooks reports a model's choices. textures is what the model was
// loaded with from archive; has reports whether an archive holds a file,
// hasItem whether the client has a worn model.
func AvailableLooks(archive, code string, luclin bool, textures []string, has func(archive, name string) bool, hasItem func(item string) bool) LookOptions {
	code = strings.ToUpper(code)
	opts := LookOptions{Faces: 1, Hair: []int{}, Beards: []int{}, HairColors: []PaletteColor{}, Eyes: []EyeColor{}}
	// A face counts when its first piece is there.
	for _, tex := range textures {
		probe := faceTexture(tex, 1, luclin)
		if dot := strings.LastIndex(probe, "."); dot < 1 || probe[dot-1] != '1' {
			continue
		}
		for f := 1; f <= 9; f++ {
			if !has(archive, faceTexture(tex, f, luclin)) {
				break
			}
			opts.Faces = max(opts.Faces, f+1)
		}
	}
	if !luclin {
		return opts
	}
	if off, ok := luclinWearOffsets[code]; ok && !strings.HasPrefix(code, "KE") {
		for n := 0; n <= 9; n++ {
			if hasItem(fmt.Sprintf("IT%d", 1000+off+n)) {
				opts.Hair = append(opts.Hair, n)
			}
			if hasItem(fmt.Sprintf("IT%d", 2000+off+n)) {
				opts.Beards = append(opts.Beards, n)
			}
		}
	}
	if lo, hi, ok := hairColorRange(code); ok {
		for i := lo; i <= hi; i++ {
			opts.HairColors = append(opts.HairColors, PaletteColor{Index: i, RGB: hairPalette[i]})
		}
		opts.DefaultHairColor = lo
	}
	for n := 1; n <= 10; n++ {
		if tex := EyeTexture(code, n); has(EyeArchive, tex) {
			opts.Eyes = append(opts.Eyes, EyeColor{Index: n, Texture: tex})
		}
	}
	return opts
}
