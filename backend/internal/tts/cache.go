// Package tts holds the local-TTS infrastructure shared by every provider
// (pipertts, kokorotts, ...): a single content-addressed WAV cache directory
// under ~/.pq-companion, keyed by each provider so they can coexist without
// collision. Provider packages own their own cacheKey/cachePath (the hash
// input differs per provider) and call into this package for the
// directory-wide operations: clearing, age-based GC, and stats.
package tts

import (
	"os"
	"path/filepath"
	"strings"
	"time"
)

// CacheDirName is the subdirectory (under ~/.pq-companion) holding generated
// TTS WAVs, shared by every local-TTS provider.
const CacheDirName = "tts-cache"

// MaxAge is how long a cached WAV can sit unused before the daily sweep
// reclaims it. Not exposed as a config field: this cache holds a handful to a
// few hundred tiny alert phrases (single-digit MB total per provider), and
// TouchCacheFile means anything still in active use never goes idle long
// enough to hit this — only genuinely orphaned phrases (edited/deleted
// triggers) age out.
const MaxAge = 30 * 24 * time.Hour

// CacheDir returns the absolute cache directory given the app base dir
// (~/.pq-companion).
func CacheDir(baseDir string) string {
	return filepath.Join(baseDir, CacheDirName)
}

// NormalizeText canonicalizes phrase text for cache keying: trims surrounding
// whitespace and collapses internal runs of whitespace to a single space, so
// cosmetically-different-but-identical phrases share a cache entry. Case is
// preserved — it can affect pronunciation.
func NormalizeText(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

// ClearCache removes every cached WAV (from every provider) and returns how
// many files were deleted. A missing cache directory is not an error. This is
// the user-triggered "Clear TTS cache" button — unconditional, unlike the
// age-aware SweepOldCache below.
func ClearCache(baseDir string) (int, error) {
	dir := CacheDir(baseDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	removed := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".wav") {
			continue
		}
		if err := os.Remove(filepath.Join(dir, e.Name())); err == nil {
			removed++
		}
	}
	return removed, nil
}

// TouchCacheFile bumps a cached WAV's mtime to now on a cache hit. This is the
// entire LRU mechanism for GC: SweepOldCache only reclaims files whose mtime
// has aged past MaxAge, so a phrase still in active use is refreshed every
// time it's played and never goes idle long enough to be swept. Best-effort: a
// failure here (e.g. a race with a concurrent clear) must never fail the
// synthesis request it's piggybacking on.
func TouchCacheFile(path string) {
	now := time.Now()
	_ = os.Chtimes(path, now, now)
}

// SweepOldCacheAge removes cached WAVs whose mtime is older than maxAge and
// returns how many were deleted. A missing cache directory is not an error.
// Exported (rather than baking in MaxAge like SweepOldCache) so provider
// packages' tests can exercise the age boundary with a short duration.
func SweepOldCacheAge(baseDir string, maxAge time.Duration) (int, error) {
	dir := CacheDir(baseDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	cutoff := time.Now().Add(-maxAge)
	removed := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".wav") {
			continue
		}
		info, err := e.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		if err := os.Remove(filepath.Join(dir, e.Name())); err == nil {
			removed++
		}
	}
	return removed, nil
}

// SweepOldCache is the exported entry point for the daily background GC job
// (wired from cmd/server/main.go), baking in MaxAge so the caller doesn't need
// to know the retention constant. Covers every provider's entries in one pass
// since they all share CacheDir.
func SweepOldCache(baseDir string) (int, error) {
	return SweepOldCacheAge(baseDir, MaxAge)
}

// CacheStats reports the number of cached WAVs (across every provider) and
// their total size, for a Settings status card. A missing cache directory
// reports (0, 0, nil), not an error.
func CacheStats(baseDir string) (fileCount int, totalBytes int64, err error) {
	dir := CacheDir(baseDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, 0, nil
		}
		return 0, 0, err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".wav") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		fileCount++
		totalBytes += info.Size()
	}
	return fileCount, totalBytes, nil
}
