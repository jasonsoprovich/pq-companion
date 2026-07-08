package pipertts

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// stopGrace is how long stop() waits for a clean (stdin-EOF-triggered) exit
// before force-killing the process.
const stopGrace = 2 * time.Second

// Polling parameters for waitForOutputFile. See its doc for why polling the
// scratch directory — rather than parsing anything the child prints — is the
// completion-detection mechanism.
const (
	pollInterval   = 50 * time.Millisecond
	stabilizeDelay = 20 * time.Millisecond
)

// warmWorker owns one persistent piper subprocess, spawned with --output_dir
// instead of -f so it loads its voice model once and then loops on stdin
// indefinitely: one phrase in, one WAV file written per line, repeatedly,
// until stdin closes (at which point piper's own stdin-read loop sees EOF and
// exits cleanly on its own — see the package doc for why this means no
// app-wide graceful-shutdown machinery is needed for the abnormal-exit case).
//
// Lock ordering: Service.mu is always outermost and is never held while
// calling into a worker. A worker's own mu is only ever taken inside
// synthesize(), which serializes callers (piper's stdin loop is strictly
// sequential — one in-flight request at a time is all the process can do
// anyway). stop() never takes mu, so it can never deadlock against a slow
// in-flight synthesize(); that call just gets a write error or eventually
// observes the process exit instead, and marks itself dead on its own.
type warmWorker struct {
	exePath, modelPath string
	scratchDir         string // unique per worker (os.MkdirTemp); removed in stop()

	cmd   *exec.Cmd
	stdin io.WriteCloser
	errPr *os.File // our read end of the child's stderr

	closed chan struct{} // closed exactly once, by waitForExit, when the process exits

	mu   sync.Mutex  // serializes synthesize() calls
	dead atomic.Bool // set on any protocol failure; a dead worker is never reused

	errMu   sync.Mutex
	lastErr string

	stopOnce sync.Once
}

// newWarmWorker allocates a worker identified by (exePath, modelPath) and
// creates its unique scratch subdirectory under scratchRoot. Does not spawn
// the process yet — call start() for that.
func newWarmWorker(exePath, modelPath, scratchRoot string) (*warmWorker, error) {
	if err := os.MkdirAll(scratchRoot, 0o755); err != nil {
		return nil, fmt.Errorf("create warm scratch root: %w", err)
	}
	dir, err := os.MkdirTemp(scratchRoot, "warm-*")
	if err != nil {
		return nil, fmt.Errorf("create warm worker scratch dir: %w", err)
	}
	return &warmWorker{
		exePath:    exePath,
		modelPath:  modelPath,
		scratchDir: dir,
		closed:     make(chan struct{}),
	}, nil
}

// matches reports whether this worker was built for the given config's
// (exePath, modelPath) identity.
func (w *warmWorker) matches(cfg Config) bool {
	return w.exePath == cfg.ExePath && w.modelPath == cfg.ModelPath
}

// start spawns the persistent piper process (args as a slice, matching
// synth.go's convention) and wires it up via startWithCommand.
func (w *warmWorker) start() error {
	if err := validatePaths(Config{ExePath: w.exePath, ModelPath: w.modelPath}); err != nil {
		return err
	}
	cmd := exec.Command(w.exePath, "-m", w.modelPath, "--output_dir", w.scratchDir)
	return w.startWithCommand(cmd)
}

