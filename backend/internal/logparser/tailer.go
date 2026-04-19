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
	cfgMgr            *config.Manager
	handler           func(LogEvent)
	lineHandler       func(time.Time, string) // called for every valid EQ log line (parsed or not)
	onCharacterChange func(string)            // called when auto-detected character changes; nil = disabled

	mu                sync.Mutex
	file              *os.File
	filePath          string
	offset            int64
	remainder         []byte // incomplete line fragment from the previous read
	detectedCharacter string // last auto-detected character name (empty when manually configured)
}

// NewTailer creates a Tailer. Call Start in a goroutine to begin tailing.
// lineHandler, if non-nil, is called for every line that has a valid EQ
// timestamp — regardless of whether it matches a known event pattern. This
// allows the trigger engine to match arbitrary log text.
// onCharacterChange, if non-nil, is called whenever the auto-detected active
// character changes (i.e. when config.Character is blank and a different log
// file becomes most-recently-modified). It is not called when the character
// is set manually in the config.
func NewTailer(cfgMgr *config.Manager, handler func(LogEvent), lineHandler func(time.Time, string), onCharacterChange func(string)) *Tailer {
	return &Tailer{
		cfgMgr:            cfgMgr,
		handler:           handler,
		lineHandler:       lineHandler,
		onCharacterChange: onCharacterChange,
	}
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
	character := cfg.Character
	if character == "" {
		character = ResolveActiveCharacter(cfg.EQPath)
	}
	fp := logFilePath(cfg.EQPath, character)
	_, statErr := os.Stat(fp)

	return Status{
		Enabled:    cfg.Preferences.ParseCombatLog,
		FilePath:   fp,
		FileExists: statErr == nil,
		Tailing:    t.file != nil,
		Offset:     t.offset,
	}
}

// rawLine holds the timestamp and message extracted from a raw EQ log line.
type rawLine struct {
	ts  time.Time
	msg string
}

// tick is the polling body. It is called every pollInterval and does all file I/O.
func (t *Tailer) tick() {
	cfg := t.cfgMgr.Get()
	if !cfg.Preferences.ParseCombatLog || cfg.EQPath == "" {
		return
	}

	character := cfg.Character
	autoDetected := character == ""
	if autoDetected {
		character = ResolveActiveCharacter(cfg.EQPath)
		if character == "" {
			return
		}
	}

	logPath := logFilePath(cfg.EQPath, character)

	// Collect events and raw lines while the mutex is held, then dispatch without it.
	var events []LogEvent
	var rawLines []rawLine
	var changedCharacter string

	t.mu.Lock()
	if autoDetected && character != t.detectedCharacter {
		t.detectedCharacter = character
		changedCharacter = character
	} else if !autoDetected && t.detectedCharacter != "" {
		// Manual character set — clear the auto-detected state.
		t.detectedCharacter = ""
	}
	events, rawLines = t.readLines(logPath)
	t.mu.Unlock()

	// Notify listener of character change outside the mutex.
	if changedCharacter != "" && t.onCharacterChange != nil {
		t.onCharacterChange(changedCharacter)
	}

	// Deliver raw lines first so trigger handlers see every line.
	if t.lineHandler != nil {
		for _, rl := range rawLines {
			t.lineHandler(rl.ts, rl.msg)
		}
	}
	for _, ev := range events {
		t.handler(ev)
	}
}

// readLines opens/maintains the file handle and returns any new events and
// raw lines parsed from newly-appended content.
// Must be called with t.mu held.
func (t *Tailer) readLines(logPath string) ([]LogEvent, []rawLine) {
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
			return nil, nil
		}
		// Seek to end so we don't replay historical log lines.
		pos, err := f.Seek(0, io.SeekEnd)
		if err != nil {
			_ = f.Close()
			return nil, nil
		}
		t.file = f
		t.offset = pos
		slog.Info("logparser: tailing log file", "path", logPath, "offset", pos)
		return nil, nil
	}

	// Re-stat to detect file truncation (shouldn't happen with EQ, but be safe).
	info, err := t.file.Stat()
	if err != nil {
		slog.Warn("logparser: stat log file", "err", err)
		t.closeFile()
		return nil, nil
	}
	if info.Size() < t.offset {
		slog.Info("logparser: log file truncated, resetting offset")
		t.offset = 0
		t.remainder = nil
	}
	if info.Size() == t.offset {
		return nil, nil // no new data
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
		return nil, nil
	}
	if n == 0 {
		return nil, nil
	}

	return t.parseChunk(buf[:n])
}

// parseChunk splits raw bytes into lines and parses each complete line.
// Any trailing incomplete line is saved in t.remainder for the next call.
// Returns the set of recognised LogEvents and every raw line that had a valid
// EQ timestamp (for trigger matching).
// Must be called with t.mu held.
func (t *Tailer) parseChunk(data []byte) ([]LogEvent, []rawLine) {
	if len(t.remainder) > 0 {
		data = append(t.remainder, data...)
		t.remainder = nil
	}

	var events []LogEvent
	var raws []rawLine

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

		// Feed every line with a valid EQ timestamp to the raw handler (triggers).
		if ts, msg, ok := ParseRawLine(line); ok {
			raws = append(raws, rawLine{ts: ts, msg: msg})
		}

		ev, ok := ParseLine(line)
		if !ok {
			continue
		}
		events = append(events, ev)
	}

	return events, raws
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
// EQ log files live at: <EQ_DIR>/eqlog_<CharName>_pq.proj.txt
func logFilePath(eqPath, character string) string {
	if eqPath == "" || character == "" {
		return ""
	}
	return filepath.Join(eqPath, "eqlog_"+character+"_pq.proj.txt")
}

