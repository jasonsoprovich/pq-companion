// Package config manages the PQ Companion user configuration file.
// The config is stored as YAML at ~/.pq-companion/config.yaml and can be
// read and updated via the REST API.
package config

import (
	"errors"
	"os"
	"path/filepath"
	"sync"

	"gopkg.in/yaml.v3"
)

// Config holds all user-configurable settings.
type Config struct {
	// EQPath is the path to the EverQuest installation directory.
	// Used to locate log files and Zeal export files.
	EQPath string `yaml:"eq_path" json:"eq_path"`

	// Character is the active character name (no server suffix).
	// Used to locate the correct log file: eqlog_<Character>_pq.proj.txt
	Character string `yaml:"character" json:"character"`

	// CharacterClass is the EverQuest class index (0–14) for the active
	// character. Used to auto-select the correct class in the Spell Checklist.
	// 0=WAR 1=CLR 2=PAL 3=RNG 4=SHD 5=DRU 6=MNK 7=BRD 8=ROG 9=SHM
	// 10=NEC 11=WIZ 12=MAG 13=ENC 14=BST   -1 = not set
	CharacterClass int `yaml:"character_class" json:"character_class"`

	// ServerAddr is the HTTP listen address for the backend API.
	ServerAddr string `yaml:"server_addr" json:"server_addr"`

	// Preferences holds UI and overlay preferences.
	Preferences Preferences `yaml:"preferences" json:"preferences"`

	// Backup holds backup manager settings.
	Backup BackupSettings `yaml:"backup" json:"backup"`

	// SpellTimer holds spell timer engine settings.
	SpellTimer SpellTimerSettings `yaml:"spell_timer" json:"spell_timer"`

	// Combat holds combat tracker / history settings.
	Combat CombatSettings `yaml:"combat" json:"combat"`

	// DPSClassColors maps each EQ class to the bar colour the DPS meter and
	// combat-history rows render for combatants of that class. Stored as
	// CSS-style #RRGGBB hex strings so the frontend can apply them
	// verbatim. Unknown / unclassified combatants fall back to Unknown.
	DPSClassColors DPSClassColors `yaml:"dps_class_colors" json:"dps_class_colors"`

	// OnboardingCompleted is true once the user has finished the first-launch
	// setup wizard. When false (default), the wizard is shown on app launch.
	OnboardingCompleted bool `yaml:"onboarding_completed" json:"onboarding_completed"`

	// ChatRetentionDays is how many days of Chat History to keep before the
	// daily purge deletes older messages. Default 30. A negative value (e.g.
	// -1) keeps chat forever. As with Combat.RetentionDays, the YAML decoder
	// can't tell a missing field from an explicit 0, so applyDefaults treats 0
	// as "use the default"; "keep forever" is stored as -1.
	ChatRetentionDays int `yaml:"chat_retention_days" json:"chat_retention_days"`
}

// BackupSettings configures automatic backup behaviour.
type BackupSettings struct {
	// AutoBackup enables file-watcher-triggered backups when *.ini files change.
	AutoBackup bool `yaml:"auto_backup" json:"auto_backup"`

	// Schedule controls periodic backups: "off", "hourly", or "daily".
	Schedule string `yaml:"schedule" json:"schedule"`

	// MaxBackups is the maximum number of backups to retain (0 = unlimited).
	// When exceeded, the oldest unlocked backups are deleted automatically.
	MaxBackups int `yaml:"max_backups" json:"max_backups"`
}

