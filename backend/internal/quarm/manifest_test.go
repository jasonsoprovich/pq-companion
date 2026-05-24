package quarm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

const sampleManifest = `version: 2025021585884904e26fe327e3716afc41ed29b8
downloadprefix: https://raw.githubusercontent.com/Pkelly668/QuarmPatcher/master/rof/
downloads:
- name: eqgame.dll
  md5: 0d7fbe37f9478f51aa983eae07546d81
  date: "20250215"
  size: 280064
- name: RaceData.txt
  md5: 355c060172b8f974ce9fbaad6249e337
  date: "20250215"
  size: 768
`

func newTestFetcher(srv *httptest.Server, ttl time.Duration) *ManifestFetcher {
	return &ManifestFetcher{
		url:    srv.URL,
		ttl:    ttl,
		client: srv.Client(),
	}
}

func TestManifestFetcher_Parse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		w.Write([]byte(sampleManifest))
	}))
	defer srv.Close()

	f := newTestFetcher(srv, time.Hour)
	m, err := f.Get(context.Background())
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if m.Version != "2025021585884904e26fe327e3716afc41ed29b8" {
		t.Errorf("version = %q", m.Version)
	}
	if len(m.Downloads) != 2 {
		t.Fatalf("downloads = %d, want 2", len(m.Downloads))
	}
	e := m.FindEntry("EQGame.DLL")
	if e == nil {
		t.Fatal("FindEntry returned nil for case-insensitive lookup")
	}
	if e.MD5 != "0d7fbe37f9478f51aa983eae07546d81" || e.Size != 280064 {
		t.Errorf("entry = %+v", e)
	}
	if m.FindEntry("eqw.dll") != nil {
		t.Error("expected nil for missing entry")
	}
}

func TestManifestFetcher_Cache(t *testing.T) {
	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Write([]byte(sampleManifest))
	}))
	defer srv.Close()

	f := newTestFetcher(srv, time.Hour)
	for i := 0; i < 5; i++ {
		if _, err := f.Get(context.Background()); err != nil {
			t.Fatalf("get %d: %v", i, err)
		}
	}
	if got := atomic.LoadInt32(&hits); got != 1 {
		t.Errorf("server hits = %d, want 1 (cache should suppress subsequent fetches)", got)
	}
}

func TestManifestFetcher_StaleFallback(t *testing.T) {
	// First request returns valid manifest, subsequent requests fail. The
	// fetcher should return the cached copy rather than erroring once the
	// upstream is down.
	var serveOK int32 = 1
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.LoadInt32(&serveOK) == 1 {
			w.Write([]byte(sampleManifest))
			return
		}
		http.Error(w, "down", http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	// Short TTL so the second Get refetches.
	f := newTestFetcher(srv, time.Nanosecond)
	if _, err := f.Get(context.Background()); err != nil {
		t.Fatalf("first get: %v", err)
	}
	atomic.StoreInt32(&serveOK, 0)
	m, err := f.Get(context.Background())
	if err != nil {
		t.Fatalf("stale fallback get: %v", err)
	}
	if m == nil || len(m.Downloads) == 0 {
		t.Fatal("expected stale cache to be returned")
	}
	if f.LastError() == nil {
		t.Error("LastError should report the upstream failure")
	}
}
