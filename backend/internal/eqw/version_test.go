package eqw

import (
	"os"
	"path/filepath"
	"testing"
)

// TestReadInstalledVersion parses the build stamp out of the real eqw.dll
// fixture. testdata/ is gitignored, so skip when it isn't present (e.g. CI).
func TestReadInstalledVersion(t *testing.T) {
	path := filepath.Join("..", "..", "..", "testdata", "TAKPv22", "eqw.dll")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("testdata fixture %s not present", path)
	}
	v, err := ReadInstalledVersion(path)
	if err != nil {
		t.Fatalf("ReadInstalledVersion: %v", err)
	}
	if v != "1.0.1" {
		t.Fatalf("version = %q, want %q", v, "1.0.1")
	}
}

func TestReadInstalledVersionBlankPath(t *testing.T) {
	v, err := ReadInstalledVersion("")
	if err != nil || v != "" {
		t.Fatalf("blank path: got (%q, %v), want (\"\", nil)", v, err)
	}
}

func TestBuildStampPatternRejectsBareVersion(t *testing.T) {
	// A statically linked lib's bare "1.1.3" literal (no build-stamp paren)
	// must not be mistaken for the EQW version.
	if m := buildStampPattern.FindSubmatch([]byte("libpng\x001.1.3\x00")); m != nil {
		t.Fatalf("matched a bare version literal: %q", m[1])
	}
	if m := buildStampPattern.FindSubmatch([]byte("1.0.1 (Jan 20 2026 22:09:32)")); m == nil {
		t.Fatal("did not match a real build stamp")
	}
}
