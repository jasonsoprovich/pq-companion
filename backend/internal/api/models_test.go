package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jasonsoprovich/pq-companion/backend/internal/character"
	"github.com/jasonsoprovich/pq-companion/backend/internal/mapgen"
	"github.com/jasonsoprovich/pq-companion/backend/internal/players"
)

func TestModelsRejectBadInput(t *testing.T) {
	h := &modelsHandler{}
	r := chi.NewRouter()
	r.Get("/resolve", h.resolve)
	r.Get("/mesh/{archive}/{code}", h.mesh)
	r.Get("/texture/{archive}/{name}", h.texture)
	r.Get("/outfit/{archive}/{code}", h.outfit)
	r.Get("/locate/{code}", h.locate)
	r.Get("/attach/{archive}/{code}/{point}/{itemArchive}/{item}", h.attach)
	for _, path := range []string{
		"/resolve?race=abc",
		"/resolve?race=-1",
		"/resolve?race=1&zone=..%2Fx",
		"/mesh/..%2Fglobal_chr.s3d/HUM",
		"/mesh/qeynos.s3d/HUM",         // not a character archive
		"/mesh/global_chr.s3d/hum.wld", // not an actor code
		"/texture/global_chr.s3d/..%2Fx.bmp",
		"/texture/global_chr.s3d/humch0001.png",
		"/texture/lgequip_obj.s3d/it1.bmp", // not a model archive
		"/texture/global_chr.s3d/humch0001.bmp?size=100",
		"/texture/global_chr.s3d/humch0001.bmp?size=512",
		"/texture/global_chr.s3d/humch0001.bmp?size=x",
		"/outfit/global_chr.s3d/HUM?mats=1,2,3",
		"/outfit/global_chr.s3d/HUM?mats=1,2,3,4,5,6,x",
		"/attach/global_chr.s3d/HUM/NECK_POINT/gequip.s3d/IT1",
		"/attach/global_chr.s3d/HUM/R_POINT/qeynos.s3d/IT1",
		"/locate/HUM",
	} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", path, rec.Code)
		}
	}
	for _, name := range []string{"global_chr.s3d", "global_chr1.s3d", "globalhum_chr2.s3d", "sleeper2_chr.s3d", "global7_chr.s3d", "gequip.s3d", "gequip8.s3d", "global17_amr.s3d"} {
		if !modelArchiveRe.MatchString(name) {
			t.Errorf("%s rejected", name)
		}
	}
}

func TestModelsPeople(t *testing.T) {
	dir := t.TempDir()
	chars, err := character.OpenStore(filepath.Join(dir, "user.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer chars.Close()
	ps, err := players.OpenStore(filepath.Join(dir, "players.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer ps.Close()
	if _, err := chars.Create("Osui", 6, 6, 60); err != nil {
		t.Fatal(err)
	}
	if err := chars.SetAppearance(character.Appearance{Name: "Osui", Gender: 1, Face: 4, HairColor: 13, Beard: -1, BeardColor: -1, Eyes: 2}); err != nil {
		t.Fatal(err)
	}
	if err := ps.Upsert(players.SightingInput{Name: "Bela", Race: "Vah Shir", Class: "Warrior", Level: 50, ObservedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	// Seen in game by Zeal: wins over /who and the character list.
	if err := ps.SetLook("Seenya", players.Look{Race: 128, Gender: 1, Size: 6, Face: 2, HairStyle: 255,
		BeardStyle: 255, HairColor: 255, BeardColor: 255,
		Mats: [7]int{0, 2, 2, 2, 2, 2, 2}, Tints: [7]uint32{0, 0x323264}, Primary: "IT10"}, time.Now()); err != nil {
		t.Fatal(err)
	}
	h := &modelsHandler{chars: chars, players: ps}
	rec := httptest.NewRecorder()
	h.people(rec, httptest.NewRequest(http.MethodGet, "/people?names=osui,Bela,Nobody,Seenya", nil))
	var got []personModel
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	want := []personModel{
		{Name: "osui", Race: 6, Gender: 1, Known: true, Looks: mapgen.Looks{Face: 4, HairColor: 13, Beard: -1, BeardColor: -1, Eyes: 2}},
		{Name: "Bela", Race: 130, Known: true, Looks: mapgen.DefaultLooks},
		{Name: "Nobody", Looks: mapgen.DefaultLooks},
		{
			Name: "Seenya", Race: 128, Gender: 1, Known: true, Seen: true, Size: 6,
			Looks: mapgen.Looks{Face: 2, Hair: 0, HairColor: -1, Beard: -1, BeardColor: -1},
			Dress: &personDress{Mats: [7]int{0, 2, 2, 2, 2, 2, 2}, Tints: [7]uint32{0, 0x323264}, Primary: "IT10"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("people = %+v, want %+v", got, want)
	}
}