// startWithCommand does the actual pipe-wiring and process spawn for the
// given, not-yet-started *exec.Cmd. Split out from start() so tests can drive
// this same lifecycle logic with a fake subprocess (a self-exec'd test
// helper) instead of a real piper binary.
//
// Stdout is discarded (cmd.Stdout left nil, which os/exec connects to
// /dev/null): completion is detected by polling the scratch directory (see
// waitForOutputFile), NOT by parsing anything the child prints. This was a
// deliberate correction — the C++ standalone build prints a bare output path
// to stdout, but at least one Python-based piper distribution instead logs
// its "Wrote <path>" message to STDERR via Python's logging module. Chasing
// every build's differing stdout convention isn't worth it; watching the
// filesystem for the actual output file works identically regardless of
// which piper distribution is configured.
//
// Stderr is still captured (via a manual os.Pipe, not cmd.StderrPipe, so
// stop() can reap the process without violating the "must finish reading
// before Wait" contract StderrPipe carries) purely as a diagnostic source for
// WarmStatus's error message.
func (w *warmWorker) startWithCommand(cmd *exec.Cmd) error {
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("warm worker stdin pipe: %w", err)
	}
	cmd.Stdout = nil // discarded — see completion-detection note above

	errR, errW, err := os.Pipe()
	if err != nil {
		return fmt.Errorf("warm worker stderr pipe: %w", err)
	}
	cmd.Stderr = errW

	spawnStart := time.Now()
	if err := cmd.Start(); err != nil {
		_ = errR.Close()
		_ = errW.Close()
		return fmt.Errorf("start piper: %w", err)
	}
	slog.Debug("piper warm: process spawned", "spawn_took", time.Since(spawnStart), "path", w.exePath, "model", w.modelPath)
	// Close our copy of the child's stderr write end now that it has its own
	// (duped across fork/exec) — this is what makes EOF propagate to
	// drainStderr exactly when the child exits or its handle closes.
	_ = errW.Close()

	w.cmd = cmd
	w.stdin = stdin
	w.errPr = errR

	go w.drainStderr()
	go w.waitForExit()
	return nil
}

// waitForExit is the single owner of cmd.Wait() for this worker's lifetime
// (Wait must be called exactly once per *exec.Cmd) — it blocks until the
// process exits for any reason (clean stdin-EOF exit, crash, or a Kill() from
// stop()) and closes `closed`, which both synthesize() (to react immediately
// to a dead process instead of waiting out the full timeout) and stop() (to
// know the process has actually been reaped) select on.
func (w *warmWorker) waitForExit() {
	err := w.cmd.Wait()
	if err != nil {
		w.errMu.Lock()
		if w.lastErr == "" {
			// Only used as a last resort — a captured stderr line (see
			// drainStderr) is almost always more informative than a bare exit
			// status, so it takes priority whenever one was captured.
			w.lastErr = err.Error()
		}
		w.errMu.Unlock()
	}
	close(w.closed)
}

