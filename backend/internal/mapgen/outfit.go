package mapgen

import (
	"fmt"
	"path"
	"regexp"
	"strconv"
	"strings"
)

// Dressing a player model: what an item looks like on a character is its
// material (items.material), which picks the body part textures, plus its
// tint (items.color), and for weapons and shields its model (items.idfile,
// an IT### actor in the gequip archives). Body part textures are named
// {code}{part}{material:02}{variant:02}: humch0301.bmp is a human male's
// plate chest. The original models keep materials 0-4 and the robes in
// global_chr.s3d and the Velious sets (17-23) in global<n>_amr.s3d; the
// Luclin models keep theirs in their own archive, with "sk" for bare skin
// in place of a material number (humchsk01.dds).

// Material slots in the order the client keeps them (and Zeal sends them).
const (
	SlotHead = iota
	SlotChest
	SlotArms
	SlotWrist
	SlotHands
	SlotLegs
	SlotFeet
	MaterialSlots
)

// slotParts maps a body part texture code to its material slot.
var slotParts = map[string]int{
	"he": SlotHead, "ch": SlotChest, "ua": SlotArms, "fa": SlotWrist,
	"hn": SlotHands, "lg": SlotLegs, "ft": SlotFeet,
}

// partTextureRe reads a body part texture name: model code, part,
// material (or "sk"), variant and extension.
var partTextureRe = regexp.MustCompile(`^([a-z]{3})(ch|fa|ft|hn|lg|ua|he)([0-9]{2}|sk)([0-9]{2})\.(bmp|dds)$`)

// robeTextureRe reads an original model's robe texture: clk{robe}{variant}.
var robeTextureRe = regexp.MustCompile(`^clk([0-9]{2})([0-9]{2})\.(bmp|dds)$`)

// IsRobe reports whether a chest material is a robe (10-16), which the
// original models draw with a robed body.
func IsRobe(material int) bool { return material >= 10 && material <= 16 }

// OriginalHelm picks the original models' helm mesh for a head material:
// leather, chain and plate have their own, and other materials show as
// one of them by the client's remaps (materialCandidates); anything else
// is bare-headed.
func OriginalHelm(material int, code string, who Wearer) int {
	mats, _ := materialCandidates(material, strings.ToLower(code), "he", who)
	for _, m := range mats {
		if m >= 1 && m <= 3 {
			return m
		}
	}
	return 0
}

// TextureSwap replaces one of a model's textures for its outfit.
type TextureSwap struct {
	From    string `json:"from"`
	Archive string `json:"archive"`
	Name    string `json:"name"`
	Slot    int    `json:"slot"`
	// Fallback is set when the client has no texture for the material on
	// this model and we pick a near one (the client's own remaps, such as
	// Velious sets to plate, aren't fallbacks).
	Fallback bool `json:"fallback,omitempty"`
}

// OutfitResult is how a model's textures change for an outfit. Missing
// lists the slots whose material this model has no texture for; they stay
// as they were.
type OutfitResult struct {
	Swaps   []TextureSwap `json:"swaps"`
	Missing []int         `json:"missing"`
}

// Wearer is who a model is dressed for, as far as the client's material
// rules care.
type Wearer struct {
	// Luclin is set for a Luclin-era model.
	Luclin bool
	// Monk: only monks show material 4 (the monk set); others get leather.
	Monk bool
}

// materialCandidates is the materials the client tries for one, in order
// (eqgame.exe's armor material setter, 0x4a1eb7): 4 is monk-only and
// shows as leather on anyone else; 7 (scale) shows as chain, or plate on
// an Iksar; the Velious sets fall back to leather, chain and plate when
// their textures aren't there. guess marks a stand-in of ours, not the
// client's (none at present: the Luclin plate chest that has no texture is
// a worn model, see LuclinWear).
func materialCandidates(mat int, code, part string, who Wearer) (mats []int, guess []bool) {
	switch {
	case mat == 4 && !who.Monk:
		mat = 1
	case mat == 7:
		mat = 2
		if strings.HasPrefix(code, "ik") {
			mat = 3
		}
	}
	mats, guess = []int{mat}, []bool{false}
	if mat >= 17 && mat <= 23 {
		base := []int{1, 2, 3, 1, 2, 3, 1}[mat-17]
		if mat == 23 && who.Monk {
			base = 4
		}
		mats, guess = append(mats, base), append(guess, false)
	}
	return mats, guess
}

