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

func TestStatus_MatchAndUnknown(t *testing.T) {
	eqPath := testdataDir(t)
	srv := newServer(t, manifestMatchingLocal)
	defer srv.Close()
	f := newTestFetcher(srv, 0)

	s := Status(context.Background(), eqPath, f)
	if s.ManifestVersion != "test-match" {
		t.Errorf("ManifestVersion = %q", s.ManifestVersion)
	}
	if len(s.Files) != 2 {
		t.Fatalf("Files = %d, want 2", len(s.Files))
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

	eqw := s.Files[1]
	if eqw.Name != "eqw.dll" {
		t.Fatalf("second file = %q, want eqw.dll", eqw.Name)
	}
	if eqw.Status != StatusUnknown {
		t.Errorf("eqw status = %q, want %q (eqw is not in manifest)", eqw.Status, StatusUnknown)
	}
	if eqw.Local == nil {
		t.Error("expected eqw.dll Local info populated for local-only display")
	}
	if eqw.Manifest != nil {
		t.Error("eqw.dll should have no Manifest entry")
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
