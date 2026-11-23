package mapgen

import (
	"reflect"
	"testing"
)

func TestHairTint(t *testing.T) {
	cases := []struct {
		code string
		idx  int
		want uint32
	}{
		{"HUM", -1, 0x2e1a0c},
		{"HUM", 12, 0xffffff},
		{"HUM", 23, 0x2e1a0c}, // past the human range: the default
		{"HIF", -1, 0x650b06}, // high elves start at 4
		{"DAF", 19, 0xc9e5fd},
		{"TRM", 23, 0x559b48},
		{"IKM", 3, 0},
		{"KEF", 3, 0},
	}
	for _, c := range cases {
		if got := hairTint(c.code, c.idx); got != c.want {
			t.Errorf("hairTint(%s, %d) = %06x, want %06x", c.code, c.idx, got, c.want)
		}
	}
}

func TestLookTextures(t *testing.T) {
	files := map[string]bool{
		"globalhum_chr.s3d/humhesk31.dds": true,
		"globalhum_chr.s3d/humhesk34.dds": true,
		"global_chr.s3d/humhe0031.bmp":    true,
		"lgequip.s3d/chr_eye025.dds":      true,
	}
	has := func(a, n string) bool { return files[a+"/"+n] }
	luclin := []string{"HUMHESK01.DDS", "humhesk02.dds", "humhesk04.dds", "chr_eye001.dds", "humch0001.dds"}
	got := LookTextures("globalhum_chr.s3d", "HUM", true, luclin, Looks{Face: 3, Eyes: 5}, has)
	want := []TextureSwap{
		{From: "HUMHESK01.DDS", Archive: "globalhum_chr.s3d", Name: "humhesk31.dds", Slot: -1},
		{From: "humhesk04.dds", Archive: "globalhum_chr.s3d", Name: "humhesk34.dds", Slot: -1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("luclin face 3: got %v, want %v", got, want)
	}
	got = LookTextures("globaldaf_chr.s3d", "DAF", true, []string{"chr_eye021.dds"}, Looks{Eyes: 5}, has)
	want = []TextureSwap{{From: "chr_eye021.dds", Archive: "lgequip.s3d", Name: "chr_eye025.dds", Slot: -1}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("dark elf eyes: got %v, want %v", got, want)
	}
	got = LookTextures("global_chr.s3d", "HUM", false, []string{"humhe0001.bmp", "humhe0002.bmp"}, Looks{Face: 3}, has)
	want = []TextureSwap{{From: "humhe0001.bmp", Archive: "global_chr.s3d", Name: "humhe0031.bmp", Slot: -1}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("original face 3: got %v, want %v", got, want)
	}
	if got := LookTextures("globalhum_chr.s3d", "HUM", true, luclin, DefaultLooks, has); len(got) != 0 {
		t.Errorf("default looks swapped %v", got)
	}
}

// TestAvailableLooksClient reads what the client offers a human male.
func TestAvailableLooksClient(t *testing.T) {
	requireModelClient(t)
	m, err := LoadModelWith(modelClientDir, "globalhum_chr.s3d", "HUM", ModelOptions{})
	if err != nil {
		t.Fatal(err)
	}
	var textures []string
	for _, mat := range m.Mesh.Materials {
		textures = append(textures, mat.Texture)
	}
	archives := map[string]*LazyArchive{}
	defer func() {
		for _, arc := range archives {
			arc.Close()
		}
	}()
	has := func(a, n string) bool {
		arc, ok := archives[a]
		if !ok {
			arc, _ = OpenArchiveLazy(modelClientDir + "/" + a)
			archives[a] = arc
		}
		return arc != nil && arc.Has(n)
	}
	ix, err := BuildModelIndex(modelClientDir)
	if err != nil {
		t.Fatal(err)
	}
	hasItem := func(item string) bool { _, ok := ix.Locate(item, "", false); return ok }
	got := AvailableLooks("globalhum_chr.s3d", "HUM", true, textures, has, hasItem)
	t.Logf("%+v", got)
	if got.Faces != 8 || len(got.Hair) != 3 || len(got.Beards) == 0 || len(got.HairColors) != 19 || len(got.Eyes) != 10 {
		t.Errorf("got %+v", got)
	}
}
