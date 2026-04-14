package logparser

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
)

const (
	pollInterval = 250 * time.Millisecond
	// maxReadPerTick caps how many bytes are consumed in a single poll cycle to
	// avoid blocking the goroutine when the file is very large on first open.
	maxReadPerTick = 1 << 20 // 1 MiB
)

// Status describes the current state of the Tailer.
type Status struct {
	// Enabled is true when ParseCombatLog is set in config.
	Enabled bool `json:"enabled"`
	// FilePath is the log file path derived from the current config.
	FilePath string `json:"file_path"`
	// FileExists reports whether the log file was found on disk.
	FileExists bool `json:"file_exists"`
	// Tailing is true when the file is currently open and being read.
	Tailing bool `json:"tailing"`
	// Offset is the byte position the tailer has read up to.
	Offset int64 `json:"offset"`
}

// Tailer watches an EverQuest log file and calls handler for each parsed
// LogEvent it finds in newly-appended lines.
//
// It reacts to config changes: when eq_path or character changes the tailer
// closes the old file and begins tailing the new one (seeking to the end so
// historical lines are not replayed).
type Tailer struct {
	cfgMgr  *config.Manager
	handler func(LogEvent)

	mu        sync.Mutex
	file      *os.File
	filePath  string
	offset    int64
	remainder []byte // incomplete line fragment from the previous read
}

// NewTailer creates a Tailer. Call Start in a goroutine to begin tailing.
func NewTailer(cfgMgr *config.Manager, handler func(LogEvent)) *Tailer {
	return &Tailer{cfgMgr: cfgMgr, handler: handler}
}

// Start begins the polling loop. It blocks until ctx is done.
// Run it in a goroutine: go tailer.Start(ctx).
func (t *Tailer) Start(ctx interface{ Done() <-chan struct{} }) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			t.mu.Lock()
			t.closeFile()
			t.mu.Unlock()
			return
		case <-ticker.C:
			t.tick()
		}
	}
}

// Status returns a snapshot of the tailer's current state.
func (t *Tailer) Status() Status {
	t.mu.Lock()
	defer t.mu.Unlock()

	cfg := t.cfgMgr.Get()
	fp := logFilePath(cfg.EQPath, cfg.Character)
	_, statErr := os.Stat(fp)

	return Status{
		Enabled:    cfg.Preferences.ParseCombatLog,
		FilePath:   fp,
		FileExists: statErr == nil,
		Tailing:    t.file != nil,
		Offset:     t.offset,
	}
}

// tick is the polling body. It is called every pollInterval and does all file I/O.
func (t *Tailer) tick() {
	cfg := t.cfgMgr.Get()
	if !cfg.Preferences.ParseCombatLog || cfg.EQPath == "" || cfg.Character == "" {
		return
	}

	logPath := logFilePath(cfg.EQPath, cfg.Character)

	// Collect events while the mutex is held, then dispatch without it.
	var events []LogEvent

	t.mu.Lock()
	events = t.readEvents(logPath)
	t.mu.Unlock()

	for _, ev := range events {
		t.handler(ev)
	}
}

// readEvents opens/maintains the file handle and returns any new events.
// Must be called with t.mu held.
func (t *Tailer) readEvents(logPath string) []LogEvent {
	// Config changed — close old handle and target the new path.
	if logPath != t.filePath {
		t.closeFile()
		t.filePath = logPath
		t.remainder = nil
	}

	if t.file == nil {
		f, err := os.Open(logPath)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				slog.Warn("logparser: open log file", "path", logPath, "err", err)
			}
			return nil
		}
		// Seek to end so we don't replay historical log lines.
		pos, err := f.Seek(0, io.SeekEnd)
		if err != nil {
			_ = f.Close()
			return nil
		}
		t.file = f
		t.offset = pos
		slog.Info("logparser: tailing log file", "path", logPath, "offset", pos)
		return nil
	}

	// Re-stat to detect file truncation (shouldn't happen with EQ, but be safe).
	info, err := t.file.Stat()
	if err != nil {
		slog.Warn("logparser: stat log file", "err", err)
		t.closeFile()
		return nil
	}
	if info.Size() < t.offset {
		slog.Info("logparser: log file truncated, resetting offset")
		t.offset = 0
		t.remainder = nil
	}
	if info.Size() == t.offset {
		return nil // no new data
	}

	// Clamp read size to maxReadPerTick.
	toRead := info.Size() - t.offset
	if toRead > maxReadPerTick {
		toRead = maxReadPerTick
	}

	buf := make([]byte, toRead)
	n, err := t.file.ReadAt(buf, t.offset)
	if n > 0 {
		t.offset += int64(n)
	}
	if err != nil && !errors.Is(err, io.EOF) {
		slog.Warn("logparser: read log file", "err", err)
		return nil
	}
	if n == 0 {
		return nil
	}

	return t.parseChunk(buf[:n])
}

// parseChunk splits raw bytes into lines and parses each complete line.
// Any trailing incomplete line is saved in t.remainder for the next call.
// Must be called with t.mu held.
func (t *Tailer) parseChunk(data []byte) []LogEvent {
	if len(t.remainder) > 0 {
		data = append(t.remainder, data...)
		t.remainder = nil
	}

	var events []LogEvent

	for len(data) > 0 {
		idx := bytes.IndexByte(data, '\n')
		if idx == -1 {
			t.remainder = make([]byte, len(data))
			copy(t.remainder, data)
			break
		}

		line := strings.TrimRight(string(data[:idx]), "\r")
		data = data[idx+1:]

		if line == "" {
			continue
		}

		ev, ok := ParseLine(line)
		if !ok {
			continue
		}
		events = append(events, ev)
	}

	return events
}

// closeFile closes the current file handle if open.
// Must be called with t.mu held.
func (t *Tailer) closeFile() {
	if t.file != nil {
		_ = t.file.Close()
		t.file = nil
	}
	t.offset = 0
}

// logFilePath returns the full path to the EQ log file for the given character.
// EQ log files live at: <EQ_DIR>/Logs/eqlog_<CharName>_pq.proj.txt
func logFilePath(eqPath, character string) string {
	if eqPath == "" || character == "" {
		return ""
	}
	return filepath.Join(eqPath, "Logs", "eqlog_"+character+"_pq.proj.txt")
}
