package pipertts

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// cacheDirName is the subdirectory (under ~/.pq-companion) holding generated
// TTS WAVs. It is deliberately provider-neutral ("tts-cache") so a future cloud
// TTS provider can share the same directory — keys are namespaced by provider.
const cacheDirName = "tts-cache"

// cacheMaxAge is how long a cached WAV can sit unused before the daily sweep
// (see SweepOldCache, wired from cmd/server/main.go) reclaims it. Not exposed
// as a config field: this cache holds a handful to a few hundred tiny alert
// phrases (single-digit MB total), and touchCacheFile below means anything
// still in active use never goes idle long enough to hit this — only
// genuinely orphaned phrases (edited/deleted triggers) age out. Mirrors the
// hardcoded-constant style of similar retention knobs elsewhere in this app,
// just without a config override since there's no practical need for one.
const cacheMaxAge = 30 * 24 * time.Hour

// cacheDir returns the absolute cache directory given the app base dir
// (~/.pq-companion).
func cacheDir(baseDir string) string {
	return filepath.Join(baseDir, cacheDirName)
}

// normalizeText canonicalizes phrase text for cache keying: trims surrounding
// whitespace and collapses internal runs of whitespace to a single space, so
// cosmetically-different-but-identical phrases share a cache entry. Case is
// preserved — it can affect pronunciation.
func normalizeText(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

// cacheKey is the content-addressed hash for a (voice, text) pair. It is
// namespaced with a "piper" prefix and the model path so the same tts-cache dir
// can hold multiple providers/voices without collision.
func cacheKey(cfg Config, text string) string {
	sum := sha256.Sum256([]byte("piper\x00" + cfg.ModelPath + "\x00" + normalizeText(text)))
	return hex.EncodeToString(sum[:])
}

// cachePath returns the WAV path for a (voice, text) pair. It does not create
// the file or check for its existence.
func cachePath(baseDir string, cfg Config, text string) string {
	return filepath.Join(cacheDir(baseDir), cacheKey(cfg, text)+".wav")
}

// clearCache removes every cached WAV and returns how many files were deleted.
// A missing cache directory is not an error (nothing to clear). This is the
// user-triggered "Clear TTS cache" button — unconditional, unlike the
// age-aware sweepOldCache below.
func clearCache(baseDir string) (int, error) {
	dir := cacheDir(baseDir)
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

// touchCacheFile bumps a cached WAV's mtime to now on a cache hit. This is the
// entire LRU mechanism for GC: sweepOldCache only reclaims files whose mtime
// has aged past cacheMaxAge, so a phrase still in active use is refreshed
// every time it's played and never goes idle long enough to be swept — only
// genuinely orphaned entries (from edited/deleted triggers) age out.
// Best-effort: a failure here (e.g. a race with a concurrent clear) must never
// fail the synthesis request it's piggybacking on.
func touchCacheFile(path string) {
	now := time.Now()
	_ = os.Chtimes(path, now, now)
}

// sweepOldCache removes cached WAVs whose mtime is older than maxAge and
// returns how many were deleted. A missing cache directory is not an error.
// Unlike clearCache, this only removes files that appear genuinely unused —
// see touchCacheFile.
func sweepOldCache(baseDir string, maxAge time.Duration) (int, error) {
	dir := cacheDir(baseDir)
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
// (wired from cmd/server/main.go), baking in cacheMaxAge so the caller doesn't
// need to know the retention constant or reach through a Service instance.
func SweepOldCache(baseDir string) (int, error) {
	return sweepOldCache(baseDir, cacheMaxAge)
}

// cacheStats reports the number of cached WAVs and their total size, for the
// Settings card. A missing cache directory reports (0, 0, nil), not an error.
func cacheStats(baseDir string) (fileCount int, totalBytes int64, err error) {
	dir := cacheDir(baseDir)
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
