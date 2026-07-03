package logparser

import (
	"os"
	"path/filepath"
	"testing"
)

// writeLog creates an empty eqlog file for the given character name in dir.
func writeLog(t *testing.T, dir, name string) {
	t.Helper()
	p := filepath.Join(dir, "eqlog_"+name+"_pq.proj.txt")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatalf("write %s: %v", p, err)
	}
}

func TestDiscoverCharactersSkipsReserved(t *testing.T) {
	dir := t.TempDir()
	writeLog(t, dir, "Sandrian")
	writeLog(t, dir, "Everquest") // login/server-select placeholder log
	writeLog(t, dir, "everquest") // case variant

	got := DiscoverCharacters(dir)
	if len(got) != 1 {
		t.Fatalf("expected 1 real character, got %d: %+v", len(got), got)
	}
	if got[0].Name != "Sandrian" {
		t.Fatalf("expected Sandrian, got %q", got[0].Name)
	}
}

func TestResolveActiveCharacterIgnoresReserved(t *testing.T) {
	dir := t.TempDir()
	writeLog(t, dir, "Sandrian")
	// Make the reserved log the most recently modified — it must still be
	// ignored so active-character resolution never lands on "Everquest".
	writeLog(t, dir, "Everquest")

	if got := ResolveActiveCharacter(dir); got != "Sandrian" {
		t.Fatalf("expected Sandrian, got %q", got)
	}
}

func TestDiscoverCharactersEmptyPath(t *testing.T) {
	if got := DiscoverCharacters(""); len(got) != 0 {
		t.Fatalf("expected empty, got %+v", got)
	}
}
