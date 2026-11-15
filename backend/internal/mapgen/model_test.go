package mapgen

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// modelClientDir is the development TAKP client; client tests skip
// without it (mapgen_test.go has the same for the external test package).
const modelClientDir = "/Volumes/T7/EQ/TAKPv22"

func requireModelClient(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(modelClientDir, "global_chr.s3d")); err != nil {
		t.Skipf("EQ client not mounted at %s", modelClientDir)
	}
}

func TestModelCodes(t *testing.T) {
	cases := []struct {
		name         string
		race, gender int
		want         []string
	}{
		{"human male", 1, 0, []string{"HUM", "HUF", "HUN"}},
		{"human female", 1, 1, []string{"HUF", "HUM", "HUN"}},
		{"wood elf", 4, 1, []string{"ELF", "ELM", "ELN"}},
		{"iksar", 128, 0, []string{"IKM", "IKF", "IKN"}},
		{"vah shir", 130, 1, []string{"KEF", "KEM", "KEN"}},
		// Three-letter bases are used as they are, whatever the gender.
		{"gnoll", 39, 0, []string{"GNN"}},
		{"drake", 89, 2, []string{"DRK"}},
		// Two-letter bases: neutral takes the race's own letter.
		{"aviak neutral", 13, 2, []string{"AVI", "AVM", "AVF", "AVN"}},
		{"wolf neutral", 42, 2, []string{"WOL", "WOM", "WOF", "WON"}},
		{"golem neutral", 17, 2, []string{"GOL", "GOM", "GOF", "GON"}},
		{"gargoyle neutral", 29, 2, []string{"GAR", "GAM", "GAF", "GAN"}},
		{"cockatrice neutral", 183, 2, []string{"COK", "COM", "COF", "CON"}},
		// Neutral with no letter of its own: just the gender variants.
		{"clockwork neutral", 15, 2, []string{"BRM", "BRF", "BRN"}},
		{"froglok male", 23, 0, []string{"CPM", "CPF", "CPN"}},
		// Races the client has no model for are drawn as humans.
		{"unknown", 999, 1, []string{"HUF", "HUM", "HUN"}},
		{"water golem (PoP)", 312, 0, []string{"HUM", "HUF", "HUN"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ModelCodes(c.race, c.gender); !reflect.DeepEqual(got, c.want) {
				t.Errorf("ModelCodes(%d, %d) = %v, want %v", c.race, c.gender, got, c.want)
			}
		})
	}
}

func TestModelIndexLocate(t *testing.T) {
	ix := &ModelIndex{Actors: map[string][]string{
		"HUM": {"global_chr.s3d", "global_chr1.s3d", "globalhum_chr.s3d"},
		"GNN": {"blackburrow_chr.s3d", "qeynos_chr.s3d"},
		"IKM": {"global4_chr.s3d", "globalikm_chr.s3d"},
		"SKE": {"global6_chr.s3d", "global_chr.s3d", "mseru_chr.s3d"},
		"KEM": {"globalkem_chr.s3d"},
	}}
	cases := []struct {
		code, zone string
		luclin     bool
		want       string
		ok         bool
	}{
		{"HUM", "", false, "global_chr.s3d", true},
		{"HUM", "", true, "globalhum_chr.s3d", true},
		{"HUM", "qeynos", false, "global_chr.s3d", true},
		// The zone's own copy first, else any archive that has it.
		{"GNN", "qeynos", false, "qeynos_chr.s3d", true},
		{"GNN", "", false, "blackburrow_chr.s3d", true},
		{"SKE", "mseru", false, "mseru_chr.s3d", true},
		{"SKE", "", false, "global_chr.s3d", true},
		{"IKM", "", false, "global4_chr.s3d", true},
		{"IKM", "", true, "globalikm_chr.s3d", true},
		// Only a Luclin copy exists: used even when not asked for.
		{"KEM", "", false, "globalkem_chr.s3d", true},
		{"ZZZ", "", false, "", false},
	}
	for _, c := range cases {
		got, ok := ix.Locate(c.code, c.zone, c.luclin)
		if got != c.want || ok != c.ok {
			t.Errorf("Locate(%s, %q, %v) = %q, %v; want %q, %v", c.code, c.zone, c.luclin, got, ok, c.want, c.ok)
		}
	}

	// Resolve falls through ModelCodes; Luclin is only for player races.
	ix.Actors["AVI"] = []string{"airplane_chr.s3d"}
	if code, arc, err := ix.Resolve(13, 2, "", true); err != nil || code != "AVI" || arc != "airplane_chr.s3d" {
		t.Errorf("Resolve(aviak) = %s, %s, %v", code, arc, err)
	}
	if code, _, err := ix.Resolve(1, 1, "", false); err != nil || code != "HUM" {
		t.Errorf("Resolve(human female, no HUF) = %s, %v; want the HUM fallback", code, err)
	}
	if _, _, err := ix.Resolve(75, 2, "", false); err != ErrNoModel {
		t.Errorf("Resolve(elemental, not indexed) err = %v, want ErrNoModel", err)
	}
}

