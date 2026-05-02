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
	//	"self"   — only spells where the recipient is the active player; buffs
	//	           and debuffs landing on other characters are ignored.
	//	"anyone" — every spell that lands on a recognised target name (default;
	//	           matches the post-PR1 behaviour and is what raid-buff trackers
	//	           need).
	//
	// An empty string is treated as "anyone" so existing config files don't
	// need migration.
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
}

// TrackingScope* are the canonical values for SpellTimerSettings.TrackingScope.
const (
	TrackingScopeSelf   = "self"
	TrackingScopeAnyone = "anyone"
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
}

// defaults returns a Config populated with sensible default values.
func defaults() Config {
	return Config{
		ServerAddr:     ":8080",
		CharacterClass: -1,
		Preferences: Preferences{
			OverlayOpacity:    0.25,
			MinimizeToTray:    true,
			ParseCombatLog:    true,
			OverlayDPSEnabled: true,
			OverlayHPSEnabled: false,
		},
		Backup: BackupSettings{
			AutoBackup: false,
			Schedule:   "off",
			MaxBackups: 10,
		},
		SpellTimer: SpellTimerSettings{
			TrackingScope: TrackingScopeAnyone,
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
	applyDefaults(&cfg)

	return &Manager{cfg: cfg, path: path}, nil
}

// applyDefaults fills in fields the user's on-disk file may be missing.
// Older config files predate newly-added settings; setting safe defaults
// here keeps the engine and UI from having to special-case empty values.
func applyDefaults(cfg *Config) {
	if cfg.SpellTimer.TrackingScope == "" {
		cfg.SpellTimer.TrackingScope = TrackingScopeAnyone
	}
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