// drainStderr keeps the most recent non-empty stderr line as a diagnostic
// candidate for lastErr, surfaced when the process dies unexpectedly. Never
// blocks a synthesize() call.
func (w *warmWorker) drainStderr() {
	reader := bufio.NewReader(w.errPr)
	for {
		line, err := reader.ReadString('\n')
		if line = strings.TrimSpace(line); line != "" {
			if len(line) > 200 {
				line = line[:200]
			}
			w.errMu.Lock()
			w.lastErr = line
			w.errMu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

// synthesize sends one phrase to the warm process and returns the raw
// scratch-dir path piper produced (unrenamed — the caller owns moving it into
// the content-addressed cache, mirroring synth.go's synthesizeToFile taking an
// explicit outPath rather than owning cache-key logic). On any failure the
// worker is marked dead and must not be reused; the caller falls back to the
// cold synthesizeToFile path.
func (w *warmWorker) synthesize(ctx context.Context, text string) (string, error) {
	if w.dead.Load() {
		return "", errors.New("piper warm worker is not running")
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", errors.New("empty text")
	}
	if len(text) > maxTextLen {
		return "", fmt.Errorf("text too long (%d > %d chars)", len(text), maxTextLen)
	}

	w.mu.Lock()
	defer w.mu.Unlock()

	if w.dead.Load() {
		return "", errors.New("piper warm worker is not running")
	}

	// Snapshot the scratch dir's current contents before writing, so
	// waitForOutputFile can unambiguously identify the NEW file this request
	// produces — self-sufficient, not reliant on the caller having already
	// moved a previous result out of the scratch dir before calling again.
	before := w.scratchDirSnapshot()

	// piper reads with getline: an embedded newline would desync the stream
	// permanently, so feed it the same normalized single-line text the cache
	// key is already computed from.
	writeStart := time.Now()
	if _, err := io.WriteString(w.stdin, normalizeText(text)+"\n"); err != nil {
		w.markDead(fmt.Errorf("write to piper: %w", err))
		return "", err
	}
	slog.Debug("piper warm: wrote phrase, waiting for output file", "write_took", time.Since(writeStart))

	waitStart := time.Now()
	rawPath, err := w.waitForOutputFile(ctx, before)
	if err != nil {
		slog.Warn("piper warm: failed waiting for output", "waited", time.Since(waitStart), "err", err, "last_stderr", w.lastError())
		w.markDead(err)
		return "", err
	}
	slog.Debug("piper warm: output file ready", "waited", time.Since(waitStart), "path", rawPath)

	info, err := os.Stat(rawPath)
	if err != nil {
		w.markDead(fmt.Errorf("piper warm worker output missing: %w", err))
		return "", fmt.Errorf("piper warm worker output missing: %w", err)
	}
	if info.Size() > maxOutputBytes {
		err := fmt.Errorf("piper warm worker output too large (%d bytes)", info.Size())
		w.markDead(err)
		return "", err
	}
	return rawPath, nil
}

// scratchDirSnapshot returns the set of filenames currently in the scratch
// directory, for waitForOutputFile to diff against. A read failure yields an
// empty (non-nil) set — the safe direction to fail in is treating everything
// found afterward as "new" rather than silently ignoring a real result.
func (w *warmWorker) scratchDirSnapshot() map[string]bool {
	entries, err := os.ReadDir(w.scratchDir)
	seen := make(map[string]bool, len(entries))
	if err != nil {
		return seen
	}
	for _, e := range entries {
		seen[e.Name()] = true
	}
	return seen
}

// waitForOutputFile polls the worker's scratch directory for a WAV file that
// wasn't present in `before` (a snapshot taken right before writing to
// stdin), rather than parsing anything the child process prints — see
// startWithCommand's doc for why that's the more robust choice across
// different piper distributions' differing stdout/stderr conventions.
func (w *warmWorker) waitForOutputFile(ctx context.Context, before map[string]bool) (string, error) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	deadline := time.After(synthTimeout)
	for {
		select {
		case <-w.closed:
			msg := w.lastError()
			if msg == "" {
				msg = "piper warm worker exited unexpectedly"
			}
			return "", errors.New(msg)
		case <-ctx.Done():
			return "", ctx.Err()
		case <-deadline:
			return "", fmt.Errorf("piper warm worker timed out after %s", synthTimeout)
		case <-ticker.C:
			if path, ok := w.findStableFile(before); ok {
				return path, nil
			}
		}
	}
}

// findStableFile returns the first .wav file in the scratch directory — not
// already present in `before` — whose size is unchanged across two reads a
// short interval apart (i.e. piper has finished writing it, not still
// mid-write).
func (w *warmWorker) findStableFile(before map[string]bool) (string, bool) {
	entries, err := os.ReadDir(w.scratchDir)
	if err != nil {
		return "", false
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".wav") {
			continue
		}
		if before[e.Name()] {
			continue // pre-existing file from an earlier request, not this one's output
		}
		path := filepath.Join(w.scratchDir, e.Name())
		info1, err := os.Stat(path)
		if err != nil || info1.Size() == 0 {
			continue
		}
		time.Sleep(stabilizeDelay)
		info2, err := os.Stat(path)
		if err != nil || info2.Size() != info1.Size() {
			continue // still being written
		}
		return path, true
	}
	return "", false
}

// markDead flags the worker as unusable and records err for WarmStatus.
func (w *warmWorker) markDead(err error) {
	w.dead.Store(true)
	if err != nil {
		w.errMu.Lock()
		w.lastErr = err.Error()
		w.errMu.Unlock()
	}
}

func (w *warmWorker) lastError() string {
	w.errMu.Lock()
	defer w.errMu.Unlock()
	return w.lastErr
}

// stop tears the worker down: closes stdin (triggering piper's own clean
// EOF-exit), waits up to stopGrace for waitForExit to observe that exit,
// force-kills if it hasn't happened by then, then releases the remaining
// pipe handle and scratch directory. Idempotent (safe to call more than
// once, or on a worker that failed to start) and never takes w.mu — see the
// type doc for why that matters. Never calls cmd.Wait() directly — that's
// waitForExit's job, and Wait must only ever be called once per *exec.Cmd.
func (w *warmWorker) stop() {
	w.stopOnce.Do(func() {
		if w.stdin != nil {
			_ = w.stdin.Close()
		}
		if w.cmd != nil && w.cmd.Process != nil {
			select {
			case <-w.closed:
				// Already exited (e.g. crashed, or exited cleanly from the stdin
				// close above racing ahead of this select).
			case <-time.After(stopGrace):
				_ = w.cmd.Process.Kill()
				<-w.closed // waitForExit always closes this once Wait() returns
			}
		}
		if w.errPr != nil {
			_ = w.errPr.Close()
		}
		if w.scratchDir != "" {
			_ = os.RemoveAll(w.scratchDir)
		}
	})
}
