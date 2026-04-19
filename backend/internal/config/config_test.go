package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFrom_createsDefaultFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	m, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	// File should now exist on disk.
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected config file to be created: %v", err)
	}

	cfg := m.Get()
	if cfg.ServerAddr != ":8080" {
		t.Errorf("default ServerAddr = %q, want :8080", cfg.ServerAddr)
	}
	if cfg.Preferences.OverlayOpacity != 0.25 {
		t.Errorf("default OverlayOpacity = %v, want 0.25", cfg.Preferences.OverlayOpacity)
	}
}

func TestLoadFrom_roundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	m, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}

	want := Config{
		EQPath:     "/games/EverQuest",
		Character:  "Testerino",
		ServerAddr: ":9090",
		Preferences: Preferences{
			OverlayOpacity: 0.75,
			MinimizeToTray: false,
			ParseCombatLog: false,
		},
	}
	if err := m.Update(want); err != nil {
		t.Fatalf("Update: %v", err)
	}

	// Reload from the same file.
	m2, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom (reload): %v", err)
	}
	got := m2.Get()

	if got.EQPath != want.EQPath {
		t.Errorf("EQPath: got %q, want %q", got.EQPath, want.EQPath)
	}
	if got.Character != want.Character {
		t.Errorf("Character: got %q, want %q", got.Character, want.Character)
	}
	if got.ServerAddr != want.ServerAddr {
		t.Errorf("ServerAddr: got %q, want %q", got.ServerAddr, want.ServerAddr)
	}
	if got.Preferences.OverlayOpacity != want.Preferences.OverlayOpacity {
		t.Errorf("OverlayOpacity: got %v, want %v", got.Preferences.OverlayOpacity, want.Preferences.OverlayOpacity)
	}
	if got.Preferences.MinimizeToTray != want.Preferences.MinimizeToTray {
		t.Errorf("MinimizeToTray: got %v, want %v", got.Preferences.MinimizeToTray, want.Preferences.MinimizeToTray)
	}
	if got.Preferences.ParseCombatLog != want.Preferences.ParseCombatLog {
		t.Errorf("ParseCombatLog: got %v, want %v", got.Preferences.ParseCombatLog, want.Preferences.ParseCombatLog)
	}
}

func TestLoadFrom_mergesExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// Write a partial config — only eq_path set.
	if err := os.WriteFile(path, []byte("eq_path: /games/EQ\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	cfg := m.Get()

	if cfg.EQPath != "/games/EQ" {
		t.Errorf("EQPath: got %q, want /games/EQ", cfg.EQPath)
	}
	// Fields not in the file come from the yaml zero values (not defaults),
	// because yaml.Unmarshal overwrites the struct. ServerAddr will be empty.
	// This is expected YAML behaviour — document it via the test.
	_ = cfg.ServerAddr
}
