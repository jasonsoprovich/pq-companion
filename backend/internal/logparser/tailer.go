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
	"sync/atomic"
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
	onCharacterChange func(string)            // called when the in-game (auto-detected) character changes; nil = disabled

	mu                sync.Mutex
	file              *os.File
	filePath          string
	offset            int64
	remainder         []byte // incomplete line fragment from the previous read
	remainderStable   bool   // remainder survived one idle poll — safe to flush as a complete line
	detectedCharacter string // last auto-detected character name (empty when manually configured)
	paused            bool   // true while a log replay session owns the dispatch pipeline

	// rawFeed, when set, makes the live feed also surface lines that match no
	// known event pattern (chat, system messages, "X is no longer mezzed", …)
	// so they can be searched there. Opt-in: off by default to keep the feed
	// to recognised combat/spell events. Read on the hot path, so atomic.
	rawFeed atomic.Bool
}

// SetRawFeed toggles whether unrecognised log lines are broadcast to the live
// feed (as log:raw events) in addition to the classified events.
func (t *Tailer) SetRawFeed(enabled bool) { t.rawFeed.Store(enabled) }

// RawFeed reports whether the raw-line passthrough is currently enabled.
func (t *Tailer) RawFeed() bool { return t.rawFeed.Load() }

// NewTailer creates a Tailer. Call Start in a goroutine to begin tailing.
// lineHandler, if non-nil, is called for every line that has a valid EQ
// timestamp — regardless of whether it matches a known event pattern. This
// allows the trigger engine to match arbitrary log text.
// onCharacterChange, if non-nil, is called whenever the in-game active
// character changes — i.e. a different eqlog file becomes most-recently-
// modified, which signals the player camped one character and logged in
// as another. Fires regardless of whether the configured character is
// manually set or auto-detected; the callback is expected to decide
// whether to drop a stale manual override.
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

// ActiveCharacter returns the name of the currently active character: the
// manual override from config when set, otherwise the auto-detected character
// (the most-recently-modified eqlog file). Returns "" if no character can be
// resolved.
func (t *Tailer) ActiveCharacter() string {
	cfg := t.cfgMgr.Get()
	if cfg.Character != "" {
		return cfg.Character
	}
	t.mu.Lock()
	detected := t.detectedCharacter
	t.mu.Unlock()
	if detected != "" {
		return detected
	}
	return ResolveActiveCharacter(cfg.EQPath)
}

