// Package applog wires slog to both stderr and a rotated log file under
// ~/.pq-companion/logs/. This exists so end users with weird failure modes
// (AV ACLs, OneDrive placeholders, exotic filesystems) can hand back a real
// log instead of "the items tab is broken." Packaged Electron builds on
// Windows have no attached console, so stderr alone is lost.
package applog

import (
	"archive/zip"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
)

// keepSessions is how many prior server.log files to retain. Each app launch
// rotates server.log → server.1.log → server.2.log → ... → dropped. Three is
// enough to capture "it broke after I updated" without filling disk.
const keepSessions = 3

// levelVar is the live log level shared by whichever handler is installed.
// Defaults to Info; SetDebug flips it to Debug at runtime so verbose
// diagnostics (tailer reads, trigger dedup drops, …) can be turned on from
// Settings without restarting the backend. Both the file+stderr handler and
// the stderr-only fallback reference this same var, so a single Set call
// re-levels every sink.
var levelVar = new(slog.LevelVar)

// SetDebug raises the log level to Debug (verbose) when enabled, or lowers it
// back to Info when disabled. Safe to call at any time — slog.LevelVar is
// goroutine-safe and the change takes effect immediately for all handlers.
// Driven by Preferences.DebugLogging (applied on startup and on config save).
func SetDebug(enabled bool) {
	if enabled {
		levelVar.Set(slog.LevelDebug)
		slog.Info("verbose (debug) logging enabled")
		return
	}
	if levelVar.Level() == slog.LevelDebug {
		slog.Info("verbose (debug) logging disabled")
	}
	levelVar.Set(slog.LevelInfo)
}

// Init wires slog.Default to a TextHandler that writes to stderr and (if a
// log dir can be created) to ~/.pq-companion/logs/server.log. Returns the
// resolved log file path, an io.Closer for the file handle (or nil), and any
// error from opening the file. A non-nil error is non-fatal — the caller
// should still proceed; slog will at least keep writing to stderr.
func Init(appVersion string) (logPath string, closer io.Closer, err error) {
	logDir, dirErr := logsDir()
	if dirErr != nil {
		setStderrOnly()
		return "", nil, fmt.Errorf("resolve logs dir: %w", dirErr)
	}
	if mkErr := os.MkdirAll(logDir, 0o755); mkErr != nil {
		setStderrOnly()
		return "", nil, fmt.Errorf("mkdir logs dir: %w", mkErr)
	}

	logPath = filepath.Join(logDir, "server.log")
	rotate(logPath, keepSessions)

	f, openErr := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if openErr != nil {
		setStderrOnly()
		return logPath, nil, fmt.Errorf("open log file: %w", openErr)
	}

	mw := io.MultiWriter(os.Stderr, f)
	handler := slog.NewTextHandler(mw, &slog.HandlerOptions{
		Level: levelVar,
	})
	slog.SetDefault(slog.New(handler))

	slog.Info("server starting",
		"version", appVersion,
		"go", runtime.Version(),
		"os", runtime.GOOS,
		"arch", runtime.GOARCH,
		"pid", os.Getpid(),
		"log_file", logPath,
	)
	if exe, exeErr := os.Executable(); exeErr == nil {
		slog.Info("executable path", "exe", exe)
	}
	if wd, wdErr := os.Getwd(); wdErr == nil {
		slog.Info("working directory", "cwd", wd)
	}
	return logPath, f, nil
}

// setStderrOnly installs a stderr-only handler. Called when file logging
// can't be set up so we still get consistent log formatting in dev.
func setStderrOnly() {
	handler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: levelVar,
	})
	slog.SetDefault(slog.New(handler))
}

// logsDir returns ~/.pq-companion/logs.
func logsDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".pq-companion", "logs"), nil
}

// ExportDebugLogs zips every file directly under ~/.pq-companion/logs/
// (server*.log written by this process, electron*.log written by the
// Electron main process — both land in the same directory) into a single
// archive at destPath, so a user can share one small file for a bug report
// instead of hunting down and attaching several. Returns destPath on success.
func ExportDebugLogs(destPath string) (string, error) {
	dir, err := logsDir()
	if err != nil {
		return "", fmt.Errorf("resolve logs dir: %w", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("read logs dir: %w", err)
	}

	out, err := os.Create(destPath)
	if err != nil {
		return "", fmt.Errorf("create export file: %w", err)
	}
	defer func() { _ = out.Close() }()

	w := zip.NewWriter(out)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if err := addLogFileToZip(w, filepath.Join(dir, e.Name()), e.Name()); err != nil {
			w.Close()
			return "", fmt.Errorf("add %s to zip: %w", e.Name(), err)
		}
	}
	if err := w.Close(); err != nil {
		return "", fmt.Errorf("finalize zip: %w", err)
	}
	if err := out.Sync(); err != nil {
		return "", fmt.Errorf("sync export file: %w", err)
	}
	return destPath, nil
}

func addLogFileToZip(w *zip.Writer, src, entryName string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	info, err := in.Stat()
	if err != nil {
		return err
	}

	hdr, err := zip.FileInfoHeader(info)
	if err != nil {
		return err
	}
	hdr.Name = entryName
	hdr.Method = zip.Deflate

	entry, err := w.CreateHeader(hdr)
	if err != nil {
		return err
	}
	_, err = io.Copy(entry, in)
	return err
}

// rotate renames server.log → server.1.log → server.2.log → ..., dropping
// anything past `keep`. Called once at startup so each launch begins with a
// fresh log file — easier to attach to a bug report than a grep-by-timestamp
// search through a single rolling file.
func rotate(base string, keep int) {
	for i := keep; i >= 1; i-- {
		src := numbered(base, i-1)
		dst := numbered(base, i)
		if i == keep {
			_ = os.Remove(dst)
		}
		if _, err := os.Stat(src); err == nil {
			_ = os.Rename(src, dst)
		}
	}
}

// numbered returns base for n==0, or base with ".N" inserted before the
// extension for n>=1: server.log → server.log, server.1.log, server.2.log.
func numbered(base string, n int) string {
	if n == 0 {
		return base
	}
	ext := filepath.Ext(base)
	stem := base[:len(base)-len(ext)]
	return fmt.Sprintf("%s.%d%s", stem, n, ext)
}
