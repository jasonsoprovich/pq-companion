package pipertts

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

// warmScratchDirName is the sibling-to-tts-cache directory where warm workers
// write their raw (nanosecond-named) output before it's renamed into the real
// content-addressed cache. Kept separate from tts-cache so it can be wiped
// unconditionally at process start (any leftovers there are crash debris —
// tts-cache itself must never be touched that way, it's the real cache).
const warmScratchDirName = "piper-warm-scratch"

// Service orchestrates cached Piper synthesis, and — when a trigger/alert
// requests "warm" mode — owns the single persistent warm worker process for
// the process's lifetime. One Service instance is constructed at router
// startup (internal/api/router.go) and reused for every request, which is
// what gives the warm worker a life longer than a single HTTP call.
type Service struct {
	baseDir     string
	scratchRoot string

	mu   sync.Mutex
	warm *warmWorker
}

// NewService returns a Service rooted at baseDir (the ~/.pq-companion
// directory, i.e. the parent of config.yaml / user.db). Best-effort wipes any
// stale warm-scratch directory from a previous run (crash debris) — safe here
// specifically because no worker exists yet at construction time.
func NewService(baseDir string) *Service {
	s := &Service{
		baseDir:     baseDir,
		scratchRoot: filepath.Join(baseDir, warmScratchDirName),
	}
	_ = os.RemoveAll(s.scratchRoot)
	return s
}

// Synthesize returns the absolute path to a WAV of text in the configured
// Piper voice, generating and caching it on a miss. On a cache hit it returns
// immediately without spawning anything (and refreshes the entry's mtime —
// see cache.go's touchCacheFile — so it survives the daily age-based sweep as
// long as it stays in active use). Errors (disabled, misconfigured, synth
// failure) are returned so the caller can fall back to Web Speech.
//
// force skips the cache-hit shortcut and always regenerates, overwriting the
// cache entry with the fresh result. The cache key is mode-independent (a
// phrase generated once should be replayable regardless of which mode
// produced it), which means normal cache hits never exercise the currently
// configured mode's live synthesis path — force exists specifically for the
// Settings "Test voice" button, so a click there genuinely proves the
// selected mode is working instead of just replaying whatever's cached.
func (s *Service) Synthesize(ctx context.Context, cfg Config, text string, force bool) (string, error) {
	if !cfg.Enabled {
		return "", errSynthUnavailable
	}
	path := cachePath(s.baseDir, cfg, text)
	if !force {
		if info, err := os.Stat(path); err == nil && info.Size() > 0 {
			touchCacheFile(path)
			return path, nil
		}
	}

	if cfg.Mode == "warm" {
		if warmPath, err := s.synthesizeWarm(ctx, cfg, text, path); err == nil {
			return warmPath, nil
		} else {
			slog.Warn("piper warm synth failed, falling back to spawn", "err", err)
		}
	}

	if err := synthesizeToFile(ctx, cfg, text, path); err != nil {
		return "", err
	}
	return path, nil
}

// synthesizeWarm resolves the current warm worker for cfg (starting/replacing
// it if needed) and asks it to synthesize text, then moves its raw scratch
// output into the real content-addressed cache location. Any failure here is
// non-fatal to the caller — Synthesize falls back to the cold path.
func (s *Service) synthesizeWarm(ctx context.Context, cfg Config, text, outPath string) (string, error) {
	w, err := s.resolveWarmWorker(cfg)
	if err != nil {
		return "", err
	}
	rawPath, err := w.synthesize(ctx, text)
	if err != nil {
		return "", err
	}
	if err := os.Rename(rawPath, outPath); err != nil {
		return "", fmt.Errorf("finalize warm wav: %w", err)
	}
	return outPath, nil
}

// resolveWarmWorker returns the current warm worker matching cfg's identity,
// replacing it first if missing, mismatched, or dead. The entire
// build-and-install step runs under s.mu — newWarmWorker (an MkdirTemp call)
// and warmWorker.start (a fork/exec that does NOT wait for piper's own model
// load) are both fast, non-blocking operations, so holding the lock across
// them is cheap and — importantly — makes this whole function race-free
// without needing double-checked locking: only one goroutine can ever be
// replacing the worker for a given identity change at a time. (On Windows,
// antivirus scanning an unfamiliar .exe on first launch could occasionally
// make Start() slower than usual; that's a bounded, one-time cost per
// identity change, not per request, and is judged an acceptable trade for the
// simplicity here.)
//
// The replaced worker's stop() — which can take up to stopGrace — always runs
// AFTER s.mu is released, so it never stalls concurrent Synthesize/status
// calls.
func (s *Service) resolveWarmWorker(cfg Config) (*warmWorker, error) {
	s.mu.Lock()
	if s.warm != nil && s.warm.matches(cfg) && !s.warm.dead.Load() {
		w := s.warm
		s.mu.Unlock()
		return w, nil
	}
	old := s.warm
	slog.Debug("piper warm: (re)spawning worker", "had_previous", old != nil,
		"previous_dead", old != nil && old.dead.Load())

	nw, buildErr := newWarmWorker(cfg.ExePath, cfg.ModelPath, s.scratchRoot)
	if buildErr == nil {
		if startErr := nw.start(); startErr != nil {
			buildErr = startErr
		}
	}

	var result *warmWorker
	if buildErr == nil {
		result = nw
	}
	s.warm = result
	s.mu.Unlock()

	if old != nil {
		old.stop()
	}
	if buildErr != nil {
		if nw != nil {
			nw.stop() // release its scratch dir even though it never fully started
		}
		return nil, buildErr
	}
	return result, nil
}

// ReconcileWarm stops a running warm worker if the current config no longer
// wants one for this identity (Piper disabled, mode switched away from
// "warm", or the exe/model path changed) — but NEVER starts one; starting
// only ever happens lazily inside Synthesize on an actual TTS request. Called
// as a side effect of the status endpoint, which the frontend already
// re-polls on every config:updated event, so a stale warm worker doesn't
// linger holding memory after the user reconfigures Piper. Safe to call on
// every poll: cfg is a stable snapshot from config.Manager.Get(), so an
// unrelated config edit reads the same piper identity and this is a cheap
// no-op — it only tears down on an actual piper-relevant change.
func (s *Service) ReconcileWarm(cfg Config) {
	s.mu.Lock()
	if s.warm == nil {
		s.mu.Unlock()
		return
	}
	if cfg.Enabled && cfg.Mode == "warm" && s.warm.matches(cfg) && !s.warm.dead.Load() {
		s.mu.Unlock()
		return
	}
	old := s.warm
	s.warm = nil
	s.mu.Unlock()
	old.stop()
}

// WarmStatus reports whether a warm worker is currently alive and its most
// recent failure (if any), for the Settings status card.
func (s *Service) WarmStatus() (running bool, lastErr string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.warm == nil {
		return false, ""
	}
	return !s.warm.dead.Load(), s.warm.lastError()
}

// CacheStats reports the number of cached TTS WAVs and their total size, for
// the Settings status card.
func (s *Service) CacheStats() (files int, bytes int64, err error) {
	return cacheStats(s.baseDir)
}

// ClearCache deletes all cached TTS WAVs and returns the number removed.
func (s *Service) ClearCache() (int, error) {
	return clearCache(s.baseDir)
}
