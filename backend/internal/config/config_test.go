package config

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
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
	if cfg.ServerAddr != ":17654" {
		t.Errorf("default ServerAddr = %q, want :17654", cfg.ServerAddr)
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
			// -1 is the "follow the global setting" sentinel and must survive.
			OverlayOpacities: map[string]float64{"buffTimer": 1, "dps": -1},
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
	if o := got.Preferences.OverlayOpacities; o["buffTimer"] != 1 || o["dps"] != -1 {
		t.Errorf("OverlayOpacities: got %v, want buffTimer=1 dps=-1", o)
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

// The Faction NPC-overlay section was added after the other five. A config
// written with the original sections populated but no faction field must have
// faction turned on exactly once, and the user's later choice to disable it
// must survive subsequent loads.
func TestLoadFrom_MigratesNPCFactionSection_Once(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// Pre-faction config: the original five sections set, no faction key and no
	// migration flag.
	const old = `eq_path: /games/EQ
preferences:
  npc_overlay_dashboard_sections:
    identity: true
    combat: true
    resists: true
    attributes: true
    special_abilities: true
  npc_overlay_popout_sections:
    identity: true
    combat: false
    resists: true
    attributes: true
    special_abilities: true
`
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	cfg := m.Get()

	if !cfg.Preferences.NPCOverlayDashboardSections.Faction {
		t.Error("dashboard Faction should be on after migration")
	}
	if !cfg.Preferences.NPCOverlayPopoutSections.Faction {
		t.Error("popout Faction should be on after migration")
	}
	if !cfg.Preferences.NPCFactionSectionMigrationDone {
		t.Error("NPCFactionSectionMigrationDone should be true after migration")
	}
	// Unrelated toggles must be preserved; the later sections-to-hidden
	// migration carries popout Combat=false over as a hidden key.
	if !slices.Contains(cfg.Preferences.NPCOverlayHiddenItems, "popout:section.combat") {
		t.Errorf("popout combat not carried over: hidden = %v", cfg.Preferences.NPCOverlayHiddenItems)
	}

	// User disables faction on the popout, then we reload.
	cfg.Preferences.NPCOverlayPopoutSections.Faction = false
	if err := m.Update(cfg); err != nil {
		t.Fatalf("Update: %v", err)
	}
	m2, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom (reload): %v", err)
	}
	if m2.Get().Preferences.NPCOverlayPopoutSections.Faction {
		t.Error("reload after disabling faction: must stay off (migration is one-shot)")
	}
}

// The Behavior NPC-overlay section (aggro/assist radius, attack delay) was
// added after the others. A config written without the behavior field must
// have it turned on exactly once, and a later choice to disable it must
// survive subsequent loads.
func TestLoadFrom_MigratesNPCBehaviorSection_Once(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	const old = `eq_path: /games/EQ
preferences:
  npc_overlay_dashboard_sections:
    identity: true
    combat: true
    resists: true
    attributes: true
    special_abilities: true
    faction: true
  npc_overlay_popout_sections:
    identity: true
    combat: false
    resists: true
    attributes: true
    special_abilities: true
    faction: true
`
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	cfg := m.Get()

	if !cfg.Preferences.NPCOverlayDashboardSections.Behavior {
		t.Error("dashboard Behavior should be on after migration")
	}
	if !cfg.Preferences.NPCOverlayPopoutSections.Behavior {
		t.Error("popout Behavior should be on after migration")
	}
	if !cfg.Preferences.NPCBehaviorSectionMigrationDone {
		t.Error("NPCBehaviorSectionMigrationDone should be true after migration")
	}
	// Unrelated toggles must be preserved; the later sections-to-hidden
	// migration carries popout Combat=false over as a hidden key.
	if !slices.Contains(cfg.Preferences.NPCOverlayHiddenItems, "popout:section.combat") {
		t.Errorf("popout combat not carried over: hidden = %v", cfg.Preferences.NPCOverlayHiddenItems)
	}

	// User disables Behavior on the popout, then we reload.
	cfg.Preferences.NPCOverlayPopoutSections.Behavior = false
	if err := m.Update(cfg); err != nil {
		t.Fatalf("Update: %v", err)
	}
	m2, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom (reload): %v", err)
	}
	if m2.Get().Preferences.NPCOverlayPopoutSections.Behavior {
		t.Error("reload after disabling behavior: must stay off (migration is one-shot)")
	}
}

