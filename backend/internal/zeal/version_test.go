package zeal

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.4.0", "1.4.0", 0},
		{"1.4.0", "1.4.1", -1},
		{"1.4.1", "1.4.0", 1},
		{"1.3.9", "1.4.0", -1},
		{"2.0.0", "1.99.99", 1},
		{"1.4", "1.4.0", 0},
		{"1.4.0.0", "1.4.0", 0},
		{"1.4.0.1", "1.4.0", 1},
		{"", "1.0.0", -1},
		{"  1.4.2 ", "1.4.2", 0},
	}
	for _, c := range cases {
		if got := CompareVersions(c.a, c.b); got != c.want {
			t.Errorf("CompareVersions(%q, %q) = %d; want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestReadInstalledVersion_Anchored(t *testing.T) {
	// Simulate a .rdata blob: anchor literal, padding, version literal, more
	// padding. The parser should locate the version via the anchor proximity.
	blob := []byte{}
	blob = append(blob, []byte("Zeal version: ")...)
	blob = append(blob, 0)
	blob = append(blob, make([]byte, 128)...)
	blob = append(blob, []byte("1.4.2")...)
	blob = append(blob, 0)
	blob = append(blob, []byte("something else")...)
	blob = append(blob, 0)

	path := filepath.Join(t.TempDir(), "Zeal.asi")
	if err := os.WriteFile(path, blob, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadInstalledVersion(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "1.4.2" {
		t.Errorf("ReadInstalledVersion = %q; want %q", got, "1.4.2")
	}
}

func TestReadInstalledVersion_CrashHandlerAnchor(t *testing.T) {
	// The real Zeal.asi keeps ZEAL_VERSION next to the crash-handler label
	// "Zeal Version: " (capital V), with the version literal sitting just
	// *before* the anchor. Verify that layout resolves.
	blob := []byte{}
	blob = append(blob, []byte("crash_reason.txt")...)
	blob = append(blob, 0)
	blob = append(blob, []byte("1.4.2")...)
	blob = append(blob, 0)
	blob = append(blob, []byte("No exception information")...)
	blob = append(blob, 0)
	blob = append(blob, []byte("Zeal Version: ")...)
	blob = append(blob, 0)

	path := filepath.Join(t.TempDir(), "Zeal.asi")
	if err := os.WriteFile(path, blob, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadInstalledVersion(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "1.4.2" {
		t.Errorf("ReadInstalledVersion = %q; want %q", got, "1.4.2")
	}
}

func TestReadInstalledVersion_IgnoresUnanchoredFalsePositive(t *testing.T) {
	// Regression: Zeal.asi statically links libpng, whose "1.1.3" version
	// literal has no Zeal anchor near it. An anchored 1.4.2 must win over an
	// unanchored 1.1.3 that appears earlier in the file.
	blob := []byte{}
	blob = append(blob, []byte("Incompatible libpng version")...)
	blob = append(blob, 0)
	blob = append(blob, []byte("1.1.3")...) // libpng version — no anchor
	blob = append(blob, 0)
	blob = append(blob, make([]byte, 8192)...) // push it outside any window
	blob = append(blob, []byte("1.4.2")...)
	blob = append(blob, 0)
	blob = append(blob, []byte("Zeal Version: ")...)
	blob = append(blob, 0)

	path := filepath.Join(t.TempDir(), "Zeal.asi")
	if err := os.WriteFile(path, blob, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadInstalledVersion(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "1.4.2" {
		t.Errorf("ReadInstalledVersion = %q; want %q (libpng false positive leaked through)", got, "1.4.2")
	}
}

func TestReadInstalledVersion_NoAnchorIsUnknown(t *testing.T) {
	// A version-shaped literal with no anchor nearby must NOT be reported:
	// it's almost certainly a statically-linked dependency's version, and a
	// wrong version would false-alarm the "update Zeal" banner.
	blob := []byte{0, 0, 0}
	blob = append(blob, []byte("1.4.0")...)
	blob = append(blob, 0)

	path := filepath.Join(t.TempDir(), "Zeal.asi")
	if err := os.WriteFile(path, blob, 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadInstalledVersion(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("ReadInstalledVersion = %q; want empty (unanchored literal)", got)
	}
}

func TestReadInstalledVersion_NoMatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Zeal.asi")
	if err := os.WriteFile(path, []byte("no version string here at all"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := ReadInstalledVersion(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("ReadInstalledVersion = %q; want empty", got)
	}
}

func TestReadInstalledVersion_EmptyPath(t *testing.T) {
	got, err := ReadInstalledVersion("")
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("ReadInstalledVersion(\"\") = %q; want empty", got)
	}
}
