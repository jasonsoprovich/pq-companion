package quarm

import (
	"os"
	"path/filepath"
	"testing"
)

// fixturePath resolves the absolute path of a testdata DLL relative to the
// repo root. Tests skip if the file isn't present so contributors without
// the Windows fixtures can still run the rest of the suite.
func fixturePath(t *testing.T, name string) string {
	t.Helper()
	p := filepath.Join("..", "..", "..", "testdata", name)
	if _, err := os.Stat(p); err != nil {
		t.Skipf("fixture %s missing: %v", name, err)
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		t.Fatalf("resolve fixture path: %v", err)
	}
	return abs
}

func TestInspectDLL_EQGame(t *testing.T) {
	info, err := InspectDLL(fixturePath(t, "eqgame.dll"))
	if err != nil {
		t.Fatalf("inspect eqgame.dll: %v", err)
	}
	if info.MD5 != "176ed594c273283a94d8b20abfb45b99" {
		t.Errorf("MD5 = %q, want 176ed594c273283a94d8b20abfb45b99", info.MD5)
	}
	if info.Size != 310272 {
		t.Errorf("Size = %d, want 310272", info.Size)
	}
	if info.FileVersion != "3.5.6.0" {
		t.Errorf("FileVersion = %q, want 3.5.6.0", info.FileVersion)
	}
	if info.CompiledAt.IsZero() {
		t.Error("CompiledAt is zero")
	}
}

func TestInspectDLL_EQW(t *testing.T) {
	// eqw.dll has no VS_VERSION_INFO resource — we should still extract MD5
	// and PE compile timestamp, and report FileVersion as "".
	info, err := InspectDLL(fixturePath(t, "eqw.dll"))
	if err != nil {
		t.Fatalf("inspect eqw.dll: %v", err)
	}
	if info.MD5 != "d59cc63a5f569a848f6dadbdbeb9b6ed" {
		t.Errorf("MD5 = %q, want d59cc63a5f569a848f6dadbdbeb9b6ed", info.MD5)
	}
	if info.Size != 84480 {
		t.Errorf("Size = %d, want 84480", info.Size)
	}
	if info.FileVersion != "" {
		t.Errorf("FileVersion = %q, want empty (eqw.dll has no resource)", info.FileVersion)
	}
	if info.CompiledAt.IsZero() {
		t.Error("CompiledAt is zero")
	}
}

func TestInspectDLL_Missing(t *testing.T) {
	if _, err := InspectDLL(filepath.Join(t.TempDir(), "nonexistent.dll")); err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}
