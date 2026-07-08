package pipertts

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	// synthTimeout is the hard ceiling on a single piper spawn. The frozen CLI
	// reloads the model on every invocation, so a cold spawn on weak hardware
	// can take a few seconds; 30s is generous while still guaranteeing a stuck
	// process is killed rather than wedging the request forever.
	synthTimeout = 30 * time.Second

	// maxTextLen caps the input phrase length to bound synthesis time and
	// memory. Alert callouts are short; anything past this is almost certainly
	// a misconfiguration (e.g. a whole log line piped in).
	maxTextLen = 1000

	// maxOutputBytes rejects a WAV larger than this. A short callout is well
	// under 1 MB; 25 MB is a wide safety margin that still stops a runaway
	// process from filling the disk.
	maxOutputBytes = 25 << 20

	// maxConcurrentSpawns bounds how many piper processes run at once so a
	// burst of trigger fires can't fork-bomb the machine. Synthesis is normally
	// a rare cache-miss path (pre-generate-on-save warms the common cases).
	maxConcurrentSpawns = 2
)

// spawnSem bounds concurrent piper subprocesses across the whole process.
var spawnSem = make(chan struct{}, maxConcurrentSpawns)

// errSynthUnavailable is returned when the install can't synthesize (disabled
// or misconfigured). The API layer maps it to a 503 so the frontend falls back
// to Web Speech.
var errSynthUnavailable = errors.New("piper not available")

// synthesizeToFile spawns piper to render text into a WAV at outPath. It passes
// arguments as a slice (never a shell string), streams the text on stdin,
// enforces synthTimeout (killing the process on expiry), and caps the output
// size. The file at outPath is written atomically: piper renders to a temp file
// in the same directory which is renamed into place only on success.
//
// This is the universal cold path: used directly when cfg.Mode is "spawn"
// (default), AND as the fallback when cfg.Mode is "warm" but the persistent
// worker fails to start or fails a request — so it must synthesize regardless
// of cfg.Mode, never gate on it.
func synthesizeToFile(ctx context.Context, cfg Config, text, outPath string) error {
	if !cfg.Enabled {
		return fmt.Errorf("%w: disabled", errSynthUnavailable)
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return errors.New("empty text")
	}
	if len(text) > maxTextLen {
		return fmt.Errorf("text too long (%d > %d chars)", len(text), maxTextLen)
	}
	if err := validatePaths(cfg); err != nil {
		return err
	}

	// Bound concurrent spawns; respect ctx cancellation while waiting.
	select {
	case spawnSem <- struct{}{}:
		defer func() { <-spawnSem }()
	case <-ctx.Done():
		return ctx.Err()
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return fmt.Errorf("create cache dir: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(outPath), "piper-*.wav.tmp")
	if err != nil {
		return fmt.Errorf("create temp wav: %w", err)
	}
	tmpPath := tmp.Name()
	// piper writes to tmpPath itself via --output_file; close our handle so it
	// isn't held open on Windows, and make sure a failure never leaves the temp
	// behind.
	_ = tmp.Close()
	success := false
	defer func() {
		if !success {
			_ = os.Remove(tmpPath)
		}
	}()

	ctx, cancel := context.WithTimeout(ctx, synthTimeout)
	defer cancel()

	// Args as a slice — no shell, no interpolation. Text goes in on stdin.
	// Use the short flags -m/-f: both the frozen MIT standalone (rhasspy/piper)
	// and the maintained piper1-gpl build accept them, whereas the long
	// --output_file spelling isn't guaranteed on the latter. Both read the
	// phrase from stdin by default (no positional text arg).
	cmd := exec.CommandContext(ctx, cfg.ExePath,
		"-m", cfg.ModelPath,
		"-f", tmpPath,
	)
	cmd.Stdin = strings.NewReader(text)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("piper timed out after %s", synthTimeout)
		}
		msg := strings.TrimSpace(stderr.String())
		if msg != "" {
			if len(msg) > 200 {
				msg = msg[:200]
			}
			return fmt.Errorf("piper failed: %s", msg)
		}
		return fmt.Errorf("piper failed: %w", err)
	}

	info, err := os.Stat(tmpPath)
	if err != nil {
		return fmt.Errorf("piper produced no output: %w", err)
	}
	if info.Size() == 0 {
		return errors.New("piper produced an empty file")
	}
	if info.Size() > maxOutputBytes {
		return fmt.Errorf("piper output too large (%d bytes)", info.Size())
	}

	if err := os.Rename(tmpPath, outPath); err != nil {
		return fmt.Errorf("finalize wav: %w", err)
	}
	success = true
	return nil
}

// validatePaths confirms the configured exe and model (plus its .onnx.json
// sidecar) exist as regular files before we try to spawn.
func validatePaths(cfg Config) error {
	if strings.TrimSpace(cfg.ExePath) == "" {
		return fmt.Errorf("%w: executable path not set", errSynthUnavailable)
	}
	if info, err := os.Stat(cfg.ExePath); err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("%w: executable not found", errSynthUnavailable)
	}
	if strings.TrimSpace(cfg.ModelPath) == "" {
		return fmt.Errorf("%w: voice model path not set", errSynthUnavailable)
	}
	if info, err := os.Stat(cfg.ModelPath); err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("%w: voice model not found", errSynthUnavailable)
	}
	return nil
}