func TestLazyArchive(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x_chr.s3d")
	raw := buildPFS(t, []string{"humch0001.bmp", "HUM.wld"}, [][]byte{[]byte("texture"), []byte("world")})
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	a, err := OpenArchiveLazy(path)
	if err != nil {
		t.Fatal(err)
	}
	if !a.Has("HUMCH0001.BMP") || !a.Has("hum.wld") || a.Has("nope.bmp") {
		t.Errorf("Has: names = %v", a.Names())
	}
	b, ok, err := a.Get("humch0001.bmp")
	if err != nil || !ok || string(b) != "texture" {
		t.Errorf("Get = %q, %v, %v", b, ok, err)
	}
	if _, ok, _ := a.Get("nope.bmp"); ok {
		t.Error("Get(missing) ok")
	}
}

func TestModelIndexCache(t *testing.T) {
	requireModelClient(t)
	cache := filepath.Join(t.TempDir(), "index.json")
	ix, err := LoadModelIndex(modelClientDir, cache)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cache); err != nil {
		t.Fatalf("index not cached: %v", err)
	}
	again, err := LoadModelIndex(modelClientDir, cache)
	if err != nil || again.Fingerprint != ix.Fingerprint || len(again.Actors) != len(ix.Actors) {
		t.Errorf("cached index differs: %v", err)
	}
	// A stale fingerprint is rebuilt, not trusted.
	os.WriteFile(cache, []byte(`{"fingerprint":"old","actors":{"X":["y"]}}`), 0o644) //nolint:errcheck
	fresh, err := LoadModelIndex(modelClientDir, cache)
	if err != nil || fresh.Fingerprint != ix.Fingerprint {
		t.Errorf("stale cache used: %v", err)
	}
}

func TestLoadModelPosed(t *testing.T) {
	requireModelClient(t)
	cases := []struct {
		archive, code string
		minTris       int
	}{
		{"global_chr.s3d", "HUM", 400},
		{"globalhum_chr.s3d", "HUM", 1500},
		{"qeynos_chr.s3d", "GNN", 400},
		{"sleeper_chr.s3d", "DRK", 800},
	}
	for _, c := range cases {
		m, err := LoadModel(modelClientDir, c.archive, c.code)
		if err != nil {
			t.Errorf("%s/%s: %v", c.archive, c.code, err)
			continue
		}
		if n := len(m.Idx) / 3; n < c.minTris {
			t.Errorf("%s/%s: %d triangles, want at least %d", c.archive, c.code, n, c.minTris)
		}
		for _, mat := range m.Materials {
			if mat.Texture == "" {
				continue
			}
			if arc, err := OpenArchiveLazy(filepath.Join(modelClientDir, c.archive)); err == nil && !arc.Has(mat.Texture) {
				t.Errorf("%s/%s: texture %s not in the archive", c.archive, c.code, mat.Texture)
			}
			break
		}
	}

	// Idle, not the rest pose: the Luclin human's arms hang down, so the
	// model is far narrower than it is tall (arms out, it's about square).
	m, err := LoadModel(modelClientDir, "globalhum_chr.s3d", "HUM")
	if err != nil {
		t.Fatal(err)
	}
	var lo, hi [3]float32
	for i := 0; i+2 < len(m.Pos); i += 3 {
		for k := 0; k < 3; k++ {
			v := m.Pos[i+k]
			if i == 0 || v < lo[k] {
				lo[k] = v
			}
			if i == 0 || v > hi[k] {
				hi[k] = v
			}
		}
	}
	width := max(hi[0]-lo[0], hi[1]-lo[1])
	if height := hi[2] - lo[2]; width > height*0.5 {
		t.Errorf("Luclin human %.1f wide, %.1f tall: not posed idle", width, height)
	}
	// Original models without an idle of their own borrow one: a dark
	// elf female (no DAF tracks) stands with her arms down too.
	for _, code := range []string{"DAF", "GNM", "HUF"} {
		m, err := LoadModel(modelClientDir, "global_chr.s3d", code)
		if err != nil {
			t.Fatal(err)
		}
		var lo, hi [3]float32
		for i := 0; i+2 < len(m.Pos); i += 3 {
			for k := 0; k < 3; k++ {
				v := m.Pos[i+k]
				if i == 0 || v < lo[k] {
					lo[k] = v
				}
				if i == 0 || v > hi[k] {
					hi[k] = v
				}
			}
		}
		width := max(hi[0]-lo[0], hi[1]-lo[1])
		if height := hi[2] - lo[2]; width > height*0.75 {
			t.Errorf("%s %.1f wide, %.1f tall: still in the rest pose", code, width, height)
		}
	}
	if _, err := LoadModel(modelClientDir, "global_chr.s3d", "NOPE"); err == nil {
		t.Error("LoadModel(missing actor) succeeded")
	}
}
