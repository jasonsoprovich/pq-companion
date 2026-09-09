package logparser

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"testing"
)

// writeArchiveZip writes a .bak.zip holding one entry (named like the live
// log) with the given content.
func writeArchiveZip(t *testing.T, path, entryName, content string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create %s: %v", path, err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	w, err := zw.Create(entryName)
	if err != nil {
		t.Fatalf("zip create entry: %v", err)
	}
	if _, err := io.WriteString(w, content); err != nil {
		t.Fatalf("zip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
}

func TestDiscoverArchives(t *testing.T) {
	dir := t.TempDir()
	// Live log — must NOT be picked up as an archive.
	if err := os.WriteFile(filepath.Join(dir, "eqlog_Grok_pq.proj.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A different character's archive — must be ignored.
	writeArchiveZip(t, filepath.Join(dir, "eqlog_Other_pq.proj.2026-05-01.bak.zip"), "eqlog_Other_pq.proj.txt", "y")
	// Grok's archives, deliberately created out of date order.
	writeArchiveZip(t, filepath.Join(dir, "eqlog_Grok_pq.proj.2026-06-15.bak.zip"), "eqlog_Grok_pq.proj.txt", "june content")
	writeArchiveZip(t, filepath.Join(dir, "eqlog_Grok_pq.proj.2026-03-01.bak.zip"), "eqlog_Grok_pq.proj.txt", "march")
	// Legacy uncompressed archive.
	if err := os.WriteFile(filepath.Join(dir, "eqlog_Grok_pq.proj.2026-04-20.bak.txt"), []byte("april legacy"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := DiscoverArchives(dir, "Grok")
	if len(got) != 3 {
		t.Fatalf("expected 3 archives for Grok, got %d: %+v", len(got), got)
	}
	wantDates := []string{"2026-03-01", "2026-04-20", "2026-06-15"}
	for i, w := range wantDates {
		if d := got[i].ArchivedAt.Format("2006-01-02"); d != w {
			t.Errorf("archive[%d] date = %s, want %s (oldest-first ordering)", i, d, w)
		}
	}
	if got[1].Compressed {
		t.Errorf("legacy .bak.txt should not be flagged Compressed")
	}
	if !got[0].Compressed {
		t.Errorf(".bak.zip should be flagged Compressed")
	}
	if got[1].Bytes != int64(len("april legacy")) {
		t.Errorf("legacy archive Bytes = %d, want %d", got[1].Bytes, len("april legacy"))
	}
	if got[0].Bytes != int64(len("march")) {
		t.Errorf("zip archive Bytes (uncompressed) = %d, want %d", got[0].Bytes, len("march"))
	}
}

func TestDiscoverArchivesEmpty(t *testing.T) {
	if got := DiscoverArchives("", "Grok"); got != nil {
		t.Errorf("empty eqPath: got %+v, want nil", got)
	}
	if got := DiscoverArchives(t.TempDir(), "Grok"); got != nil {
		t.Errorf("no archives: got %+v, want nil", got)
	}
}

func TestOpenArchive(t *testing.T) {
	dir := t.TempDir()

	zipPath := filepath.Join(dir, "eqlog_Grok_pq.proj.2026-03-01.bak.zip")
	writeArchiveZip(t, zipPath, "eqlog_Grok_pq.proj.txt", "line one\nline two\n")
	rc, err := OpenArchive(zipPath)
	if err != nil {
		t.Fatalf("OpenArchive zip: %v", err)
	}
	b, _ := io.ReadAll(rc)
	rc.Close()
	if string(b) != "line one\nline two\n" {
		t.Errorf("zip entry content = %q", string(b))
	}

	txtPath := filepath.Join(dir, "eqlog_Grok_pq.proj.2026-04-20.bak.txt")
	if err := os.WriteFile(txtPath, []byte("plain content"), 0o644); err != nil {
		t.Fatal(err)
	}
	rc2, err := OpenArchive(txtPath)
	if err != nil {
		t.Fatalf("OpenArchive txt: %v", err)
	}
	b2, _ := io.ReadAll(rc2)
	rc2.Close()
	if string(b2) != "plain content" {
		t.Errorf("plain content = %q", string(b2))
	}
}
