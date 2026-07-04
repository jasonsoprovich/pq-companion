package eqw

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// LatestReleaseURL is the GitHub shortcut that 302s to the most recently
// tagged EQW-TAKP release. As with the Zeal fetcher we HEAD this URL and read
// the Location header instead of hitting the JSON releases API, to stay well
// under GitHub's 60/hour unauthenticated rate limit per user IP.
const LatestReleaseURL = "https://github.com/CoastalRedwood/eqw_takp/releases/latest"

// LatestFetcher caches the latest EQW-TAKP version string in memory for a TTL
// so the Settings panel can poll cheaply. Fetch failures fall back to the last
// good cache entry — the UI surfaces "unknown" rather than 500ing. This mirrors
// zeal.LatestFetcher (same author's repo, same release-tag layout).
type LatestFetcher struct {
	url    string
	ttl    time.Duration
	client *http.Client

	mu       sync.Mutex
	cached   string
	cachedAt time.Time
}

// NewLatestFetcher returns a fetcher with a 6-hour cache TTL and a 10s HTTP
// timeout. EQW-TAKP cuts releases infrequently, so a slow refresh cadence is
// plenty.
func NewLatestFetcher() *LatestFetcher {
	return &LatestFetcher{
		url: LatestReleaseURL,
		ttl: 6 * time.Hour,
		client: &http.Client{
			Timeout: 10 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
	}
}

// Get returns the latest known EQW-TAKP version (e.g. "1.0.2") or "" when we
// can't determine it. A blank result is not an error from the caller's
// perspective — the UI simply skips the "update available" notice.
func (f *LatestFetcher) Get(ctx context.Context) string {
	f.mu.Lock()
	defer f.mu.Unlock()

	if f.cached != "" && time.Since(f.cachedAt) < f.ttl {
		return f.cached
	}

	v, err := fetchLatest(ctx, f.client, f.url)
	if err != nil {
		return f.cached
	}
	f.cached = v
	f.cachedAt = time.Now()
	return v
}

func fetchLatest(ctx context.Context, client *http.Client, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("fetch latest: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 300 || resp.StatusCode >= 400 {
		return "", fmt.Errorf("latest http %d", resp.StatusCode)
	}
	loc := resp.Header.Get("Location")
	if loc == "" {
		return "", errors.New("no Location header on redirect")
	}
	return extractTagVersion(loc), nil
}

// extractTagVersion pulls a MAJOR.MINOR.PATCH-ish version string out of a
// GitHub /releases/tag/<tag> URL. A leading "v" on the tag is stripped.
func extractTagVersion(location string) string {
	const marker = "/releases/tag/"
	idx := strings.Index(location, marker)
	if idx < 0 {
		return ""
	}
	tag := location[idx+len(marker):]
	if i := strings.IndexAny(tag, "/?#"); i >= 0 {
		tag = tag[:i]
	}
	tag = strings.TrimSpace(tag)
	tag = strings.TrimPrefix(tag, "v")
	tag = strings.TrimPrefix(tag, "V")
	return tag
}
