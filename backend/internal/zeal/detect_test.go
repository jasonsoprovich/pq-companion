package zeal

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectInstall(t *testing.T) {
	tests := []struct {
		name          string
		files         []string
		wantInstalled bool
		wantEQGame    bool
	}{
		{"empty dir", nil, false, false},
		{"eqgame only", []string{"eqgame.exe"}, false, true},
		{"zeal only", []string{"Zeal.asi"}, true, false},
		{"both", []string{"eqgame.exe", "Zeal.asi"}, true, true},
		{"case insensitive", []string{"EQGAME.EXE", "zeal.asi"}, true, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			for _, f := range tc.files {
				if err := os.WriteFile(filepath.Join(dir, f), []byte("x"), 0o644); err != nil {
					t.Fatalf("write %s: %v", f, err)
				}
			}
			s := DetectInstall(dir)
			if s.Installed != tc.wantInstalled {
				t.Errorf("Installed = %v, want %v", s.Installed, tc.wantInstalled)
			}
			if s.EQGamePresent != tc.wantEQGame {
				t.Errorf("EQGamePresent = %v, want %v", s.EQGamePresent, tc.wantEQGame)
			}
			if tc.wantInstalled && s.ASIPath == "" {
				t.Errorf("ASIPath empty when installed")
			}
		})
	}
}

func TestDetectInstallEmptyPath(t *testing.T) {
	s := DetectInstall("")
	if s.Installed || s.EQGamePresent {
		t.Errorf("empty path should return zero status, got %+v", s)
	}
}

func TestDetectInstallNonexistent(t *testing.T) {
	s := DetectInstall(filepath.Join(os.TempDir(), "does-not-exist-pq-companion-test"))
	if s.Installed || s.EQGamePresent {
		t.Errorf("nonexistent path should return zero status, got %+v", s)
	}
}
