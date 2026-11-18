package api

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestProbeLog(t *testing.T) {
	dir := t.TempDir()

	if got := probeLog("", ""); got.Path != "" || got.Exists {
		t.Fatalf("blank path: %+v", got)
	}
	if got := probeLog(filepath.Join(dir, "missing"), "Osui"); got.Path != "" {
		t.Fatalf("missing dir: %+v", got)
	}

	// Named character with no log yet.
	got := probeLog(dir, "Osui")
	if got.Character != "Osui" || got.Exists || got.Path != filepath.Join(dir, "eqlog_Osui_pq.proj.txt") {
		t.Fatalf("no log: %+v", got)
	}

	// Two logs; blank character resolves to the newest.
	old := filepath.Join(dir, "eqlog_Alt_pq.proj.txt")
	cur := filepath.Join(dir, "eqlog_Osui_pq.proj.txt")
	if err := os.WriteFile(old, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cur, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatal(err)
	}
	got = probeLog(dir, "")
	if got.Character != "Osui" || !got.Exists || got.SizeBytes != 5 || got.LastWriteAt == "" {
		t.Fatalf("auto-detect: %+v", got)
	}
}
