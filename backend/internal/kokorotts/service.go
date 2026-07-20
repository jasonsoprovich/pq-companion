package kokorotts

import (
	"context"
	"os"

	"github.com/jasonsoprovich/pq-companion/backend/internal/tts"
)

// Service orchestrates cached Kokoro synthesis. One instance is constructed
// at router startup (internal/api/router.go) and reused for every request,
// mirroring pipertts.Service minus the warm-worker machinery — see the
// package doc for why there's no warm mode here.
type Service struct {
	baseDir string
}

// NewService returns a Service rooted at baseDir (the ~/.pq-companion
// directory, i.e. the parent of config.yaml / user.db).
func NewService(baseDir string) *Service {
	return &Service{baseDir: baseDir}
}

// Synthesize returns the absolute path to a WAV of text in the configured
// Kokoro voice, generating and caching it on a miss. On a cache hit it
// returns immediately without spawning anything (and refreshes the entry's
// mtime so it survives the daily age-based sweep as long as it stays in
// active use). Errors (disabled, misconfigured, synth failure) are returned
// so the caller can fall back to Web Speech.
//
// force skips the cache-hit shortcut and always regenerates — used by the
// Settings "Test voice" button so a click there genuinely exercises kokoro-tts
// instead of just replaying whatever's cached.
func (s *Service) Synthesize(ctx context.Context, cfg Config, text string, force bool) (string, error) {
	if !cfg.Enabled {
		return "", errSynthUnavailable
	}
	path := cachePath(s.baseDir, cfg, text)
	if !force {
		if info, err := os.Stat(path); err == nil && info.Size() > 0 {
			tts.TouchCacheFile(path)
			return path, nil
		}
	}
	if err := synthesizeToFile(ctx, cfg, text, path); err != nil {
		return "", err
	}
	return path, nil
}

// CacheStats reports the number of cached TTS WAVs (across every provider
// sharing the tts-cache dir) and their total size, for the Settings status
// card.
func (s *Service) CacheStats() (files int, bytes int64, err error) {
	return tts.CacheStats(s.baseDir)
}

// ClearCache deletes every cached TTS WAV (across every provider sharing the
// tts-cache dir) and returns the number removed.
func (s *Service) ClearCache() (int, error) {
	return tts.ClearCache(s.baseDir)
}
