package eqw

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/jasonsoprovich/pq-companion/backend/internal/zeal"
)

// Status describes the eqw.dll (EQW-TAKP) install in a given EverQuest
// directory. It intentionally mirrors the shape of zeal.InstallStatus's
// version fields — same string-scan + GitHub-tag mechanism — minus the
// min-version gate: there is no PQ Companion feature that depends on an EQW
// version, so we only ever surface a soft "update available" hint, never a
// blocking banner.
type Status struct {
	EQPath string `json:"eq_path"`
	// Installed is true when eqw.dll sits next to eqgame.exe.
	Installed bool   `json:"installed"`
	DLLPath   string `json:"dll_path,omitempty"`
	// Version is the EQW-TAKP build stamp parsed from eqw.dll (e.g. "1.0.1").
	// Empty when eqw.dll isn't installed or the stamp couldn't be located.
	Version string `json:"version,omitempty"`
	// LatestVersion is the most recent EQW-TAKP release we know about, fetched
	// from GitHub. Empty when offline or never fetched.
	LatestVersion string `json:"latest_version,omitempty"`
	// UpdateAvailable is true only when both Version and LatestVersion are
	// known and Version is strictly behind LatestVersion.
	UpdateAvailable bool `json:"update_available"`
}

// DetectStatus reports the eqw.dll install state for eqPath. Returns a
// zero-value status when eqPath is empty or unreadable. If latest is non-nil,
// the status is enriched with the newest release version and an
// UpdateAvailable flag; pass nil to skip the network call.
func DetectStatus(ctx context.Context, eqPath string, latest *LatestFetcher) Status {
	status := Status{EQPath: eqPath}
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
		if strings.EqualFold(entry.Name(), "eqw.dll") {
			status.Installed = true
			status.DLLPath = filepath.Join(eqPath, entry.Name())
			break
		}
	}
	if !status.Installed {
		return status
	}
	if v, err := ReadInstalledVersion(status.DLLPath); err == nil && v != "" {
		status.Version = v
	}
	if latest != nil {
		if lv := latest.Get(ctx); lv != "" {
			status.LatestVersion = lv
			if status.Version != "" && zeal.CompareVersions(status.Version, lv) < 0 {
				status.UpdateAvailable = true
			}
		}
	}
	return status
}
