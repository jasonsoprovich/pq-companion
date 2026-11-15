package mapgen

import (
	"reflect"
	"testing"
)

func TestLuclinWear(t *testing.T) {
	// all pretends the client has every model; erudites lets only the
	// Erudite scalp through for the bare head.
	all := func(string) bool { return true }
	noScalp := func(item string) bool { return item != "IT5360" && item != "IT5210" }
	type w = Wearable
	cases := []struct {
		name string
		code string
		mats [MaterialSlots]int
		hair int
		who  Wearer
		has  func(string) bool
		want []Wearable
	}{
		{
			name: "bare human: loincloth, bare legs and hair", code: "HUM", has: noScalp,
			want: []Wearable{
				{"IT4370", "TUNIC_POINT", SlotChest},
				{"IT7360", "SHOULDR_POINT", SlotArms}, {"IT7361", "SHOULDL_POINT", SlotArms},
				{"IT8360", "GAUNTR_POINT", SlotWrist}, {"IT8361", "GAUNTL_POINT", SlotWrist},
				{"IT4380", "PELVIS_POINT", SlotLegs},
				{"IT9360", "LEGL_POINT", SlotFeet}, {"IT9361", "LEGR_POINT", SlotFeet},
				{"IT1360", "HAIR_POINT", -1},
			},
		},
		{
			name: "plate human: helm hides the hair", code: "HUM", hair: 1, has: noScalp,
			mats: [MaterialSlots]int{3, 3, 3, 3, 3, 3, 3},
			want: []Wearable{
				{"IT5388", "HEAD_POINT", SlotHead},
				{"IT6360", "CHEST_POINT", SlotChest}, {"IT4373", "TUNIC_POINT", SlotChest},
				{"IT7366", "SHOULDR_POINT", SlotArms}, {"IT7367", "SHOULDL_POINT", SlotArms},
				{"IT8366", "GAUNTR_POINT", SlotWrist}, {"IT8367", "GAUNTL_POINT", SlotWrist},
				{"IT4383", "PELVIS_POINT", SlotLegs},
				{"IT9366", "LEGL_POINT", SlotFeet}, {"IT9367", "LEGR_POINT", SlotFeet},
			},
		},
		{
			name: "robed dark elf: robe covers the limbs", code: "DAF", hair: 2, has: noScalp,
			mats: [MaterialSlots]int{0, 12, 1, 1, 1, 1, 1},
			want: []Wearable{{"IT4212", "TUNIC_POINT", SlotChest}, {"IT1212", "HAIR_POINT", -1}},
		},
		{
			name: "remaps: Velious chain set, scale on an iksar", code: "IKM", has: all, hair: NoHair,
			mats: [MaterialSlots]int{0, 18, 7, 0, 0, 0, 0},
			want: []Wearable{
				{"IT5420", "HEAD_POINT", SlotHead},
				{"IT4432", "TUNIC_POINT", SlotChest},
				{"IT7426", "SHOULDR_POINT", SlotArms}, {"IT7427", "SHOULDL_POINT", SlotArms},
				{"IT8420", "GAUNTR_POINT", SlotWrist}, {"IT8421", "GAUNTL_POINT", SlotWrist},
				{"IT4440", "PELVIS_POINT", SlotLegs},
				{"IT9420", "LEGL_POINT", SlotFeet}, {"IT9421", "LEGR_POINT", SlotFeet},
			},
		},
		{
			name: "barbarian plate helm", code: "BAF", has: func(i string) bool { return i == "IT5599" },
			mats: [MaterialSlots]int{SlotHead: 3},
			want: []Wearable{{"IT5599", "HEAD_POINT", SlotHead}},
		},
		{
			name: "only what the client has", code: "HUF", has: func(i string) bool { return i == "IT1390" },
			want: []Wearable{{"IT1390", "HAIR_POINT", -1}},
		},
		{name: "not a Luclin model", code: "SKE", has: all},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := LuclinWear(c.code, c.mats, c.hair, c.who, c.has)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("got  %v\nwant %v", got, c.want)
			}
		})
	}
}

// TestWearOnBody hangs Luclin worn models on a posed body: hair on the
// head, the robe from the shoulders down, the plate chest on the torso.
func TestWearOnBody(t *testing.T) {
	requireModelClient(t)
	body, err := LoadModelWith(modelClientDir, "globalhum_chr.s3d", "HUM", ModelOptions{})
	if err != nil {
		t.Fatal(err)
	}
	zRange := func(m *Mesh3D) (lo, hi float32) {
		lo, hi = 1e9, -1e9
		for i := 2; i < len(m.Pos); i += 3 {
			lo, hi = min(lo, m.Pos[i]), max(hi, m.Pos[i])
		}
		return lo, hi
	}
	_, top := zRange(body.Mesh)
	bottom, _ := zRange(body.Mesh)
	height := top - bottom
	cases := []struct {
		archive, item, point string
		// lo and hi bound the worn model's z extent as fractions of the
		// body's height from its feet.
		lo, hi float32
	}{
		{"lgequip.s3d", "IT1360", "HAIR_POINT", 0.8, 1.02},
		{"lgequip.s3d", "IT4360", "TUNIC_POINT", 0, 0.95},
		{"lgequip_amr.s3d", "IT6360", "CHEST_POINT", 0.5, 0.95},
		{"lgequip_amr.s3d", "IT5388", "HEAD_POINT", 0.8, 1.1},
	}
	for _, c := range cases {
		m, err := body.Wear(modelClientDir, c.archive, c.item, c.point)
		if err != nil {
			t.Errorf("%s: %v", c.item, err)
			continue
		}
		lo, hi := zRange(m)
		flo, fhi := (lo-bottom)/height, (hi-bottom)/height
		if flo < c.lo-0.02 || fhi > c.hi {
			t.Errorf("%s spans %.2f-%.2f of the body, want within %.2f-%.2f", c.item, flo, fhi, c.lo, c.hi)
		}
	}
	if _, err := body.Wear(modelClientDir, "lgequip.s3d", "IT1360", "NO_POINT"); err == nil {
		t.Error("worn on a bone the body hasn't")
	}
}
