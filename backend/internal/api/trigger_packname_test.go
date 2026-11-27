package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
)

// Raid pack names carry a colon, which the client's encodeURIComponent
// sends as %3A; chi then routes on RawPath and hands back the encoded
// param. Every pack route must still see the plain name.
func TestPackNameParamDecodesEncodedName(t *testing.T) {
	var got string
	r := chi.NewRouter()
	r.Post("/packs/{name}", func(w http.ResponseWriter, r *http.Request) {
		got = packNameParam(r)
	})
	r.Get("/packs/{name}/diff", func(w http.ResponseWriter, r *http.Request) {
		got = packNameParam(r)
	})
	cases := map[string]string{
		"/packs/Raid%3A%20PoP%20Tier%201":          "Raid: PoP Tier 1",
		"/packs/Raid%3A%20Velious/diff":            "Raid: Velious",
		"/packs/Raid%20Alerts":                     "Raid Alerts",
		"/packs/Raid:%20Scripted%20Event%20Timers": "Raid: Scripted Event Timers",
	}
	for path, want := range cases {
		got = ""
		method := http.MethodPost
		if len(path) > 5 && path[len(path)-5:] == "/diff" {
			method = http.MethodGet
		}
		r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(method, path, nil))
		if got != want {
			t.Errorf("%s: got %q, want %q", path, got, want)
		}
	}
}
