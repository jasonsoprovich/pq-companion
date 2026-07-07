package pipertts

import (
	"context"
	"os"
)

// Service orchestrates cached Piper synthesis. It owns the app base directory
// (~/.pq-companion) under which the shared tts-cache lives; the per-request
// Piper Config is passed in each call so the service holds no mutable settings
// state and always reflects the latest saved config.
type Service struct {
	baseDir string
}

// NewService returns a Service rooted at baseDir (the ~/.pq-companion
// directory, i.e. the parent of config.yaml / user.db).
func NewService(baseDir string) *Service {
	return &Service{baseDir: baseDir}
}

// Synthesize returns the absolute path to a WAV of text in the configured Piper
// voice, generating and caching it on a miss. On a cache hit it returns
// immediately without spawning anything. Errors (disabled, misconfigured, spawn
// failure) are returned so the caller can fall back to Web Speech.
func (s *Service) Synthesize(ctx context.Context, cfg Config, text string) (string, error) {
	if !cfg.Enabled {
		return "", errSynthUnavailable
	}
	path := cachePath(s.baseDir, cfg, text)
	if info, err := os.Stat(path); err == nil && info.Size() > 0 {
		return path, nil
	}
	if err := synthesizeToFile(ctx, cfg, text, path); err != nil {
		return "", err
	}
	return path, nil
}

// ClearCache deletes all cached TTS WAVs and returns the number removed.
func (s *Service) ClearCache() (int, error) {
	return clearCache(s.baseDir)
}
