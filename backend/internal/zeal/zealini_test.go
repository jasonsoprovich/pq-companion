package zeal

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadExportOnCamp(t *testing.T) {
	cases := []struct {
		name        string
		content     string
		wantFound   bool
		wantEnabled bool
	}{
		{
			name:        "enabled true uppercase",
			content:     "[Zeal]\nExportOnCamp=TRUE\n",
			wantFound:   true,
			wantEnabled: true,
		},
		{
			name:        "enabled mixed case with spaces",
			content:     "[Zeal]\nExportOnCamp = True\n",
			wantFound:   true,
			wantEnabled: true,
		},
		{
			name:        "disabled",
			content:     "[Zeal]\nExportOnCamp=FALSE\n",
			wantFound:   true,
			wantEnabled: false,
		},
		{
			name:        "key absent",
			content:     "[Zeal]\nMapEnabled = TRUE\n",
			wantFound:   false,
			wantEnabled: false,
		},
		{
			name:        "comment ignored",
			content:     "; ExportOnCamp=TRUE\n[Zeal]\nExportOnCamp=FALSE\n",
			wantFound:   true,
			wantEnabled: false,
		},
		{
			name:        "case-insensitive key match",
			content:     "[Zeal]\nexportoncamp=true\n",
			wantFound:   true,
			wantEnabled: true,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, ZealINIFilename), []byte(c.content), 0o644); err != nil {
				t.Fatal(err)
			}
			got := ReadExportOnCamp(dir)
			if got.Found != c.wantFound {
				t.Errorf("Found = %v; want %v", got.Found, c.wantFound)
			}
			if got.Enabled != c.wantEnabled {
				t.Errorf("Enabled = %v; want %v", got.Enabled, c.wantEnabled)
			}
		})
	}
}

func TestReadExportOnCamp_MissingFile(t *testing.T) {
	got := ReadExportOnCamp(t.TempDir())
	if got.Found || got.Enabled {
		t.Errorf("missing zeal.ini should return zero status, got %+v", got)
	}
}

func TestReadExportOnCamp_EmptyPath(t *testing.T) {
	got := ReadExportOnCamp("")
	if got.Found || got.Enabled {
		t.Errorf("empty path should return zero status, got %+v", got)
	}
}

func TestReadExportOnCamp_RealFixture(t *testing.T) {
	// Sanity-check against the testdata zeal.ini from a live install. The
	// file has ExportOnCamp=TRUE under [Zeal]; if either of those changes
	// the parser needs a re-look.
	fixture, err := filepath.Abs(filepath.Join("..", "..", "..", "testdata"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(fixture, ZealINIFilename)); err != nil {
		t.Skipf("testdata zeal.ini not present: %v", err)
	}
	got := ReadExportOnCamp(fixture)
	if !got.Found {
		t.Fatal("expected to find ExportOnCamp in testdata fixture")
	}
	if !got.Enabled {
		t.Errorf("testdata fixture has ExportOnCamp=TRUE; parser read disabled")
	}
}
