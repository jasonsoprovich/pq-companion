package zeal

import (
	"context"
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
			s := DetectInstall(context.Background(), dir, nil)
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
	s := DetectInstall(context.Background(), "", nil)
	if s.Installed || s.EQGamePresent {
		t.Errorf("empty path should return zero status, got %+v", s)
	}
}

func TestDetectInstallNonexistent(t *testing.T) {
	s := DetectInstall(context.Background(), filepath.Join(os.TempDir(), "does-not-exist-pq-companion-test"), nil)
	if s.Installed || s.EQGamePresent {
		t.Errorf("nonexistent path should return zero status, got %+v", s)
	}
}

func TestDetectInstall_VersionTooOld(t *testing.T) {
	dir := t.TempDir()
	blob := append([]byte("Zeal version: \x00"), []byte("1.3.5\x00")...)
	if err := os.WriteFile(filepath.Join(dir, "Zeal.asi"), blob, 0o644); err != nil {
		t.Fatal(err)
	}
	s := DetectInstall(context.Background(), dir, nil)
	if !s.Installed {
		t.Fatal("expected Installed")
	}
	if s.Version != "1.3.5" {
		t.Errorf("Version = %q; want 1.3.5", s.Version)
	}
	if s.VersionOK {
		t.Errorf("VersionOK true; want false for 1.3.5 < %s", MinSupportedVersion)
	}
	if s.MinVersion != MinSupportedVersion {
		t.Errorf("MinVersion = %q; want %q", s.MinVersion, MinSupportedVersion)
	}
}

func TestDetectInstall_VersionOK(t *testing.T) {
	dir := t.TempDir()
	blob := append([]byte("Zeal version: \x00"), []byte("1.4.2\x00")...)
	if err := os.WriteFile(filepath.Join(dir, "Zeal.asi"), blob, 0o644); err != nil {
		t.Fatal(err)
	}
	s := DetectInstall(context.Background(), dir, nil)
	if s.Version != "1.4.2" {
		t.Errorf("Version = %q; want 1.4.2", s.Version)
	}
	if !s.VersionOK {
		t.Error("VersionOK false; want true for 1.4.2 >= min")
	}
}

func TestDetectInstall_VersionUnknownDoesNotWarn(t *testing.T) {
	// Older Zeal binaries or stripped builds may not contain the literal we
	// scan for. Treat as unknown — never trigger the warning banner on a
	// detection failure.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "Zeal.asi"), []byte("opaque-content"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := DetectInstall(context.Background(), dir, nil)
	if !s.Installed {
		t.Fatal("expected Installed")
	}
	if s.Version != "" {
		t.Errorf("Version = %q; want empty", s.Version)
	}
	if !s.VersionOK {
		t.Error("VersionOK false on unknown version; should be true to avoid false alarm")
	}
}
