package mapgen

import (
	"reflect"
	"testing"
)

func TestDressTextures(t *testing.T) {
	files := map[string]bool{
		"global_chr.s3d|humch0301.bmp":    true,
		"global_chr.s3d|humch0302.bmp":    true,
		"global_chr.s3d|humlg0101.bmp":    true,
		"global_chr.s3d|humua0301.bmp":    true,
		"global_chr.s3d|clk0601.bmp":      true,
		"global17_amr.s3d|humua1701.bmp":  true,
		"globalhum_chr.s3d|humch0201.dds": true,
		"globalhum_chr.s3d|humlg0301.dds": true,
	}
	has := func(a, n string) bool { return files[a+"|"+n] }
	old := []string{"humch0001.bmp", "humch0002.bmp", "humlg0001.bmp", "humua0001.bmp", "humhe0001.bmp", "humft0001.bmp"}
	luclin := []string{"humchsk01.dds", "humlgsk01.dds", "humhesk01.dds"}
	robe := []string{"clk0401.bmp", "humft0001.bmp"}

	cases := []struct {
		name     string
		archive  string
		who      Wearer
		textures []string
		mats     [MaterialSlots]int
		want     OutfitResult
	}{
		{
			name: "plate chest, leather legs, Velious arms", archive: "global_chr.s3d", textures: old,
			mats: [MaterialSlots]int{SlotHead: 3, SlotChest: 3, SlotArms: 17, SlotLegs: 1},
			want: OutfitResult{Swaps: []TextureSwap{
				{From: "humch0001.bmp", Archive: "global_chr.s3d", Name: "humch0301.bmp", Slot: SlotChest},
				{From: "humch0002.bmp", Archive: "global_chr.s3d", Name: "humch0302.bmp", Slot: SlotChest},
				{From: "humlg0001.bmp", Archive: "global_chr.s3d", Name: "humlg0101.bmp", Slot: SlotLegs},
				{From: "humua0001.bmp", Archive: "global17_amr.s3d", Name: "humua1701.bmp", Slot: SlotArms},
			}},
		},
		{
			name: "nothing worn", archive: "global_chr.s3d", textures: old,
			want: OutfitResult{},
		},
		{
			name: "no texture for the material", archive: "global_chr.s3d", textures: old,
			mats: [MaterialSlots]int{SlotFeet: 9},
			want: OutfitResult{Missing: []int{SlotFeet}},
		},
		{
			// The client's remaps: monk armor is leather on others, scale
			// is chain, a Velious set without its textures is plate.
			name: "client remaps", archive: "global_chr.s3d", textures: old,
			mats: [MaterialSlots]int{SlotLegs: 4, SlotChest: 7, SlotArms: 19},
			want: OutfitResult{Swaps: []TextureSwap{
				{From: "humlg0001.bmp", Archive: "global_chr.s3d", Name: "humlg0101.bmp", Slot: SlotLegs},
				{From: "humua0001.bmp", Archive: "global_chr.s3d", Name: "humua0301.bmp", Slot: SlotArms},
			}, Missing: []int{SlotChest}},
		},
		{
			name: "robe", archive: "global_chr.s3d", textures: robe,
			mats: [MaterialSlots]int{SlotChest: 12},
			want: OutfitResult{Swaps: []TextureSwap{
				{From: "clk0401.bmp", Archive: "global_chr.s3d", Name: "clk0601.bmp", Slot: SlotChest},
			}},
		},
		{
			// The plate chest is a worn model; the body under it stays bare.
			name: "luclin plate: no chest texture", archive: "globalhum_chr.s3d", who: Wearer{Luclin: true}, textures: luclin,
			mats: [MaterialSlots]int{SlotChest: 3, SlotLegs: 3},
			want: OutfitResult{Swaps: []TextureSwap{
				{From: "humlgsk01.dds", Archive: "globalhum_chr.s3d", Name: "humlg0301.dds", Slot: SlotLegs},
			}, Missing: []int{SlotChest}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := DressTextures(c.archive, c.who, c.textures, c.mats, has)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("got %+v\nwant %+v", got, c.want)
			}
		})
	}
}

func TestItemModelCode(t *testing.T) {
	for in, want := range map[string]string{"IT63": "IT63", "it10500": "IT10500", "IT0": "", "": "", "ITX": "", "BBBOARD": ""} {
		if got := ItemModelCode(in); got != want {
			t.Errorf("ItemModelCode(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestLoadModelWithAndAttach(t *testing.T) {
	requireModelClient(t)
	texturesOf := func(m *Mesh3D) map[string]bool {
		out := map[string]bool{}
		for _, mat := range m.Materials {
			out[mat.Texture] = true
		}
		return out
	}
	bare, err := LoadModelWith(modelClientDir, "global_chr.s3d", "HUM", ModelOptions{})
	if err != nil {
		t.Fatal(err)
	}
	helm, err := LoadModelWith(modelClientDir, "global_chr.s3d", "HUM", ModelOptions{Helm: 2})
	if err != nil {
		t.Fatal(err)
	}
	if len(helm.Mesh.Idx) <= len(bare.Mesh.Idx) {
		t.Errorf("chain helm adds nothing: %d vs %d indices", len(helm.Mesh.Idx), len(bare.Mesh.Idx))
	}
	robe, err := LoadModelWith(modelClientDir, "global_chr.s3d", "HUM", ModelOptions{Robe: true})
	if err != nil {
		t.Fatal(err)
	}
	if tex := texturesOf(robe.Mesh); !tex["clk0401.bmp"] || tex["humch0001.bmp"] {
		t.Errorf("robed body textures = %v", tex)
	}

	// A sword in the right hand ends up beside the body at hand height,
	// not at the model's feet or origin.
	sword, err := LoadModel(modelClientDir, "gequip.s3d", "IT1")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []*PosedModel{bare, func() *PosedModel {
		m, err := LoadModelWith(modelClientDir, "globalhum_chr.s3d", "HUM", ModelOptions{})
		if err != nil {
			t.Fatal(err)
		}
		return m
	}()} {
		placed, ok := p.Attach(sword, "R_POINT")
		if !ok {
			t.Fatal("no R_POINT")
		}
		bottom, top := p.Mesh.Pos[2], p.Mesh.Pos[2]
		for i := 2; i < len(p.Mesh.Pos); i += 3 {
			bottom, top = min(bottom, p.Mesh.Pos[i]), max(top, p.Mesh.Pos[i])
		}
		var sz float32
		n := 0
		for i := 2; i < len(placed.Pos); i += 3 {
			sz += placed.Pos[i]
			n++
		}
		if h := (sz/float32(n) - bottom) / (top - bottom); h < 0.15 || h > 0.85 {
			t.Errorf("%s sword at %.0f%% of the model's height", p.code, h*100)
		}
		if _, ok := p.Attach(sword, "NOWHERE"); ok {
			t.Error("attached at a missing point")
		}
	}
}