// SpellTimerSettings configures the spell timer engine.
type SpellTimerSettings struct {
	// TrackingScope controls which spell-landed events become timers.
	//
	//	"self"       — only spells where the recipient is the active player.
	//	"cast_by_me" — every land where the active character is the caster
	//	               (default). Heuristic: a land within lastCastWindow of
	//	               our most recent EventSpellCast for the same spell name.
	//	               Filters out the noise of other players buffing each
	//	               other while still showing buffs we put on group/raid.
	//	"anyone"     — every spell that lands on a recognised target. Useful
	//	               when you want to see e.g. another enchanter's Tash on
	//	               a raid mob.
	//
	// An empty string is treated as "cast_by_me" so existing config files
	// without this field migrate to the new default.
	TrackingScope string `yaml:"tracking_scope" json:"tracking_scope"`

	// BuffDisplayThresholdSecs hides buff overlay rows whose remaining time
	// exceeds this value. 0 (default) means always show. Useful for raid
	// buff tracking where dozens of long-duration buffs would otherwise
	// drown the overlay — set to e.g. 600 to only see buffs in the last 10
	// minutes of their duration.
	BuffDisplayThresholdSecs int `yaml:"buff_display_threshold_secs" json:"buff_display_threshold_secs"`

	// DetrimDisplayThresholdSecs is the corresponding cap for the
	// Detrimental overlay. 0 (default) means always show. Detrimentals are
	// usually short-lived so the default of 0 matches existing behaviour.
	DetrimDisplayThresholdSecs int `yaml:"detrim_display_threshold_secs" json:"detrim_display_threshold_secs"`

	// CastByMeMigrationDone records that the one-time migration from the
	// old "anyone" default to "cast_by_me" has run for this config file.
	// Once true, the user's explicit "anyone" choice is preserved on load.
	CastByMeMigrationDone bool `yaml:"cast_by_me_migration_done,omitempty" json:"cast_by_me_migration_done,omitempty"`

	// ClassFilter, when true, drops buff timers whose source spell isn't
	// castable by the active character's class. Useful for hiding
	// out-of-class buffs (e.g. paladin Spiritual Purity, shaman Talisman,
	// bard songs) from a class with a long buff list of its own. Composes
	// with TrackingScope: detrimentals are always allowed regardless.
	// Defaults to false to preserve existing behaviour.
	ClassFilter bool `yaml:"class_filter,omitempty" json:"class_filter,omitempty"`

	// TrackingMode controls whether the spell-landed pipeline creates timer
	// rows automatically or only triggers/packs do.
	//
	//	"auto"          — default. Every recognised spell-landed event creates
	//	                  a timer; triggers/packs may attach metadata
	//	                  (thresholds, fading-soon TTS) by firing on the same
	//	                  cast.
	//	"triggers_only" — the spell-landed pipeline still parses log lines
	//	                  for cast disambiguation but does NOT create timers.
	//	                  Only triggers (custom or pack-installed) produce
	//	                  rows. Best for users who want a curated overlay
	//	                  rather than full auto-coverage.
	//
	// Empty string is treated as "auto" so existing config files migrate
	// cleanly without an explicit value.
	TrackingMode string `yaml:"tracking_mode,omitempty" json:"tracking_mode,omitempty"`
}

// DPSClassColors holds the user-customisable bar colours for the DPS meter
// and combat history rows. One hex value per EQ class plus an Unknown
// fallback for combatants whose class can't be resolved.
type DPSClassColors struct {
	Warrior      string `yaml:"warrior" json:"warrior"`
	Cleric       string `yaml:"cleric" json:"cleric"`
	Paladin      string `yaml:"paladin" json:"paladin"`
	Ranger       string `yaml:"ranger" json:"ranger"`
	ShadowKnight string `yaml:"shadow_knight" json:"shadow_knight"`
	Druid        string `yaml:"druid" json:"druid"`
	Monk         string `yaml:"monk" json:"monk"`
	Bard         string `yaml:"bard" json:"bard"`
	Rogue        string `yaml:"rogue" json:"rogue"`
	Shaman       string `yaml:"shaman" json:"shaman"`
	Necromancer  string `yaml:"necromancer" json:"necromancer"`
	Wizard       string `yaml:"wizard" json:"wizard"`
	Magician     string `yaml:"magician" json:"magician"`
	Enchanter    string `yaml:"enchanter" json:"enchanter"`
	Beastlord    string `yaml:"beastlord" json:"beastlord"`
	Unknown      string `yaml:"unknown" json:"unknown"`
}

