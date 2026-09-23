package popflag

import (
	"log/slog"
	"strings"
	"sync"
	"time"
)

// FlushIdle is how long to wait after the last matching line before
// committing a buffered burst (Seer or '#popflags'). Both print their lines
// in one tight burst (same second), so 1.5s is ample headroom while staying
// responsive — mirrors keyring.FlushIdle.
const FlushIdle = 1500 * time.Millisecond

// Consumer turns live log lines into per-character progression snapshots. It
// runs two independent buffers off the same line feed — one for Seer Mal
// Nae`Shi "guided meditation" text, one for '#popflags' report blocks — since
// the two are structurally disjoint (narrative sentences vs. "Label: Value"
// report lines) and can never both match the same line. Each buffer commits
// on the first line that doesn't match ITS OWN matcher, or after FlushIdle of
// no new matches for that buffer; matching a burst never flushes the other.
//
// This is the live-log counterpart to the paste-in / scan-log paths: all
// three ultimately call ParseSeer+ApplySeer or ParsePopFlagsReport+
// ApplyPopFlagsReport, so they share precedence (manual rows are preserved).
type Consumer struct {
	store      *Store
	activeChar func() string
	onSnapshot func(string)

	mu sync.Mutex

	seerBuffer      []string
	seerCharacter   string
	seerLastMatchAt time.Time
	seerTimer       *time.Timer

	popflagsBuffer      []string
	popflagsCharacter   string
	popflagsLastMatchAt time.Time
	popflagsTimer       *time.Timer
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
		c.appendSeerMatch(msg, ts)
		return
	}
	if MatchPopFlagsLine(msg) {
		c.appendPopFlagsMatch(msg, ts)
		return
	}
	// A line matching neither buffer's format ends whichever burst(s) are in
	// progress — matching one buffer never flushes the other, since an
	// unrelated intervening line (e.g. a stray Seer hail during a #popflags
	// paste) shouldn't be possible in practice, but if it happens each
	// buffer's own idle timer is still a safety net.
	c.flushSeer()
	c.flushPopFlags()
}

// HandleEvent evaluates a typed live event (kind "kill"/"zone", entity name)
// against the dataset's EventRules and optimistically records matches as
// 'auto'-sourced rows for the active character. Auto never overwrites a manual
// or seer/popflags row (enforced by Store.SetAuto), and a snapshot event is
// broadcast only when something actually changed. Independent of both buffers.
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

// ── Seer guided-meditation buffer ───────────────────────────────────────────

func (c *Consumer) appendSeerMatch(line string, ts time.Time) {
	c.mu.Lock()
	if len(c.seerBuffer) == 0 {
		// First match in a new burst — snapshot the active character now so a
		// mid-burst /camp+login can't move the snapshot to the wrong row.
		c.seerCharacter = ""
		if c.activeChar != nil {
			c.seerCharacter = c.activeChar()
		}
	}
	c.seerBuffer = append(c.seerBuffer, line)
	c.seerLastMatchAt = ts
	if c.seerTimer == nil {
		c.seerTimer = time.AfterFunc(FlushIdle, c.flushSeer)
	} else {
		c.seerTimer.Reset(FlushIdle)
	}
	c.mu.Unlock()
}

func (c *Consumer) flushSeer() {
	c.mu.Lock()
	if len(c.seerBuffer) == 0 {
		c.mu.Unlock()
		return
	}
	text := strings.Join(c.seerBuffer, "\n")
	character := c.seerCharacter
	observedAt := c.seerLastMatchAt
	c.seerBuffer = nil
	c.seerCharacter = ""
	if c.seerTimer != nil {
		c.seerTimer.Stop()
		c.seerTimer = nil
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
	c.notify(character)
}

// ── '#popflags' report buffer ───────────────────────────────────────────────

func (c *Consumer) appendPopFlagsMatch(line string, ts time.Time) {
	c.mu.Lock()
	if len(c.popflagsBuffer) == 0 {
		c.popflagsCharacter = ""
		if c.activeChar != nil {
			c.popflagsCharacter = c.activeChar()
		}
	}
	c.popflagsBuffer = append(c.popflagsBuffer, line)
	c.popflagsLastMatchAt = ts
	if c.popflagsTimer == nil {
		c.popflagsTimer = time.AfterFunc(FlushIdle, c.flushPopFlags)
	} else {
		c.popflagsTimer.Reset(FlushIdle)
	}
	c.mu.Unlock()
}

func (c *Consumer) flushPopFlags() {
	c.mu.Lock()
	if len(c.popflagsBuffer) == 0 {
		c.mu.Unlock()
		return
	}
	lines := c.popflagsBuffer
	character := c.popflagsCharacter
	observedAt := c.popflagsLastMatchAt
	c.popflagsBuffer = nil
	c.popflagsCharacter = ""
	if c.popflagsTimer != nil {
		c.popflagsTimer.Stop()
		c.popflagsTimer = nil
	}
	c.mu.Unlock()

	if character == "" {
		slog.Debug("popflag: skipped #popflags snapshot — no active character")
		return
	}
	report := ParsePopFlagsReport(lines)
	done, err := c.store.ApplyPopFlagsReport(character, report, strings.Join(lines, "\n"), observedAt)
	if err != nil {
		slog.Warn("popflag: #popflags snapshot failed", "character", character, "err", err)
		return
	}
	slog.Info("popflag: #popflags snapshot committed", "character", character, "section", report.Section, "flags", len(done))
	c.notify(character)
}

func (c *Consumer) notify(character string) {
	c.mu.Lock()
	cb := c.onSnapshot
	c.mu.Unlock()
	if cb != nil {
		cb(character)
	}
}

// Shutdown flushes any in-progress bursts. Safe to call multiple times.
func (c *Consumer) Shutdown() {
	c.flushSeer()
	c.flushPopFlags()
}
