package zeal

import (
	"context"
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
	// Version is the ZEAL_VERSION string parsed from Zeal.asi (e.g. "1.4.2").
	// Empty when Zeal isn't installed or the literal couldn't be located.
	Version string `json:"version,omitempty"`
	// MinVersion is the lowest Zeal release pq-companion fully supports.
	// Frontend compares against Version to decide whether to show the red
	// "please update Zeal" banner.
	MinVersion string `json:"min_version,omitempty"`
	// VersionOK is true when Version is known and >= MinVersion, OR when
	// Version is unknown (we don't false-alarm on detection failures).
	// False only when we positively know the installed version is too old.
	VersionOK bool `json:"version_ok"`
	// LatestVersion is the most recent Zeal release we know about, fetched
	// from GitHub (see LatestFetcher). Empty when offline or never fetched.
	LatestVersion string `json:"latest_version,omitempty"`
	// UpdateAvailable is true when Version is known, LatestVersion is known,
	// Version >= MinVersion (so the red banner doesn't apply), AND Version
	// is strictly behind LatestVersion. This drives a soft "newer Zeal
	// available" hint in Settings — never a full-app banner.
	UpdateAvailable bool `json:"update_available"`
	// ExportOnCampFound is true when we successfully read zeal.ini and the
	// ExportOnCamp key was present. False when the file is missing or the
	// key isn't written yet — in that case the UI treats it as unknown.
	ExportOnCampFound bool `json:"export_on_camp_found"`
	// ExportOnCamp is true when zeal.ini's ExportOnCamp setting is enabled.
	// Most of pq-companion's character-data features (inventory, quarmy,
	// spellbook, spellsets) depend on Zeal writing these files at /camp; a
	// false value here is worth surfacing as a warning.
	ExportOnCamp bool `json:"export_on_camp"`
}

// DetectInstall reports whether Zeal.asi is present in the given EverQuest
// directory. Returns a zero-value status when eqPath is empty or unreadable —
// callers treat that as "not installed" rather than an error.
//
// If latest is non-nil, the status is enriched with the most recent Zeal
// release version and an UpdateAvailable flag. Pass nil for the lightweight
// onboarding-wizard path that runs before we've constructed the fetcher.
func DetectInstall(ctx context.Context, eqPath string, latest *LatestFetcher) InstallStatus {
	status := InstallStatus{
		EQPath:     eqPath,
		MinVersion: MinSupportedVersion,
		VersionOK:  true,
	}
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
	if status.Installed && status.ASIPath != "" {
		if v, err := ReadInstalledVersion(status.ASIPath); err == nil && v != "" {
			status.Version = v
			status.VersionOK = CompareVersions(v, MinSupportedVersion) >= 0
		}
		if latest != nil {
			if lv := latest.Get(ctx); lv != "" {
				status.LatestVersion = lv
				if status.Version != "" && status.VersionOK && CompareVersions(status.Version, lv) < 0 {
					status.UpdateAvailable = true
				}
			}
		}
		ec := ReadExportOnCamp(eqPath)
		status.ExportOnCampFound = ec.Found
		status.ExportOnCamp = ec.Enabled
	}
	return status
}
