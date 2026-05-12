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

	// OnboardingCompleted is true once the user has finished the first-launch
	// setup wizard. When false (default), the wizard is shown on app launch.
	OnboardingCompleted bool `yaml:"onboarding_completed" json:"onboarding_completed"`
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
			OverlayOpacity:    0.25,
			MinimizeToTray:    true,
			ParseCombatLog:    true,
			OverlayDPSEnabled: true,
			OverlayHPSEnabled: false,
			MasterVolume:      100,
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