// SetPaused suspends (true) or resumes (false) live log tailing. Used by the
// replay pipeline so historical lines don't interleave with live ones. While
// paused the file handle is closed; on resume the tailer reopens it and seeks
// to EOF, so lines appended during the pause are NOT replayed (consistent
// with the first-open behavior).
func (t *Tailer) SetPaused(paused bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.paused == paused {
		return
	}
	t.paused = paused
	if paused {
		t.closeFile()
	}
	slog.Info("logparser: live tailing", "paused", paused)
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
	t.mu.Lock()
	paused := t.paused
	t.mu.Unlock()
	if paused {
		return
	}

	// Always resolve the in-game character from the most-recent eqlog
	// mtime, regardless of manual override. We need this to detect the
	// camp-then-login case: the user manually picked Osui at some point,
	// then in-game camped Osui and logged in as Nariana. The companion
	// should follow the in-game character, not stay pinned to the stale
	// manual selection.
	autoDetectedChar := ResolveActiveCharacter(cfg.EQPath)
	manualChar := cfg.Character
	autoDetected := manualChar == ""

	character := manualChar
	if autoDetected {
		character = autoDetectedChar
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
	prevDetected := t.detectedCharacter
	t.detectedCharacter = autoDetectedChar
	// Fire onCharacterChange when the auto-detected character changes:
	//   - Auto mode: fire on any change including the initial transition
	//     from empty — the UI needs to learn the active character on
	//     startup.
	//   - Manual mode: fire only on a transition between two distinct
	//     non-empty characters, i.e. the user clearly switched characters
	//     in-game during this session. The initial "from empty" detection
	//     is suppressed so opening the app with a stale manual selection
	//     doesn't immediately clobber it just because some other
	//     character's log happens to have the most recent historical
	//     mtime.
	if autoDetectedChar != "" && autoDetectedChar != prevDetected {
		if autoDetected || prevDetected != "" {
			changedCharacter = autoDetectedChar
		}
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
		t.remainderStable = false
	}

	if t.file == nil {
		f, err := openShared(logPath)
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
		// A re-read hazard: the live file is now smaller than where we'd read
		// to. Seek to the new end rather than rewinding to 0 — rewinding would
		// re-dispatch every historical line (mass duplicate trigger fires).
		// Real EQ logs only ever grow, so this path is defensive; logging it at
		// info level flags the rare case in a bug report.
		slog.Info("logparser: log file shrank, seeking to end (not replaying)",
			"prev_offset", t.offset, "new_size", info.Size())
		t.offset = info.Size()
		t.remainder = nil
		t.remainderStable = false
		return nil, nil
	}
	if info.Size() == t.offset {
		// No new data. EQ writes each line terminated by '\n', but the final
		// line before the game goes idle (e.g. the camp countdown right before
		// you log out to character select) can sit unterminated until the next
		// session appends to the file — leaving it parked in t.remainder, so
		// triggers never see it even though it's on disk and visible in browse.
		// Once that remainder has survived a full idle poll it must be a
		// complete line, so flush it through the normal pipeline.
		return t.flushStaleRemainder()
	}

	// Clamp read size to maxReadPerTick.
	toRead := info.Size() - t.offset
	if toRead > maxReadPerTick {
		toRead = maxReadPerTick
	}

	buf := make([]byte, toRead)
	readAt := t.offset
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

	// Verbose diagnostics: the byte range each dispatch came from. If the same
	// line is ever reported as firing twice, two reads covering an overlapping
	// [from,to) range are the signature of a re-read (vs. two distinct physical
	// lines, which always sit at strictly increasing offsets).
	slog.Debug("logparser: read chunk", "from", readAt, "to", t.offset, "bytes", n)

	return t.parseChunk(buf[:n])
}

// flushStaleRemainder emits a parked remainder line once it has survived a full
// idle poll with no new bytes appended — at which point EQ has finished writing
// it and the only thing missing is the trailing newline. The first idle poll
// marks the remainder stable; the next one flushes it. Returns nothing while
// the remainder is empty, still settling, or not yet a complete EQ line.
// Must be called with t.mu held.
func (t *Tailer) flushStaleRemainder() ([]LogEvent, []rawLine) {
	if len(t.remainder) == 0 {
		t.remainderStable = false
		return nil, nil
	}
	if !t.remainderStable {
		t.remainderStable = true // give EQ one more poll to finish the write
		return nil, nil
	}

	line := strings.TrimRight(string(t.remainder), "\r")
	ts, msg, ok := ParseRawLine(line)
	if !ok {
		return nil, nil // not a complete EQ line yet — keep waiting
	}
	t.remainder = nil
	t.remainderStable = false

	// Verbose diagnostics: this idle-only path flushes the final unterminated
	// line of a session. It's the one place a still-being-written line could be
	// emitted early, so surface each flush when debugging the "alert fired on a
	// partial/odd line" reports.
	slog.Debug("logparser: flushed stale remainder (idle)", "offset", t.offset, "line", msg)

	raws := []rawLine{{ts: ts, msg: msg}}
	var events []LogEvent
	if ev, classified := ParseLine(line); classified {
		events = append(events, ev)
	}
	return events, raws
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
	// New bytes arrived: any leftover remainder this produces is fresh, so it
	// must settle for another idle poll before flushStaleRemainder emits it.
	t.remainderStable = false

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
