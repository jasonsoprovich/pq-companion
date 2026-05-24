package quarm

import (
	"context"
	"errors"
	"os"
	"path/filepath"
)

// MatchStatus describes the manifest-comparison outcome for a single client
// file. The values are stable strings so the frontend can switch on them
// without a numeric enum.
type MatchStatus string

const (
	// StatusMatch — local file MD5 + size match the manifest entry.
	StatusMatch MatchStatus = "match"
	// StatusMismatch — manifest knows about this file and the local copy
	// differs (MD5 or size mismatch). Could mean the local copy is newer
	// than the manifest, older, or hand-modified. Surfaced as "out of date"
	// in the UI without speculating which direction.
	StatusMismatch MatchStatus = "mismatch"
	// StatusMissing — file is configured by Quarm (in the manifest) but
	// not present on disk. Strong signal something's wrong.
	StatusMissing MatchStatus = "missing"
	// StatusUnknown — we couldn't determine a verdict, e.g. EQ path not
	// configured, manifest fetch failed, or the file isn't covered by the
	// manifest (eqw.dll). UI shows informational data without a verdict.
	StatusUnknown MatchStatus = "unknown"
)

// FileStatus is the per-file payload returned by /api/quarm/client-status.
// Local holds whatever we could extract from the user's copy (MD5, size, PE
// timestamp, FileVersion); Manifest is the matching entry from the Quarm
// manifest when one exists; Status is the comparison verdict.
type FileStatus struct {
	Name     string         `json:"name"`
	Status   MatchStatus    `json:"status"`
	Local    *FileInfo      `json:"local,omitempty"`
	Manifest *ManifestEntry `json:"manifest,omitempty"`
	// Reason is a short human-readable hint when Status is unknown or
	// mismatch. Empty when no extra context is needed.
	Reason string `json:"reason,omitempty"`
}

// ClientStatus is the full response. ManifestVersion and ManifestError
// surface the upstream state so the UI can warn ("manifest unreachable")
// distinct from "your file is out of date."
type ClientStatus struct {
	EQPath          string       `json:"eq_path"`
	Files           []FileStatus `json:"files"`
	ManifestVersion string       `json:"manifest_version,omitempty"`
	ManifestError   string       `json:"manifest_error,omitempty"`
}

// trackedFiles enumerates the client DLLs we report on. Order matches the
// display order in the Settings UI.
//
// inManifest=false means "we never expect a manifest entry; show local-only
// info" — that's true for eqw.dll because it ships with Zeal rather than as
// part of the Quarm-patched game files.
var trackedFiles = []struct {
	name        string
	inManifest  bool
}{
	{"eqgame.dll", true},
	{"eqw.dll", false},
}

// Status inspects the configured EQ install and returns a per-file
// comparison against the manifest. A zero-value EQ path produces a
// status-unknown response for every tracked file — the API caller is
// expected to render that as "configure your EQ path in Settings."
func Status(ctx context.Context, eqPath string, fetcher *ManifestFetcher) ClientStatus {
	out := ClientStatus{EQPath: eqPath}

	var manifest *Manifest
	if fetcher != nil {
		if m, err := fetcher.Get(ctx); err == nil {
			manifest = m
			out.ManifestVersion = m.Version
		} else {
			out.ManifestError = err.Error()
		}
	}

	for _, t := range trackedFiles {
		out.Files = append(out.Files, evaluateFile(eqPath, t.name, t.inManifest, manifest))
	}
	return out
}

// evaluateFile reads one DLL and produces its FileStatus. It is total: it
// always returns a FileStatus with a meaningful Status, even for missing or
// unreadable files. Errors are surfaced as Reason text, not as exceptions.
func evaluateFile(eqPath, name string, inManifest bool, manifest *Manifest) FileStatus {
	out := FileStatus{Name: name, Status: StatusUnknown}

	if eqPath == "" {
		out.Reason = "EQ path not configured"
		return out
	}
	full := filepath.Join(eqPath, name)
	info, err := InspectDLL(full)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if inManifest && manifest != nil {
				out.Status = StatusMissing
				out.Reason = "file not found in EQ directory"
				if entry := manifest.FindEntry(name); entry != nil {
					out.Manifest = entry
				}
			} else {
				out.Reason = "file not found in EQ directory"
			}
			return out
		}
		out.Reason = err.Error()
		return out
	}
	out.Local = &info

	if !inManifest || manifest == nil {
		// Local-only display. Reason left empty unless we have a positive
		// note to add — eqw.dll falls here every time.
		return out
	}

	entry := manifest.FindEntry(name)
	if entry == nil {
		// In manifest set conceptually but absent from the actual manifest
		// — likely the manifest changed shape. Don't claim a mismatch.
		out.Reason = "manifest has no entry for this file"
		return out
	}
	out.Manifest = entry

	if entry.MD5 == info.MD5 && entry.Size == info.Size {
		out.Status = StatusMatch
		return out
	}
	out.Status = StatusMismatch
	return out
}