func TestLoadFrom_MigratesRespawnTTSSpelling(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// A config that enabled the respawn alert before the pronunciation fix has
	// the exact legacy template saved verbatim.
	const old = `eq_path: /games/EQ
preferences:
  respawn_alert:
    enabled: true
    type: text_to_speech
    tts_template: '{npc} has respawned'
`
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	cfg := m.Get()
	if cfg.Preferences.RespawnAlert == nil {
		t.Fatal("respawn_alert should be present")
	}
	if got := cfg.Preferences.RespawnAlert.TTSTemplate; got != DefaultRespawnTTSTemplate {
		t.Errorf("legacy template should migrate to %q, got %q", DefaultRespawnTTSTemplate, got)
	}

	// A customized template must be left untouched.
	const custom = `eq_path: /games/EQ
preferences:
  respawn_alert:
    enabled: true
    type: text_to_speech
    tts_template: '{npc} is back, watch out'
`
	if err := os.WriteFile(path, []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}
	m2, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom (custom): %v", err)
	}
	if got := m2.Get().Preferences.RespawnAlert.TTSTemplate; got != "{npc} is back, watch out" {
		t.Errorf("custom template must be preserved, got %q", got)
	}
}

// Configs snapshot DefaultCHChainPattern at save time, so when the shipped
// default improves, a user who never customized theirs would stay pinned to
// the old one. applyDefaults upgrades exact matches of any legacy default to
// the current default — and leaves hand-customized patterns alone.
func TestLoadFrom_UpgradesLegacyCHChainPattern(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// v1 default, as saved by configs from when the feature first shipped.
	const old = `eq_path: /games/EQ
ch_chain:
  enabled: true
  pattern: ^(?P<caster>\w+) tells the raid, '-+\s*0*(?P<chainnum>\d+)\s*-+\s*CH\s+(?P<target>\w+)
  interval_secs: 6
`
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	cfg := m.Get()
	if cfg.CHChain.Pattern != DefaultCHChainPattern {
		t.Errorf("legacy default not upgraded:\ngot  %q\nwant %q", cfg.CHChain.Pattern, DefaultCHChainPattern)
	}
	// The secondary pattern backfills alongside it.
	if cfg.CHChain.SecondaryPattern != DefaultCHChainSecondaryPattern {
		t.Errorf("SecondaryPattern: got %q, want letters-only default", cfg.CHChain.SecondaryPattern)
	}

	// A customized pattern must never be rewritten.
	custom := `^(?P<caster>\w+) yells, '(?P<chainnum>\d+) CH (?P<target>\w+)'`
	cfg.CHChain.Pattern = custom
	if err := m.Update(cfg); err != nil {
		t.Fatalf("Update: %v", err)
	}
	m2, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom (reload): %v", err)
	}
	if got := m2.Get().CHChain.Pattern; got != custom {
		t.Errorf("custom pattern was rewritten: got %q, want %q", got, custom)
	}
}

// TestLoadFrom_PossibleMissMigratesOnForUpgradingUsers checks that a config
// predating the possible-miss feature entirely gets it turned on once, and
// that a user's explicit opt-out survives a reload once the migration marker
// is set — the migration must never re-flip an explicit choice back on.
func TestLoadFrom_PossibleMissMigratesOnForUpgradingUsers(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	// Predates the possible-miss field entirely (no possible_miss_* keys).
	const old = `eq_path: /games/EQ
ch_chain:
  enabled: true
  interval_secs: 6
`
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	cfg := m.Get()
	if !cfg.CHChain.PossibleMissEnabled || !cfg.CHChain.PossibleMissMigrated {
		t.Errorf("possible-miss detection should migrate on: enabled=%v migrated=%v",
			cfg.CHChain.PossibleMissEnabled, cfg.CHChain.PossibleMissMigrated)
	}

	// An explicit opt-out must survive a reload — the migration marker is
	// now set, so applyDefaults must not run the backfill again.
	cfg.CHChain.PossibleMissEnabled = false
	if err := m.Update(cfg); err != nil {
		t.Fatalf("Update: %v", err)
	}
	m2, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom (reload): %v", err)
	}
	if m2.Get().CHChain.PossibleMissEnabled {
		t.Error("explicit opt-out of possible-miss detection was reverted by a reload")
	}
}