// DefaultDPSClassColors is the seeded palette new installs (and any field
// that's been left blank on an existing config) fall back to.
func DefaultDPSClassColors() DPSClassColors {
	return DPSClassColors{
		Warrior:      "#C79C6E",
		Cleric:       "#FFFFFF",
		Paladin:      "#F58CBA",
		Ranger:       "#ABD473",
		ShadowKnight: "#C41F3B",
		Druid:        "#FF7D0A",
		Monk:         "#00FF96",
		Bard:         "#8A47E8",
		Rogue:        "#FFF569",
		Shaman:       "#0070DE",
		Necromancer:  "#9482C9",
		Wizard:       "#40ED57",
		Magician:     "#69CCF0",
		Enchanter:    "#ED5CE5",
		Beastlord:    "#03B78A",
		Unknown:      "#B2B2B2",
	}
}

// CombatSettings configures the combat tracker and persisted fight history.
type CombatSettings struct {
	// RetentionDays is the number of days of combat history to keep in
	// user.db. Fights older than this are pruned on startup and once per
	// hour. 0 (or negative) disables pruning entirely. Default 30 — covers
	// the common "let me see last week's raid" case without growing the DB
	// beyond a few MB for normal use.
	RetentionDays int `yaml:"retention_days" json:"retention_days"`
}

// TrackingScope* are the canonical values for SpellTimerSettings.TrackingScope.
const (
	TrackingScopeSelf     = "self"
	TrackingScopeCastByMe = "cast_by_me"
	TrackingScopeAnyone   = "anyone"
)

// TrackingMode* are the canonical values for SpellTimerSettings.TrackingMode.
const (
	TrackingModeAuto         = "auto"
	TrackingModeTriggersOnly = "triggers_only"
)

