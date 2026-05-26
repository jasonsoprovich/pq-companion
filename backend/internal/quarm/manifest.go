package quarm

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// ManifestURL is the GitHub Releases shortcut for the Quarm patcher's
// canonical file list. /releases/latest/download/<name> redirects to whatever
// the most recently tagged release ships, so this URL never needs to change
// even when the Quarm patcher cuts a new release.
//
// Source: github.com/Pkelly668/QuarmPatcher (the active fork of EQEmu's
// patcher used by Project Quarm). The manifest is YAML with one entry per
// file containing name, md5, date, size — same format as the upstream
// secretsothep/eqemupatcher patcher consumes.
const ManifestURL = "https://github.com/Pkelly668/QuarmPatcher/releases/latest/download/filelist_rof.yml"

// ManifestEntry is one file listed in filelist_rof.yml.
//
// RefFileVersion is NOT read from the YAML — it is populated by the
// ManifestFetcher after parsing, by downloading the reference DLL and
// inspecting its VS_VERSION_INFO resource. The Pkelly668 patcher manifest
// has historically lagged Quarm's actual distribution (the user runs the
// official patcher and ends up with a different MD5 but the same product
// version), so the Settings UI now prefers the version-string comparison
// over MD5 and only flags a true version difference as "out of date."
// Empty when the file has no version resource, the reference download
// failed, or the manifest's downloadprefix isn't reachable — in those
// cases callers should fall back to MD5.
type ManifestEntry struct {
	Name           string `yaml:"name" json:"name"`
	MD5            string `yaml:"md5" json:"md5"`
	Date           string `yaml:"date" json:"date"` // YYYYMMDD
	Size           int64  `yaml:"size" json:"size"`
	RefFileVersion string `yaml:"-" json:"ref_file_version,omitempty"`
}

// Manifest mirrors the YAML structure exactly. We only read the fields we
// need; extra fields (download_prefix, the actual patch URL prefix used by
// eqemupatcher.exe) are kept for completeness but ignored by callers.
type Manifest struct {
	Version         string          `yaml:"version" json:"version"`
	DownloadPrefix  string          `yaml:"downloadprefix" json:"download_prefix,omitempty"`
	Downloads       []ManifestEntry `yaml:"downloads" json:"downloads"`
}

// FindEntry returns the ManifestEntry for the given filename (case-insensitive
// match on the base name), or nil if the manifest does not list it. eqw.dll
// is intentionally absent from the Quarm manifest because it ships with Zeal
// rather than as a patched game file — callers must tolerate nil.
func (m *Manifest) FindEntry(name string) *ManifestEntry {
	for i := range m.Downloads {
		if equalFold(m.Downloads[i].Name, name) {
			return &m.Downloads[i]
		}
	}
	return nil
}

func equalFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		ai, bi := a[i], b[i]
		if ai >= 'A' && ai <= 'Z' {
			ai += 'a' - 'A'
		}
		if bi >= 'A' && bi <= 'Z' {
			bi += 'a' - 'A'
		}
		if ai != bi {
			return false
		}
	}
	return true
}

// ManifestFetcher caches the parsed manifest in memory for a short TTL so the
// Settings page can poll cheaply without hammering GitHub. A fetch failure
// (network down, GitHub rate-limited, file moved) returns the last good cache
// entry if one exists, or an error otherwise — the API layer surfaces that as
// "unknown" status rather than 500ing.
type ManifestFetcher struct {
	url    string
	ttl    time.Duration
	client *http.Client

	mu       sync.Mutex
	cached   *Manifest
	cachedAt time.Time
	lastErr  error
}

// NewManifestFetcher returns a fetcher with a 1-hour cache TTL and a 10s HTTP
// timeout. Both are tuned for "settings panel polled occasionally" use — long
// enough that we don't re-fetch on every page open, short enough that the
// user sees updates within an hour of a Quarm patcher release.
func NewManifestFetcher() *ManifestFetcher {
	return &ManifestFetcher{
		url: ManifestURL,
		ttl: time.Hour,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Get returns the manifest, fetching over HTTP only when the cache is empty
// or stale. Concurrent callers share a single in-flight fetch via the mutex.
func (f *ManifestFetcher) Get(ctx context.Context) (*Manifest, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.cached != nil && time.Since(f.cachedAt) < f.ttl {
		return f.cached, nil
	}

	m, err := fetchManifest(ctx, f.client, f.url)
	if err != nil {
		f.lastErr = err
		// Fall back to stale cache if we have one — better to show
		// "possibly out of date" data than nothing.
		if f.cached != nil {
			return f.cached, nil
		}
		return nil, err
	}
	enrichRefFileVersions(ctx, f.client, m)
	f.cached = m
	f.cachedAt = time.Now()
	f.lastErr = nil
	return m, nil
}

// enrichRefFileVersions downloads each .dll entry's reference binary from
// the manifest's downloadprefix and fills in RefFileVersion by inspecting
// the VS_VERSION_INFO resource. Best-effort: any per-file failure (network
// error, malformed PE, missing prefix) leaves RefFileVersion empty and the
// status path falls back to MD5 comparison. We only enrich .dll entries
// because they're the only files Settings inspects today; doing every
// entry would be a ~250 MB download for no benefit.
func enrichRefFileVersions(ctx context.Context, client *http.Client, m *Manifest) {
	if m == nil || m.DownloadPrefix == "" {
		return
	}
	for i := range m.Downloads {
		e := &m.Downloads[i]
		if !strings.HasSuffix(strings.ToLower(e.Name), ".dll") {
			continue
		}
		url := m.DownloadPrefix + e.Name
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			continue
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20)) // 32 MiB cap
		resp.Body.Close()
		if err != nil || resp.StatusCode != http.StatusOK {
			continue
		}
		info, err := InspectDLLBytes(body)
		if err != nil {
			continue
		}
		e.RefFileVersion = info.FileVersion
	}
}

// LastError reports the most recent fetch failure, if any. Used by the API
// layer to surface a "manifest unreachable" hint to the UI.
func (f *ManifestFetcher) LastError() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.lastErr
}

func fetchManifest(ctx context.Context, client *http.Client, url string) (*Manifest, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch manifest: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("manifest http %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1 MiB cap; real file is ~3 KiB
	if err != nil {
		return nil, fmt.Errorf("read manifest body: %w", err)
	}
	var m Manifest
	if err := yaml.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("parse manifest yaml: %w", err)
	}
	return &m, nil
}
