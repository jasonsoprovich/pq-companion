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
// leather, chain and plate have their own; anything else is bare-headed.
func OriginalHelm(material int) int {
	if material >= 1 && material <= 3 {
		return material
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
	// this model and a near one stands in.
	Fallback bool `json:"fallback,omitempty"`
}

// OutfitResult is how a model's textures change for an outfit. Missing
// lists the slots whose material this model has no texture for; they stay
// as they were.
type OutfitResult struct {
	Swaps   []TextureSwap `json:"swaps"`
	Missing []int         `json:"missing"`
}

// DressTextures works out the texture swaps that dress a model, given the
// textures it was loaded with (from its archive), the material in each
// slot, and has, which reports whether an archive holds a file. Heads
// aren't retextured: their textures are faces, and helms are meshes.
func DressTextures(archive string, luclin bool, textures []string, mats [MaterialSlots]int, has func(archive, name string) bool) OutfitResult {
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
		if slot == SlotHead {
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
		candidates := []int{mat}
		if luclin && slot == SlotChest && mat == 3 {
			// The Luclin models in the TAKP client have plate for every
			// part but the chest; chain is the nearest look.
			candidates = append(candidates, 2)
		}
		found := false
		for i, c := range candidates {
			if a, name, ok := findPartTexture(archive, code, part, c, variant, ext, has); ok {
				res.Swaps = append(res.Swaps, TextureSwap{From: tex, Archive: a, Name: name, Slot: slot, Fallback: i > 0})
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