// Preferences holds optional UI and overlay settings.
type Preferences struct {
	// OverlayOpacity is the transparency of overlay windows (0.0–1.0).
	OverlayOpacity float64 `yaml:"overlay_opacity" json:"overlay_opacity"`

	// MinimizeToTray controls whether closing the main window hides to tray.
	MinimizeToTray bool `yaml:"minimize_to_tray" json:"minimize_to_tray"`

	// HighContrast raises the contrast of muted text and borders for
	// readability on high-resolution displays (issue #130). Off by default.
	HighContrast bool `yaml:"high_contrast,omitempty" json:"high_contrast"`

	// ZoomFactor scales the entire main window via Electron's zoom (like a
	// browser Ctrl+/Ctrl-), for readability on high-resolution displays
	// (issue #130). 1.0 = 100% (default). 0 is treated as 1.0 by the client.
	ZoomFactor float64 `yaml:"zoom_factor,omitempty" json:"zoom_factor"`

	// ParseCombatLog enables real-time combat log parsing.
	ParseCombatLog bool `yaml:"parse_combat_log" json:"parse_combat_log"`

	// OverlayDPSEnabled controls whether the DPS floating overlay is shown.
	OverlayDPSEnabled bool `yaml:"overlay_dps_enabled" json:"overlay_dps_enabled"`

	// OverlayHPSEnabled controls whether the HPS floating overlay is shown.
	OverlayHPSEnabled bool `yaml:"overlay_hps_enabled" json:"overlay_hps_enabled"`

	// MasterVolume scales every trigger / event / timer alert sound and TTS
	// utterance multiplicatively on top of its per-action volume. Stored as
	// 0–100 (percent); the frontend converts to 0.0–1.0 at playback time.
	// 100 = no dampening (default), 0 = mute everything.
	MasterVolume int `yaml:"master_volume" json:"master_volume"`

	// DeveloperMode reveals the Developer tab in the Settings page, which
	// hosts power-user tools (SQL sandbox, schema viewer). Toggled in-app
	// via the Ctrl+Shift+D shortcut while the Settings page is focused.
	// Off by default so casual users don't stumble into raw-DB territory.
	DeveloperMode bool `yaml:"developer_mode,omitempty" json:"developer_mode"`

	// NPCOverlayDashboardSections controls which optional sections of the
	// NPC overlay are visible in the dashboard panel. Name, zone, pet
	// owner, raid/rare badges, and the HP bar are always shown regardless
	// of these toggles.
	NPCOverlayDashboardSections NPCOverlaySections `yaml:"npc_overlay_dashboard_sections" json:"npc_overlay_dashboard_sections"`

	// NPCOverlayPopoutSections is the same toggle set applied to the
	// floating popout overlay window, so users can run a denser surface
	// in one place and a sparser one in the other.
	NPCOverlayPopoutSections NPCOverlaySections `yaml:"npc_overlay_popout_sections" json:"npc_overlay_popout_sections"`

	// NPCFactionSectionMigrationDone marks that the one-time migration which
	// turns the (later-added) Faction section on for pre-existing configs has
	// run. Without it, configs saved before the Faction field existed would
	// deserialize Faction=false and never show the new section even though it
	// defaults on for fresh installs. See applyDefaults.
	NPCFactionSectionMigrationDone bool `yaml:"npc_faction_section_migration_done,omitempty" json:"npc_faction_section_migration_done"`

	// OverlayLockedModes maps each popout overlay (by its canonical name:
	// "dps", "hps", "buffTimer", "detrimTimer", "npc", "rollTracker",
	// "respawnTimer") to how it behaves while locked:
	//   "interactive"  — hover the overlay to scroll / clear individual rows;
	//                     move off and clicks pass through to the game (default)
	//   "clickthrough" — only the title-bar buttons are clickable; scrolling
	//                     and clicks everywhere else pass through to the game
	// Missing keys default to "interactive" (the pre-existing behaviour), so
	// the map can be nil/omitted for upgrading users with no migration.
	OverlayLockedModes map[string]string `yaml:"overlay_locked_modes,omitempty" json:"overlay_locked_modes,omitempty"`

	// SidebarHidden lists side-nav route keys (e.g. "/loot") the user has
	// hidden from the navigation menu. Hiding only removes the tab from the
	// sidebar — the page is still reachable by URL. Empty/omitted = nothing
	// hidden. The fixed controls (back/forward, search, character switcher,
	// settings) are never hideable.
	SidebarHidden []string `yaml:"sidebar_hidden,omitempty" json:"sidebar_hidden,omitempty"`

	// SidebarOrder is a flat list of side-nav route keys in the user's
	// preferred display order. Items are ordered within their section by their
	// position here; keys absent from the list keep their default order after
	// the listed ones. Empty/omitted = default order.
	SidebarOrder []string `yaml:"sidebar_order,omitempty" json:"sidebar_order,omitempty"`
}

// NPCOverlaySections toggles individual NPC overlay sections on/off. All
// fields default to true so behaviour is unchanged for upgrading users.
type NPCOverlaySections struct {
	Identity         bool `yaml:"identity" json:"identity"`
	Combat           bool `yaml:"combat" json:"combat"`
	Resists          bool `yaml:"resists" json:"resists"`
	Attributes       bool `yaml:"attributes" json:"attributes"`
	SpecialAbilities bool `yaml:"special_abilities" json:"special_abilities"`
	Faction          bool `yaml:"faction" json:"faction"`
}

// DefaultNPCOverlaySections returns the all-visible default — matches the
// behaviour before this preference existed (Faction was added later and is
// also on by default; see the migration in applyDefaults for existing configs).
func DefaultNPCOverlaySections() NPCOverlaySections {
	return NPCOverlaySections{
		Identity:         true,
		Combat:           true,
		Resists:          true,
		Attributes:       true,
		SpecialAbilities: true,
		Faction:          true,
	}
}

