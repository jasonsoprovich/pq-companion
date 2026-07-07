package pipertts

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
)

// cacheDirName is the subdirectory (under ~/.pq-companion) holding generated
// TTS WAVs. It is deliberately provider-neutral ("tts-cache") so a future cloud
// TTS provider can share the same directory — keys are namespaced by provider.
const cacheDirName = "tts-cache"

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
// A missing cache directory is not an error (nothing to clear).
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
