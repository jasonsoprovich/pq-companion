package mapgen

import (
	"fmt"
	"strings"
)

// The Luclin models wear more than textures: hair, robes, tunics, skirts,
// helms and every piece of plate are separate models (IT#### actors in the
// lgequip archives) that the client hangs on the body's bones. Without
// them a Luclin character is bald and plate shows as bare skin (there are
// no Luclin plate chest textures because the plate is a model).
//
// The rules are eqgame.exe's (TAKP): 0x4a02e8 picks the models per armor
// slot, 0x49f7c7 adds the race and gender's offset to each, 0x49f915 the
// hair, 0x4a0714 takes the hair off under a helm, and 0x4a00d9 names the
// bone each piece hangs from.

// luclinWearOffsets is the client's per-model offset into the IT numbers
// (0x49f7c7): 30 per race, males first.
var luclinWearOffsets = map[string]int{
	"GNM": 0, "GNF": 30, "HAM": 60, "HAF": 90, "DWM": 120, "DWF": 150,
	"DAM": 180, "DAF": 210, "ELM": 240, "ELF": 270, "HIM": 300, "HIF": 330,
	"HUM": 360, "HUF": 390, "IKM": 420, "IKF": 450, "ERM": 480, "ERF": 510,
	"BAM": 540, "BAF": 570, "KEM": 600, "KEF": 630, "TRM": 660, "TRF": 690,
	"OGM": 720, "OGF": 750, "HOM": 780, "HOF": 810,
}

// LuclinWearArchives are the client's archives of Luclin worn models, in
// the order it loads them.
var LuclinWearArchives = []string{"lgequip.s3d", "lgequip2.s3d", "lgequip_amr.s3d", "lgequip_amr2.s3d"}

// Wearable is one model a Luclin character wears.
type Wearable struct {
	// Item is the actor code ("IT4360").
	Item string `json:"item"`
	// Point is the body's bone it hangs from ("TUNIC_POINT").
	Point string `json:"point"`
	// Slot is the material slot it belongs to, for its tint; -1 for hair.
	Slot int `json:"slot"`
}

// NoHair is the hairstyle for a bald character.
const NoHair = -1

// LuclinWear lists the models a Luclin model (code, "HUM") wears for its
// materials, hairstyle (0-2 on most models; NoHair for none) and class,
// keeping those has finds in the client. nil for a code that isn't a
// Luclin player model.
func LuclinWear(code string, mats [MaterialSlots]int, hair int, who Wearer, has func(item string) bool) []Wearable {
	code = strings.ToUpper(code)
	off, ok := luclinWearOffsets[code]
	if !ok {
		return nil
	}
	lower := strings.ToLower(code)
	// What each slot shows as, by the client's remaps (scale as chain,
	// Velious sets as their base material, and so on).
	var m [MaterialSlots]int
	for s, mat := range mats {
		c, _ := materialCandidates(mat, lower, "", who)
		m[s] = c[len(c)-1]
	}
	iksarLike := strings.HasPrefix(code, "IK") || strings.HasPrefix(code, "KE")
	robe := IsRobe(m[SlotChest])

	var out []Wearable
	add := func(n int, point string, slot int) bool {
		item := fmt.Sprintf("IT%d", n+off)
		if !has(item) {
			return false
		}
		out = append(out, Wearable{Item: item, Point: point, Slot: slot})
		return true
	}

	// Head: leather and chain helms by material, plate (and the plate
	// stand-ins 240 and 241) as its own; a bare head tries 5000, which
	// only the Erudites' scalp has. Erudites' robes have no helm.
	helm := false
	if h := m[SlotHead]; !(robe && strings.HasPrefix(code, "ER")) {
		switch {
		case h == 241, h >= 3 && strings.HasPrefix(code, "BA"):
			helm = add(5029, "HEAD_POINT", SlotHead)
		case (h == 3 || h == 240) && iksarLike:
			helm = add(5003, "HEAD_POINT", SlotHead)
		case h == 3 || h == 240:
			helm = add(5028, "HEAD_POINT", SlotHead)
		default:
			helm = add(5000+h, "HEAD_POINT", SlotHead)
		}
	}

	// Chest: a plate chest, and a tunic (the bare chest's is a loincloth)
	// or robe.
	switch c := m[SlotChest]; {
	case c >= 0 && c <= 3:
		if c == 3 {
			add(6000, "CHEST_POINT", SlotChest)
		}
		if !(code == "GNF" && c == 1) {
			add(4010+c, "TUNIC_POINT", SlotChest)
		}
	case robe:
		add(3990+c, "TUNIC_POINT", SlotChest)
	}

	// A robe covers the arms, wrists, legs and feet.
	if !robe {
		pair := func(base, mat int, right, left string, slot int) {
			if mat >= 0 && mat <= 3 {
				add(base+2*mat, right, slot)
				add(base+2*mat+1, left, slot)
			}
		}
		pair(7000, m[SlotArms], "SHOULDR_POINT", "SHOULDL_POINT", SlotArms)
		pair(8000, m[SlotWrist], "GAUNTR_POINT", "GAUNTL_POINT", SlotWrist)
		add(4020+m[SlotLegs], "PELVIS_POINT", SlotLegs)
		pair(9000, m[SlotFeet], "LEGL_POINT", "LEGR_POINT", SlotFeet)
	}

	// Hair, unless something's on the head (Vah Shir have none).
	if hair >= 0 && !helm && !strings.HasPrefix(code, "KE") {
		add(1000+hair, "HAIR_POINT", -1)
	}
	return out
}
