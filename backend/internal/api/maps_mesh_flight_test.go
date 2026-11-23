package api

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
)

// Concurrent requests for one uncached zone build it once and all get the
// same bytes.
func TestMesh3DConcurrentMissesBuildOnce(t *testing.T) {
	cfg, err := config.LoadFrom(filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if err := cfg.Modify(func(c *config.Config) { c.EQPath = t.TempDir() }); err != nil {
		t.Fatal(err)
	}
	var builds atomic.Int32
	want := []byte("mesh bytes")
	release := make(chan struct{})
	h := &mapsHandler{cfg: cfg}
	h.loadMesh = func(eqPath, zone string) ([]byte, error) {
		builds.Add(1)
		<-release
		return want, nil
	}
	r := chi.NewRouter()
	r.Get("/zone/{zone}/mesh", h.mesh3D)
	const n = 8
	var wg sync.WaitGroup
	bodies := make([][]byte, n)
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rec := httptest.NewRecorder()
			r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/zone/tz/mesh", nil))
			bodies[i] = rec.Body.Bytes()
		}()
	}
	time.Sleep(50 * time.Millisecond) // let every request reach the build
	close(release)
	wg.Wait()
	if b := builds.Load(); b != 1 {
		t.Errorf("%d builds, want 1", b)
	}
	for i, b := range bodies {
		if !bytes.Equal(b, want) {
			t.Errorf("request %d got %q", i, b)
		}
	}
}

func TestFlightSharesErrorsAndForgets(t *testing.T) {
	var f flight
	boom := errors.New("boom")
	if _, err := f.do("k", func() ([]byte, error) { return nil, boom }); err != boom {
		t.Fatalf("err = %v", err)
	}
	// A finished call is forgotten: the next one runs again.
	b, err := f.do("k", func() ([]byte, error) { return []byte("ok"), nil })
	if err != nil || string(b) != "ok" {
		t.Errorf("second call = %q, %v", b, err)
	}
	if len(f.calls) != 0 {
		t.Errorf("%d calls left in flight", len(f.calls))
	}
}

func TestMeshCacheDefaultByteBound(t *testing.T) {
	var c meshCache
	c.put("big1", make([]byte, 40<<20))
	c.put("big2", make([]byte, 40<<20))
	if _, ok := c.get("big1"); ok {
		t.Error("two big meshes kept past meshCacheBytes")
	}
	c.put("small", make([]byte, 4<<20))
	if _, ok := c.get("big2"); !ok {
		t.Error("big mesh plus a small one should both fit")
	}
}
