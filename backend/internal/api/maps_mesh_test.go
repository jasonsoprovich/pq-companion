package api

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/go-chi/chi/v5"
)

func TestMesh3DRejectsBadZone(t *testing.T) {
	h := &mapsHandler{}
	r := chi.NewRouter()
	r.Get("/zone/{zone}/mesh", h.mesh3D)
	for _, zone := range []string{"..%2F..%2Fetc", "Qeynos", "a.b"} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/zone/"+zone+"/mesh", nil))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%q: status %d, want 400", zone, rec.Code)
		}
	}
}

func TestMeshCacheEvicts(t *testing.T) {
	var c meshCache
	for _, k := range []string{"a", "b", "c", "d"} {
		c.put(k, []byte(k))
	}
	if _, ok := c.get("a"); ok {
		t.Fatal("oldest entry kept past the cache size")
	}
	if b, ok := c.get("d"); !ok || string(b) != "d" {
		t.Fatal("newest entry missing")
	}
}

func TestTexture3DRejectsBadName(t *testing.T) {
	h := &mapsHandler{}
	r := chi.NewRouter()
	r.Get("/zone/{zone}/texture/{name}", h.texture3D)
	for _, name := range []string{"..%2Fx.bmp", "brick.png", "a.b.bmp"} {
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/zone/unrest/texture/"+name, nil))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%q: status %d, want 400", name, rec.Code)
		}
	}
	for _, name := range []string{"stained%20glass.bmp", "drana%27sbb.bmp", "sidewalk%26vines.bmp"} {
		if n, _ := url.PathUnescape(name); !textureNameRe.MatchString(n) {
			t.Errorf("%q rejected", n)
		}
	}
}
