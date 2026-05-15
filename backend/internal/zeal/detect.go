package zeal

import (
	"os"
	"path/filepath"
	"strings"
)

// InstallStatus describes whether the Zeal mod (https://github.com/CoastalRedwood/Zeal)
// is installed in a given EverQuest directory.
//
// The check is a filesystem probe: we look for Zeal.asi sitting next to
// eqgame.exe. This is the same install layout Zeal documents and matches the
// runtime expectation that the ASI loader injects the DLL into eqgame.exe.
//
// A separate runtime check — whether Zeal is currently *running* — happens via
// the named-pipe dialer in a later stage. Filesystem presence is sufficient
// for the setup-wizard "do you have Zeal" question.
type InstallStatus struct {
	EQPath        string `json:"eq_path"`
	Installed     bool   `json:"installed"`
	EQGamePresent bool   `json:"eqgame_present"`
	ASIPath       string `json:"asi_path,omitempty"`
}

// DetectInstall reports whether Zeal.asi is present in the given EverQuest
// directory. Returns a zero-value status when eqPath is empty or unreadable —
// callers treat that as "not installed" rather than an error.
func DetectInstall(eqPath string) InstallStatus {
	status := InstallStatus{EQPath: eqPath}
	if strings.TrimSpace(eqPath) == "" {
		return status
	}
	entries, err := os.ReadDir(eqPath)
	if err != nil {
		return status
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		switch strings.ToLower(name) {
		case "eqgame.exe":
			status.EQGamePresent = true
		case "zeal.asi":
			status.Installed = true
			status.ASIPath = filepath.Join(eqPath, name)
		}
	}
	return status
}
