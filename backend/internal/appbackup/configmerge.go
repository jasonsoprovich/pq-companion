package appbackup

import (
	"os"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
)

// mergeImportedConfig combines an imported config.yaml (from a .pqcb bundle,
// exported on a different machine) with the local install's current config.
// local is nil for a fresh install that has no config.yaml yet. Most fields
// are machine-independent preferences, so the import should win — that's the
// whole point of transferring a setup. A handful of fields point at local
// filesystem paths that may not exist on this machine, and importing those
// verbatim would leave the app pointed at a log file or TTS binary that
// silently does nothing. Those are validated against disk and fall back to
// local (or blank) when the imported path doesn't resolve here.
func mergeImportedConfig(imported config.Config, local *config.Config) config.Config {
	merged := imported

	// The listen address is a per-install choice (picked to dodge whatever's
	// already bound on this machine) — never carry it across an existing
	// install. A fresh install has nothing to protect, so the imported value
	// (itself a legitimate default) is used as-is rather than falling back to
	// a zero value.
	if local != nil {
		merged.ServerAddr = local.ServerAddr
	}

	// EQPath: only take the imported path if it exists on this machine.
	// Otherwise keep whatever the local install already had; if neither
	// resolves, blank it and force onboarding rather than leaving the app
	// pointed at a dead path with no indication anything's wrong.
	localEQPath := ""
	if local != nil {
		localEQPath = local.EQPath
	}
	if pathExists(imported.EQPath) {
		merged.EQPath = imported.EQPath
	} else if pathExists(localEQPath) {
		merged.EQPath = local.EQPath
		merged.Character = local.Character
		merged.CharacterClass = local.CharacterClass
	} else {
		merged.EQPath = ""
		merged.Character = ""
		merged.CharacterClass = -1
		merged.OnboardingCompleted = false
	}

	// Piper is a user-installed external binary + model file PQC never
	// bundles — importing a path to an exe that isn't on this machine would
	// make every "piper:local" alert fail silently. Disable rather than carry
	// a dangling reference.
	if !pathExists(imported.Preferences.PiperExePath) || !pathExists(imported.Preferences.PiperModelPath) {
		merged.Preferences.PiperEnabled = false
		merged.Preferences.PiperExePath = ""
		merged.Preferences.PiperModelPath = ""
	}

	return merged
}

// pathExists reports whether p is non-empty and resolves to an existing file
// or directory on this machine.
func pathExists(p string) bool {
	if p == "" {
		return false
	}
	_, err := os.Stat(p)
	return err == nil
}
