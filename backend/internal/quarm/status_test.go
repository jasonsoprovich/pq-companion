package quarm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// testdataDir is where the eqgame.dll / eqw.dll fixtures live. We treat that
// directory as a stand-in EQ install for these tests.
func testdataDir(t *testing.T) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join("..", "..", "..", "testdata"))
	if err != nil {
		t.Fatalf("resolve testdata: %v", err)
	}
	if _, err := os.Stat(filepath.Join(p, "eqgame.dll")); err != nil {
		t.Skipf("testdata/eqgame.dll missing: %v", err)
	}
	return p
}

// manifestWithLocalEQGame builds a manifest YAML whose eqgame.dll entry
// exactly matches the testdata fixture. Used to assert StatusMatch.
const manifestMatchingLocal = `version: test-match
downloadprefix: https://example.com/
downloads:
- name: eqgame.dll
  md5: 176ed594c273283a94d8b20abfb45b99
  date: "20260325"
  size: 310272
`

// manifestStale is what we'd get if Quarm's manifest is older than the local
// copy (the real-world situation today).
const manifestStale = `version: test-stale
downloadprefix: https://example.com/
downloads:
- name: eqgame.dll
  md5: 0d7fbe37f9478f51aa983eae07546d81
  date: "20250215"
  size: 280064
`

func newServer(t *testing.T, body string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(body))
	}))
}

func TestStatus_Match(t *testing.T) {
	eqPath := testdataDir(t)
	srv := newServer(t, manifestMatchingLocal)
	defer srv.Close()
	f := newTestFetcher(srv, 0)

	s := Status(context.Background(), eqPath, f)
	if s.ManifestVersion != "test-match" {
		t.Errorf("ManifestVersion = %q", s.ManifestVersion)
	}
	if len(s.Files) != 1 {
		t.Fatalf("Files = %d, want 1", len(s.Files))
	}
	eqgame := s.Files[0]
	if eqgame.Name != "eqgame.dll" {
		t.Fatalf("first file = %q, want eqgame.dll", eqgame.Name)
	}
	if eqgame.Status != StatusMatch {
		t.Errorf("eqgame status = %q, want %q (reason=%q)", eqgame.Status, StatusMatch, eqgame.Reason)
	}
	if eqgame.Manifest == nil || eqgame.Local == nil {
		t.Error("expected both Local and Manifest populated for eqgame.dll")
	}
}

func TestStatus_Mismatch(t *testing.T) {
	eqPath := testdataDir(t)
	srv := newServer(t, manifestStale)
	defer srv.Close()
	f := newTestFetcher(srv, 0)

	s := Status(context.Background(), eqPath, f)
	if s.Files[0].Status != StatusMismatch {
		t.Errorf("eqgame status = %q, want %q", s.Files[0].Status, StatusMismatch)
	}
}

// When the manifest's MD5 differs from the local file but the
// FileVersion strings match (because the manifest's downloadprefix
// resolves to a DLL with the same product version), Status should
// report Match — not Mismatch — so users who ran the official Quarm
// patcher aren't told they're out of date when they're actually fine.
func TestStatus_VersionMatchesEvenWhenMD5Differs(t *testing.T) {
	eqPath := testdataDir(t)
	dllBytes, err := os.ReadFile(filepath.Join(eqPath, "eqgame.dll"))
	if err != nil {
		t.Fatalf("read fixture DLL: %v", err)
	}

	// Serve the manifest at /manifest.yml and the same DLL bytes the
	// local fixture has at /eqgame.dll. The manifest declares a wrong
	// MD5/size so the byte comparison fails, forcing the version path.
	var srv *httptest.Server
	mux := http.NewServeMux()
	mux.HandleFunc("/eqgame.dll", func(w http.ResponseWriter, r *http.Request) {
		w.Write(dllBytes)
	})
	mux.HandleFunc("/manifest.yml", func(w http.ResponseWriter, r *http.Request) {
		body := `version: test-version-match
downloadprefix: ` + srv.URL + `/
downloads:
- name: eqgame.dll
  md5: deadbeefdeadbeefdeadbeefdeadbeef
  date: "20260101"
  size: 1
`
		w.Write([]byte(body))
	})
	srv = httptest.NewServer(mux)
	defer srv.Close()

	f := &ManifestFetcher{
		url:    srv.URL + "/manifest.yml",
		ttl:    0,
		client: srv.Client(),
	}

	s := Status(context.Background(), eqPath, f)
	if len(s.Files) != 1 || s.Files[0].Name != "eqgame.dll" {
		t.Fatalf("unexpected files: %+v", s.Files)
	}
	got := s.Files[0]
	if got.Status != StatusMatch {
		t.Errorf("status = %q, want %q (reason=%q)", got.Status, StatusMatch, got.Reason)
	}
	if got.Manifest == nil || got.Manifest.RefFileVersion == "" {
		t.Errorf("expected manifest entry with non-empty RefFileVersion, got %+v", got.Manifest)
	}
	if got.Reason == "" {
		t.Error("expected a Reason hint when match is via FileVersion fallback")
	}
}

func TestStatus_NoEQPath(t *testing.T) {
	srv := newServer(t, manifestMatchingLocal)
	defer srv.Close()
	f := newTestFetcher(srv, 0)

	s := Status(context.Background(), "", f)
	for _, fs := range s.Files {
		if fs.Status != StatusUnknown {
			t.Errorf("%s status = %q, want unknown when EQ path empty", fs.Name, fs.Status)
		}
		if fs.Local != nil {
			t.Errorf("%s should have no Local info when EQ path empty", fs.Name)
		}
	}
}

func TestStatus_MissingFile(t *testing.T) {
	dir := t.TempDir()
	srv := newServer(t, manifestMatchingLocal)
	defer srv.Close()
	f := newTestFetcher(srv, 0)

	s := Status(context.Background(), dir, f)
	if s.Files[0].Status != StatusMissing {
		t.Errorf("eqgame status = %q, want %q when file absent", s.Files[0].Status, StatusMissing)
	}
}

func TestStatus_ManifestUnreachable(t *testing.T) {
	eqPath := testdataDir(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "down", http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	f := newTestFetcher(srv, 0)

	s := Status(context.Background(), eqPath, f)
	if s.ManifestError == "" {
		t.Error("expected ManifestError populated when upstream is down")
	}
	if s.Files[0].Status != StatusUnknown {
		t.Errorf("eqgame status = %q, want unknown when manifest unreachable", s.Files[0].Status)
	}
	// Local info should still be populated even when manifest fails.
	if s.Files[0].Local == nil {
		t.Error("Local info should still be populated when manifest fails")
	}
}
