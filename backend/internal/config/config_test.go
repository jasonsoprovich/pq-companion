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
	if cfg.Preferences.MasterVolume != 100 {
		t.Errorf("default MasterVolume = %d, want 100", cfg.Preferences.MasterVolume)
	}
}

// Existing configs predating MasterVolume should be backfilled to 100 (no
// dampening) on next load — otherwise the missing field would unmarshal to 0
// and silently mute every alert.
func TestLoadFrom_BackfillsMasterVolumeDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	const old = `eq_path: /games/EQ
preferences:
  overlay_opacity: 0.5
  minimize_to_tray: true
`
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	if got := m.Get().Preferences.MasterVolume; got != 100 {
		t.Errorf("MasterVolume: got %d, want 100 (backfilled default)", got)
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

// applyDefaults backfills newly-added fields when an existing config file
// pre-dates them. We rely on this so users don't need to manually edit
// ~/.pq-companion/config.yaml after upgrading.
func TestLoadFrom_BackfillsSpellTimerDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// Write a config from before SpellTimer existed.
	const old = `eq_path: /games/EQ
character: Osui
server_addr: :8080
`
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	cfg := m.Get()

	if cfg.SpellTimer.TrackingScope != TrackingScopeCastByMe {
		t.Errorf("TrackingScope: got %q, want %q (default)", cfg.SpellTimer.TrackingScope, TrackingScopeCastByMe)
	}
	if !cfg.SpellTimer.CastByMeMigrationDone {
		t.Error("CastByMeMigrationDone should be true after backfill")
	}
	// Threshold defaults are 0 by design (always show); just confirm we
	// didn't accidentally set them to anything else.
	if cfg.SpellTimer.BuffDisplayThresholdSecs != 0 {
		t.Errorf("BuffDisplayThresholdSecs: got %d, want 0", cfg.SpellTimer.BuffDisplayThresholdSecs)
	}
	if cfg.SpellTimer.DetrimDisplayThresholdSecs != 0 {
		t.Errorf("DetrimDisplayThresholdSecs: got %d, want 0", cfg.SpellTimer.DetrimDisplayThresholdSecs)
	}
}

// One-time migration rewrites pre-existing "anyone" configs to "cast_by_me"
// the first time a post-upgrade load happens, then leaves the user's choice
// alone forever after.
func TestLoadFrom_MigratesAnyoneToCastByMe_Once(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// Pre-migration config: explicitly set to "anyone", no migration flag.
	const old = `eq_path: /games/EQ
spell_timer:
  tracking_scope: anyone
`
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	cfg := m.Get()

	if cfg.SpellTimer.TrackingScope != TrackingScopeCastByMe {
		t.Errorf("first load: got %q, want migration to %q",
			cfg.SpellTimer.TrackingScope, TrackingScopeCastByMe)
	}
	if !cfg.SpellTimer.CastByMeMigrationDone {
		t.Error("CastByMeMigrationDone should be true after migration")
	}

	// User then explicitly chooses "anyone" again.
	cfg.SpellTimer.TrackingScope = TrackingScopeAnyone
	if err := m.Update(cfg); err != nil {
		t.Fatalf("Update: %v", err)
	}

	// Reload — choice must stick because the migration flag is set.
	m2, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom (reload): %v", err)
	}
	if got := m2.Get().SpellTimer.TrackingScope; got != TrackingScopeAnyone {
		t.Errorf("reload after explicit anyone: got %q, want %q (migration must be one-shot)",
			got, TrackingScopeAnyone)
	}
}