// The Distance NPC-overlay section was added after the others: an older config
// gets it turned on once, and a missing cast range is normalised to 200.
func TestLoadFrom_MigratesNPCDistanceSection_Once(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")

	const old = `eq_path: /games/EQ
preferences:
  npc_behavior_section_migration_done: true
  npc_overlay_dashboard_sections:
    identity: true
  npc_overlay_popout_sections:
    identity: true
`
	if err := os.WriteFile(path, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	cfg := m.Get()
	if !cfg.Preferences.NPCOverlayDashboardSections.Distance || !cfg.Preferences.NPCOverlayPopoutSections.Distance {
		t.Error("Distance should be on after migration")
	}
	if cfg.Preferences.NPCOverlayCastRange != 200 {
		t.Errorf("NPCOverlayCastRange = %d, want 200", cfg.Preferences.NPCOverlayCastRange)
	}

	// An explicit opt-out survives a reload.
	cfg.Preferences.NPCOverlayPopoutSections.Distance = false
	if err := m.Update(cfg); err != nil {
		t.Fatalf("Update: %v", err)
	}
	m2, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if m2.Get().Preferences.NPCOverlayPopoutSections.Distance {
		t.Error("explicit Distance=false was overridden on reload")
	}
}

func TestDetrimSpellOverride(t *testing.T) {
	p := Preferences{DetrimSpellOverrides: map[string]string{
		"Tashanian": DetrimOverrideMute,
		"Mesmerize": DetrimOverrideHide,
		"Enstill":   "", // blank = no override
	}}
	tests := []struct {
		spell string
		want  string
	}{
		{"Tashanian", DetrimOverrideMute},
		{"tashanian", DetrimOverrideMute}, // case-insensitive
		{"Mesmerize", DetrimOverrideHide},
		{"Enstill", ""},
		{"Slow", ""},
	}
	for _, tt := range tests {
		if got := p.DetrimSpellOverride(tt.spell); got != tt.want {
			t.Errorf("DetrimSpellOverride(%q) = %q, want %q", tt.spell, got, tt.want)
		}
	}
	if got := (Preferences{}).DetrimSpellOverride("Tashanian"); got != "" {
		t.Errorf("nil map override = %q, want empty", got)
	}
}

// A character with an empty override list must survive a save/reload: a present
// key means "use my own (empty) list", distinct from "fall back to global".
func TestLoadFrom_NPCOverlayHiddenItemsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	m, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom: %v", err)
	}
	c := m.Get()
	c.Preferences.NPCOverlayHiddenItems = []string{"ability.10", "combat.atk"}
	c.Preferences.NPCOverlayHiddenItemsByCharacter = map[string][]string{
		"osui":  {"ability.14"},
		"empty": {},
	}
	if err := m.Update(c); err != nil {
		t.Fatalf("Update: %v", err)
	}

	m2, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("LoadFrom (reload): %v", err)
	}
	got := m2.Get().Preferences
	if len(got.NPCOverlayHiddenItems) != 2 || got.NPCOverlayHiddenItems[0] != "ability.10" {
		t.Errorf("global list = %v", got.NPCOverlayHiddenItems)
	}
	if v := got.NPCOverlayHiddenItemsByCharacter["osui"]; len(v) != 1 || v[0] != "ability.14" {
		t.Errorf("osui override = %v", v)
	}
	if _, ok := got.NPCOverlayHiddenItemsByCharacter["empty"]; !ok {
		t.Error("empty override key was dropped on round trip")
	}
}

