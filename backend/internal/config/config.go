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

	// ServerAddr is the HTTP listen address for the backend API.
	ServerAddr string `yaml:"server_addr" json:"server_addr"`

	// Preferences holds UI and overlay preferences.
	Preferences Preferences `yaml:"preferences" json:"preferences"`

	// Backup holds backup manager settings.
	Backup BackupSettings `yaml:"backup" json:"backup"`
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
		ServerAddr: ":8080",
		Preferences: Preferences{
			OverlayOpacity:    0.9,
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

	return &Manager{cfg: cfg, path: path}, nil
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
