package popflag

import (
	"log/slog"
	"strings"
	"sync"
	"time"
)

// FlushIdle is how long to wait after the last Seer line before committing a
// buffered guided-meditation burst. The Seer prints its lines in one tight
// burst (same second), so 1.5s is ample headroom while staying responsive —
// mirrors keyring.FlushIdle.
const FlushIdle = 1500 * time.Millisecond

// Consumer turns live Seer guided-meditation log lines into a per-character
// snapshot. Every incoming line is tested with MatchSeerLine; matching lines
// are buffered and committed (via Store.ApplySeer) on the first non-matching
// line OR after FlushIdle of no new matches.
//
// This is the live-log counterpart to the paste-in path: both ultimately call
// ParseSeer + ApplySeer, so they share precedence (manual rows are preserved).
type Consumer struct {
	store      *Store
	activeChar func() string
	onSnapshot func(string)

	mu          sync.Mutex
	buffer      []string
	character   string
	lastMatchAt time.Time
	timer       *time.Timer
}

// NewConsumer constructs a consumer wired to store. activeChar should return
// the currently active character name; when it returns empty the burst is
// buffered but the snapshot is suppressed on flush.
func NewConsumer(store *Store, activeChar func() string) *Consumer {
	return &Consumer{store: store, activeChar: activeChar}
}

// SetOnSnapshot registers a callback fired after each successful commit, used
// to broadcast a WebSocket event so open views refresh in place.
func (c *Consumer) SetOnSnapshot(fn func(character string)) {
	c.mu.Lock()
	c.onSnapshot = fn
	c.mu.Unlock()
}

// HandleLine processes one raw log line (same feed the keyring/trigger paths use).
func (c *Consumer) HandleLine(ts time.Time, msg string) {
	if msg == "" {
		return
	}
	if MatchSeerLine(msg) {
		c.appendMatch(msg, ts)
		return
	}
	c.flush()
}

// HandleEvent evaluates a typed live event (kind "kill"/"zone", entity name)
// against the dataset's EventRules and optimistically records matches as
// 'auto'-sourced rows for the active character. Auto never overwrites a manual
// or seer row (enforced by Store.SetAuto), and a snapshot event is broadcast
// only when something actually changed. Independent of the Seer line buffer.
func (c *Consumer) HandleEvent(kind, name string) {
	if c.store == nil {
		return
	}
	ids := MatchEvent(kind, name)
	if len(ids) == 0 {
		return
	}
	character := ""
	if c.activeChar != nil {
		character = c.activeChar()
	}
	if character == "" {
		return
	}
	changed := false
	for _, id := range ids {
		inserted, err := c.store.SetAuto(character, id)
		if err != nil {
			slog.Warn("popflag: auto-detect failed", "flag", id, "err", err)
			continue
		}
		if inserted {
			changed = true
			slog.Info("popflag: auto-detected flag", "character", character, "flag", id, "kind", kind, "match", name)
		}
	}
	if !changed {
		return
	}
	c.mu.Lock()
	cb := c.onSnapshot
	c.mu.Unlock()
	if cb != nil {
		cb(character)
	}
}

func (c *Consumer) appendMatch(line string, ts time.Time) {
	c.mu.Lock()
	if len(c.buffer) == 0 {
		// First match in a new burst — snapshot the active character now so a
		// mid-burst /camp+login can't move the snapshot to the wrong row.
		c.character = ""
		if c.activeChar != nil {
			c.character = c.activeChar()
		}
	}
	c.buffer = append(c.buffer, line)
	c.lastMatchAt = ts
	if c.timer == nil {
		c.timer = time.AfterFunc(FlushIdle, c.flush)
	} else {
		c.timer.Reset(FlushIdle)
	}
	c.mu.Unlock()
}

func (c *Consumer) flush() {
	c.mu.Lock()
	if len(c.buffer) == 0 {
		c.mu.Unlock()
		return
	}
	text := strings.Join(c.buffer, "\n")
	character := c.character
	observedAt := c.lastMatchAt
	c.buffer = nil
	c.character = ""
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
	c.mu.Unlock()

	if character == "" {
		slog.Debug("popflag: skipped Seer snapshot — no active character")
		return
	}
	done, err := c.store.ApplySeer(character, ParseSeer(text), text, observedAt)
	if err != nil {
		slog.Warn("popflag: Seer snapshot failed", "character", character, "err", err)
		return
	}
	slog.Info("popflag: Seer snapshot committed", "character", character, "flags", len(done))
	c.mu.Lock()
	cb := c.onSnapshot
	c.mu.Unlock()
	if cb != nil {
		cb(character)
	}
}

// Shutdown flushes any in-progress burst. Safe to call multiple times.
func (c *Consumer) Shutdown() { c.flush() }