// DressTextures works out the texture swaps that dress a model, given the
// textures it was loaded with (from its archive), the material in each
// slot, and has, which reports whether an archive holds a file. A player
// model's head isn't retextured (its textures are faces, and helms are
// meshes); any other model's head takes its material like the rest (a
// Greater Wolf's head is wolhe03).
func DressTextures(archive string, who Wearer, textures []string, mats [MaterialSlots]int, has func(archive, name string) bool) OutfitResult {
	var res OutfitResult
	missing := map[int]bool{}
	done := map[string]bool{}
	for _, tex := range textures {
		lower := strings.ToLower(tex)
		if done[lower] {
			continue
		}
		done[lower] = true

		if m := robeTextureRe.FindStringSubmatch(lower); m != nil {
			// The robed body's own textures, for robes 10-16 (clk04-clk10).
			mat := mats[SlotChest]
			if !IsRobe(mat) {
				continue
			}
			name := fmt.Sprintf("clk%02d%s.%s", mat-6, m[2], m[3])
			if name != lower && has(archive, name) {
				res.Swaps = append(res.Swaps, TextureSwap{From: tex, Archive: archive, Name: name, Slot: SlotChest})
			}
			continue
		}

		m := partTextureRe.FindStringSubmatch(lower)
		if m == nil {
			continue
		}
		code, part, variant, ext := m[1], m[2], m[4], m[5]
		slot := slotParts[part]
		if slot == SlotHead && isPlayerModel(code) {
			continue
		}
		mat := mats[slot]
		if mat <= 0 {
			continue // as loaded: bare skin, or the default clothes
		}
		cur := m[3]
		if cur != "sk" {
			if n, _ := strconv.Atoi(cur); n == mat {
				continue
			}
		}
		candidates, guess := materialCandidates(mat, code, part, who)
		found := false
		for i, c := range candidates {
			if cur != "sk" {
				if n, _ := strconv.Atoi(cur); n == c {
					found = true // already wearing what it shows as
					break
				}
			}
			if a, name, ok := findPartTexture(archive, code, part, c, variant, ext, has); ok {
				res.Swaps = append(res.Swaps, TextureSwap{From: tex, Archive: a, Name: name, Slot: slot, Fallback: guess[i]})
				found = true
				break
			}
		}
		if !found && !missing[slot] {
			missing[slot] = true
			res.Missing = append(res.Missing, slot)
		}
	}
	return res
}

// isPlayerModel reports whether a model code ("hum", "elf") is a player
// race's: its race letters, then m or f ("ele", the elemental, isn't).
func isPlayerModel(code string) bool {
	return len(code) == 3 && modelRace(code) > 0 && (code[2] == 'm' || code[2] == 'f' || code[2] == 'M' || code[2] == 'F')
}

// findPartTexture looks for one body part texture: in the model's archive,
// then for the Velious sets in their own armor archive. Either extension
// is accepted.
func findPartTexture(archive, code, part string, mat int, variant, ext string, has func(string, string) bool) (string, string, bool) {
	archives := []string{archive}
	if mat >= 17 && mat <= 23 {
		archives = append(archives, fmt.Sprintf("global%d_amr.s3d", mat))
	}
	exts := []string{ext, "bmp", "dds"}
	for _, a := range archives {
		for _, e := range exts {
			name := fmt.Sprintf("%s%s%02d%s.%s", code, part, mat, variant, e)
			if has(a, name) {
				return a, name, true
			}
		}
	}
	return "", "", false
}

// ItemModelCode turns an item's idfile ("IT63") into its actor code, or ""
// for items with no model of their own (IT0, blank).
func ItemModelCode(idfile string) string {
	s := strings.ToUpper(strings.TrimSpace(path.Base(idfile)))
	if !strings.HasPrefix(s, "IT") {
		return ""
	}
	n, err := strconv.Atoi(s[2:])
	if err != nil || n <= 0 {
		return ""
	}
	return s
}
