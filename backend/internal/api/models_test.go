package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestModelsRejectBadInput(t *testing.T) {
	h := &modelsHandler{}
	r := chi.NewRouter()
	r.Get("/resolve", h.resolve)
	r.Get("/mesh/{archive}/{code}", h.mesh)
	r.Get("/texture/{archive}/{name}", h.texture)
	for _, path := range []string{
		"/resolve?race=abc",
		"/resolve?race=-1",
		"/resolve?race=1&zone=..%2Fx",
		"/mesh/..%2Fglobal_chr.s3d/HUM",
		"/mesh/qeynos.s3d/HUM",         // not a character archive
		"/mesh/global_chr.s3d/hum.wld", // not an actor code
		"/texture/global_chr.s3d/..%2Fx.bmp",
		"/texture/global_chr.s3d/humch0001.png",
		"/texture/gequip.s3d/it1.bmp",
	} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", path, rec.Code)
		}
	}
	for _, name := range []string{"global_chr.s3d", "global_chr1.s3d", "globalhum_chr2.s3d", "sleeper2_chr.s3d", "global7_chr.s3d"} {
		if !chrArchiveRe.MatchString(name) {
			t.Errorf("%s rejected", name)
		}
	}
}