// defaults returns a Config populated with sensible default values.
func defaults() Config {
	return Config{
		// 17654 is in the IANA registered range (1024-49151) but not used by
		// any well-known service — picked specifically to avoid collisions
		// with the more commonly squatted ports (8080, 3000, 5000, etc.).
		// If this port is still occupied on a particular user's machine the
		// server falls back to an OS-assigned port at startup.
		ServerAddr:     ":17654",
		CharacterClass: -1,
		Preferences: Preferences{
			OverlayOpacity:              0.25,
			MinimizeToTray:              true,
			ParseCombatLog:              true,
			OverlayDPSEnabled:           true,
			OverlayHPSEnabled:           false,
			MasterVolume:                100,
			ZoomFactor:                  1.0,
			NPCOverlayDashboardSections: DefaultNPCOverlaySections(),
			NPCOverlayPopoutSections:    DefaultNPCOverlaySections(),
		},
		Backup: BackupSettings{
			AutoBackup: false,
			Schedule:   "off",
			MaxBackups: 10,
		},
		SpellTimer: SpellTimerSettings{
			TrackingScope: TrackingScopeCastByMe,
			// CastByMeMigrationDone is intentionally left false here so that
			// applyDefaults can detect a config file that predates the
			// migration. For brand-new files we set it to true in the
			// not-exist branch of LoadFrom.
		},
		Combat: CombatSettings{
			RetentionDays: 30,
		},
		ChatRetentionDays: 30,
		DPSClassColors:    DefaultDPSClassColors(),
	}
}

// Manager holds a loaded Config and serializes concurrent access.
type Manager struct {
	mu   sync.RWMutex
	cfg  Config
	path string
}

// Load reads (or creates) the config file at the default location
// (~/.pq-companion/config.yaml) and returns a Manager.
func Load() (*Manager, error) {
	path, err := defaultPath()
	if err != nil {
		return nil, err
	}
	return LoadFrom(path)
}

// LoadFrom reads (or creates) the config file at the given path.
func LoadFrom(path string) (*Manager, error) {
	cfg := defaults()

	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		// File does not exist — write defaults so the user has a template.
		// New files are born already-migrated; the marker only matters for
		// pre-existing configs that had "anyone" as the prior default.
		cfg.SpellTimer.CastByMeMigrationDone = true
		m := &Manager{cfg: cfg, path: path}
		if saveErr := m.save(); saveErr != nil {
			// Non-fatal: we can operate without a file on disk.
			_ = saveErr
		}
		return m, nil
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	migrated := applyDefaults(&cfg)

	m := &Manager{cfg: cfg, path: path}
	if migrated {
		// Persist the migration so it doesn't run again next launch.
		_ = m.save()
	}
	return m, nil
}

