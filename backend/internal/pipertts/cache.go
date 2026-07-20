package pipertts

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/tts"
)

// cacheDirName re-exports tts.CacheDirName for this package's call sites
// (including pipertts_test.go, which predates the internal/tts extraction).
const cacheDirName = tts.CacheDirName

// cacheDir re-exports tts.CacheDir for this package's call sites.
func cacheDir(baseDir string) string {
	return tts.CacheDir(baseDir)
}

// normalizeText re-exports tts.NormalizeText for this package's call sites.
func normalizeText(text string) string {
	return tts.NormalizeText(text)
}

// cacheKey is the content-addressed hash for a (voice, text) pair. It is
// namespaced with a "piper" prefix and the model path so the same shared
// tts-cache dir (internal/tts) can hold multiple providers/voices without
// collision.
func cacheKey(cfg Config, text string) string {
	sum := sha256.Sum256([]byte("piper\x00" + cfg.ModelPath + "\x00" + normalizeText(text)))
	return hex.EncodeToString(sum[:])
}

// cachePath returns the WAV path for a (voice, text) pair. It does not create
// the file or check for its existence.
func cachePath(baseDir string, cfg Config, text string) string {
	return filepath.Join(tts.CacheDir(baseDir), cacheKey(cfg, text)+".wav")
}

// touchCacheFile re-exports tts.TouchCacheFile for this package's call sites.
func touchCacheFile(path string) {
	tts.TouchCacheFile(path)
}

// clearCache re-exports tts.ClearCache for this package's call sites.
func clearCache(baseDir string) (int, error) {
	return tts.ClearCache(baseDir)
}

// cacheStats re-exports tts.CacheStats for this package's call sites.
func cacheStats(baseDir string) (fileCount int, totalBytes int64, err error) {
	return tts.CacheStats(baseDir)
}

// sweepOldCache re-exports tts.SweepOldCacheAge for pipertts_test.go, which
// predates the internal/tts extraction and exercises the age boundary with a
// custom (short) maxAge.
func sweepOldCache(baseDir string, maxAge time.Duration) (int, error) {
	return tts.SweepOldCacheAge(baseDir, maxAge)
}