// Section toggles move into the hidden-items lists as section keys, prefixed
// by surface when hidden on only one, and the structs end up all-true.
func TestMigrateNPCSectionsToHidden(t *testing.T) {
	var p Preferences
	p.NPCOverlayDashboardSections = DefaultNPCOverlaySections()
	p.NPCOverlayPopoutSections = DefaultNPCOverlaySections()
	p.NPCOverlayDashboardSections.Faction = false
	p.NPCOverlayPopoutSections.Faction = false
	p.NPCOverlayPopoutSections.Attributes = false
	p.NPCOverlayDashboardSections.SpellsClass = false
	p.NPCOverlayHiddenItems = []string{"ability.10"}
	p.NPCOverlayHiddenItemsByCharacter = map[string][]string{"osui": {}}

	migrateNPCSectionsToHidden(&p)

	want := []string{"ability.10", "popout:section.attributes", "section.faction", "dashboard:section.spells_class"}
	if !reflect.DeepEqual(p.NPCOverlayHiddenItems, want) {
		t.Errorf("global = %v, want %v", p.NPCOverlayHiddenItems, want)
	}
	if got := p.NPCOverlayHiddenItemsByCharacter["osui"]; !reflect.DeepEqual(got, want[1:]) {
		t.Errorf("osui = %v, want %v", got, want[1:])
	}
	if p.NPCOverlayDashboardSections != DefaultNPCOverlaySections() ||
		p.NPCOverlayPopoutSections != DefaultNPCOverlaySections() {
		t.Error("section structs not reset to all-true")
	}
}

// Ranges switched to automatic by default; a config that already had custom
// ranges keeps them by being marked manual.
func TestLoadFrom_NPCRangesAutoMigration(t *testing.T) {
	tests := []struct {
		name       string
		prefs      string
		wantManual bool
	}{
		{"defaults go automatic", "  npc_overlay_cast_range: 200\n", false},
		{"custom cast range stays manual", "  npc_overlay_cast_range: 250\n", true},
		{"custom ranged range stays manual", "  npc_overlay_cast_range: 200\n  npc_overlay_ranged_range: 300\n", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte("eq_path: /games/EQ\npreferences:\n"+tt.prefs), 0o644); err != nil {
				t.Fatal(err)
			}
			m, err := LoadFrom(path)
			if err != nil {
				t.Fatalf("LoadFrom: %v", err)
			}
			p := m.Get().Preferences
			if p.NPCOverlayRangesManual != tt.wantManual {
				t.Errorf("manual = %v, want %v", p.NPCOverlayRangesManual, tt.wantManual)
			}
			if !p.NPCRangesAutoMigrationDone {
				t.Error("migration flag not set")
			}
		})
	}
}

func TestSeedItemInfoOverlay(t *testing.T) {
	tests := []struct {
		name       string
		in         ItemInfoOverlaySettings
		wantHidden []string
		wantLimit  int
	}{
		{
			name:       "fresh config gets the default hidden sections",
			in:         ItemInfoOverlaySettings{},
			wantHidden: []string{"section.drops", "section.sources", "facts.races"},
			wantLimit:  DefaultItemInfoListLimit,
		},
		{
			name:       "seeded config that shows everything stays empty",
			in:         ItemInfoOverlaySettings{DefaultsSeeded: true, ListLimit: 3},
			wantHidden: nil,
			wantLimit:  3,
		},
		{
			name:       "existing list is kept",
			in:         ItemInfoOverlaySettings{HiddenItems: []string{"facts.weight"}},
			wantHidden: []string{"facts.weight"},
			wantLimit:  DefaultItemInfoListLimit,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := Preferences{ItemInfoOverlay: tt.in}
			seedItemInfoOverlay(&p)
			got := p.ItemInfoOverlay
			if !slices.Equal(got.HiddenItems, tt.wantHidden) {
				t.Errorf("HiddenItems = %v, want %v", got.HiddenItems, tt.wantHidden)
			}
			if got.ListLimit != tt.wantLimit {
				t.Errorf("ListLimit = %d, want %d", got.ListLimit, tt.wantLimit)
			}
			if !got.DefaultsSeeded {
				t.Error("DefaultsSeeded = false, want true")
			}
		})
	}
}

func TestLoadFrom_ItemInfoShowAllSurvivesReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	m, err := LoadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	c := m.Get()
	c.Preferences.ItemInfoOverlay.HiddenItems = []string{}
	if err := m.Update(c); err != nil {
		t.Fatal(err)
	}
	m2, err := LoadFrom(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := m2.Get().Preferences.ItemInfoOverlay.HiddenItems; len(got) != 0 {
		t.Errorf("HiddenItems after reload = %v, want empty", got)
	}
}