// applyDefaults fills in fields the user's on-disk file may be missing.
// Older config files predate newly-added settings; setting safe defaults
// here keeps the engine and UI from having to special-case empty values.
//
// Returns true when a value was changed and the file should be re-saved.
func applyDefaults(cfg *Config) bool {
	changed := false
	// One-time migration: the prior default was "anyone", which produced
	// noisy overlays when other players were buffing each other. New default
	// is "cast_by_me". Existing configs that haven't been migrated yet are
	// rewritten exactly once; subsequent explicit choices of "anyone" are
	// preserved.
	if !cfg.SpellTimer.CastByMeMigrationDone {
		if cfg.SpellTimer.TrackingScope == "" || cfg.SpellTimer.TrackingScope == TrackingScopeAnyone {
			cfg.SpellTimer.TrackingScope = TrackingScopeCastByMe
		}
		cfg.SpellTimer.CastByMeMigrationDone = true
		changed = true
	} else if cfg.SpellTimer.TrackingScope == "" {
		cfg.SpellTimer.TrackingScope = TrackingScopeCastByMe
		changed = true
	}
	// Combat history retention default — applies to configs that predate
	// the combat history feature. The YAML decoder leaves missing numeric
	// fields at zero, so a missing value and an explicit "0" are
	// indistinguishable. Treat both as "use the default"; users who really
	// want pruning disabled can write a negative value (e.g. -1), which
	// pruneCombatHistory honours as "skip".
	if cfg.Combat.RetentionDays == 0 {
		cfg.Combat.RetentionDays = 30
		changed = true
	}
	// Chat history retention: same convention as Combat — a missing/zero value
	// means "use the default" (30 days); a negative value disables the purge
	// (keep forever).
	if cfg.ChatRetentionDays == 0 {
		cfg.ChatRetentionDays = 30
		changed = true
	}
	// DPS class colour palette: fill in any blank entries from the defaults so
	// upgrading users get the palette without losing per-class overrides
	// they may have set previously.
	if fillDPSColorDefaults(&cfg.DPSClassColors) {
		changed = true
	}
	// NPC overlay sections: configs that predate this preference deserialize
	// with every bool false, which would hide every section. Detect the
	// all-zero case and substitute the all-visible default so the overlay
	// keeps working after upgrade. Once the user makes any explicit choice
	// (toggling at least one section on), the struct will have a true value
	// and this branch becomes a no-op.
	if isZeroNPCOverlaySections(cfg.Preferences.NPCOverlayDashboardSections) {
		cfg.Preferences.NPCOverlayDashboardSections = DefaultNPCOverlaySections()
		changed = true
	}
	if isZeroNPCOverlaySections(cfg.Preferences.NPCOverlayPopoutSections) {
		cfg.Preferences.NPCOverlayPopoutSections = DefaultNPCOverlaySections()
		changed = true
	}
	// One-time migration for the later-added Faction section: configs written
	// before the field existed deserialize Faction=false on otherwise-populated
	// section structs (so the all-zero reset above doesn't touch them). Turn it
	// on once to match the fresh-install default, then never override the user's
	// explicit choice again.
	if !cfg.Preferences.NPCFactionSectionMigrationDone {
		cfg.Preferences.NPCOverlayDashboardSections.Faction = true
		cfg.Preferences.NPCOverlayPopoutSections.Faction = true
		cfg.Preferences.NPCFactionSectionMigrationDone = true
		changed = true
	}
	return changed
}

func isZeroNPCOverlaySections(s NPCOverlaySections) bool {
	return !s.Identity && !s.Combat && !s.Resists && !s.Attributes &&
		!s.SpecialAbilities && !s.Faction
}

// fillDPSColorDefaults populates any empty hex fields with the corresponding
// default. Returns true when at least one field was changed.
func fillDPSColorDefaults(c *DPSClassColors) bool {
	d := DefaultDPSClassColors()
	changed := false
	pairs := []struct {
		dst *string
		def string
	}{
		{&c.Warrior, d.Warrior},
		{&c.Cleric, d.Cleric},
		{&c.Paladin, d.Paladin},
		{&c.Ranger, d.Ranger},
		{&c.ShadowKnight, d.ShadowKnight},
		{&c.Druid, d.Druid},
		{&c.Monk, d.Monk},
		{&c.Bard, d.Bard},
		{&c.Rogue, d.Rogue},
		{&c.Shaman, d.Shaman},
		{&c.Necromancer, d.Necromancer},
		{&c.Wizard, d.Wizard},
		{&c.Magician, d.Magician},
		{&c.Enchanter, d.Enchanter},
		{&c.Beastlord, d.Beastlord},
		{&c.Unknown, d.Unknown},
	}
	for _, p := range pairs {
		if *p.dst == "" {
			*p.dst = p.def
			changed = true
		}
	}
	return changed
}

// Get returns a copy of the current config.
func (m *Manager) Get() Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.cfg
}

// Update replaces the current config with c and persists it to disk.
func (m *Manager) Update(c Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cfg = c
	return m.save()
}

// save writes the current config to disk (must be called with m.mu held).
func (m *Manager) save() error {
	if err := os.MkdirAll(filepath.Dir(m.path), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(&m.cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(m.path, data, 0o644)
}

// Path returns the path to the config file on disk.
func (m *Manager) Path() string { return m.path }

// defaultPath returns ~/.pq-companion/config.yaml.
func defaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".pq-companion", "config.yaml"), nil
}
