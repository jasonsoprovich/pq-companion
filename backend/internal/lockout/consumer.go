package lockout

import (
	"log/slog"
	"sync"
	"time"
)

// FlushIdle is the inactivity window after which a buffered `/sll` burst is
// committed even if no further log lines arrive. `/sll` output drops in one
// tight burst (every line stamped to the same second in observed logs), so
// 1.5s is ample headroom over the natural spacing while still committing
// promptly. Mirrors the keyring tracker's flush model.
const FlushIdle = 1500 * time.Millisecond

// Consumer turns raw log lines into per-character lockout data via two
// independent paths:
//
//  1. Kill notices ("You have incurred a lockout for X that expires in Y.")
//     are single lines, handled immediately by upserting one row — see
//     handleIncurred. This is what keeps the tracker current without the
//     player ever running `/sll`.
//  2. An `/sll` block begins on a section header line
//     ("=== Current Loot Lockouts ==="). While in a block, "== Name: ..." rows
//     are buffered and tagged with the current section; further headers
//     switch the section. The buffer is committed (replacing the character's
//     entire snapshot) on the first line that is neither a header nor a row,
//     OR after FlushIdle of no new matches. `/sll` has no command echo or
//     footer, so this burst-with-idle approach — the same one the keyring
//     tracker uses for /keys — is the reliable way to bound the block. `/sll`
//     is authoritative: its snapshot overwrites any rows path 1 inserted.
type Consumer struct {
	store      *Store
	activeChar func() string // current in-game character; "" suppresses the snapshot
	onSnapshot func(string)  // optional, fired after a successful Snapshot with the character name

	mu          sync.Mutex
	inBlock     bool
	section     Section
	buffer      []Entry
	character   string // active character captured when the block started
	lastMatchAt time.Time
	timer       *time.Timer
}

// NewConsumer constructs a consumer wired to store. activeChar should return
// the currently active character name (typically tailer.ActiveCharacter); when
// it returns empty the burst is still buffered but the snapshot is suppressed
// on flush so we don't commit lockouts under no character.
func NewConsumer(store *Store, activeChar func() string) *Consumer {
	return &Consumer{store: store, activeChar: activeChar}
}

// SetOnSnapshot registers a callback fired after each successful snapshot
// commit. Used by the API layer to broadcast a WebSocket event so an open
// Lockouts page refetches in place. Safe to call before HandleLine starts.
func (c *Consumer) SetOnSnapshot(fn func(character string)) {
	c.mu.Lock()
	c.onSnapshot = fn
	c.mu.Unlock()
}

// HandleLine processes one raw log line. The tailer feeds this for every line
// that successfully parsed a timestamp — the same path the trigger engine and
// keyring tracker use.
func (c *Consumer) HandleLine(ts time.Time, msg string) {
	if msg == "" {
		return
	}
	if name, remaining, ok := ParseIncurred(msg); ok {
		c.handleIncurred(name, remaining, ts)
		return
	}
	if section, ok := IsHeader(msg); ok {
		c.handleHeader(section, ts)
		return
	}
	if row, ok := ParseRow(msg); ok {
		c.handleRow(row, ts)
		return
	}
	// Any other line ends the block. Cheap no-op when nothing is buffered.
	c.flush()
}

func (c *Consumer) handleHeader(section Section, ts time.Time) {
	c.mu.Lock()
	if !c.inBlock {
		// First header of a new `/sll` block — capture the active character
		// now so a mid-burst /camp+login can't move the snapshot to the wrong
		// row.
		c.inBlock = true
		c.character = ""
		if c.activeChar != nil {
			c.character = c.activeChar()
		}
	}
	c.section = section
	c.lastMatchAt = ts
	c.arm()
	c.mu.Unlock()
}

func (c *Consumer) handleRow(row Row, ts time.Time) {
	c.mu.Lock()
	if !c.inBlock {
		// A "== ..." line outside any header block — not part of an /sll
		// snapshot. Ignore rather than buffer under an unknown section.
		c.mu.Unlock()
		return
	}
	e := Entry{Section: c.section, TargetName: row.TargetName}
	if !row.Available {
		e.ExpiresAt = ts.Add(row.Remaining).Unix()
	}
	c.buffer = append(c.buffer, e)
	c.lastMatchAt = ts
	c.arm()
	c.mu.Unlock()
}

// handleIncurred records a single kill-triggered lockout notice, independent
// of any in-progress `/sll` burst. The duplicated log line EQ sometimes emits
// for the same kill is harmless here: both lines carry identical data, so the
// second upsert just rewrites the same row.
//
// The notice fires the instant a raid boss dies — the same moment the combat
// tracker is writing its own end-of-fight rows and loot rolls are starting,
// so user.db write contention is likeliest right here. HandleLine runs
// inline on the tailer's single dispatch goroutine (shared with every other
// log consumer, including the roll tracker), so a blocking SQLite write here
// would stall delivery of every line behind it — including the roll
// announce/result pairs raiders are watching live. The write doesn't touch
// c.mu, so backgrounding it is safe.
func (c *Consumer) handleIncurred(name string, remaining time.Duration, ts time.Time) {
	character := ""
	if c.activeChar != nil {
		character = c.activeChar()
	}
	if character == "" {
		slog.Debug("lockout: skipped incurred notice — no active character", "target", name)
		return
	}
	expiresAt := ts.Add(remaining)
	go func() {
		if err := c.store.UpsertEntry(character, SectionLoot, name, expiresAt, ts); err != nil {
			slog.Warn("lockout: upsert failed", "character", character, "target", name, "err", err)
			return
		}
		slog.Info("lockout: incurred notice recorded", "character", character, "target", name, "expires_at", expiresAt)
		c.mu.Lock()
		cb := c.onSnapshot
		c.mu.Unlock()
		if cb != nil {
			cb(character)
		}
	}()
}

// arm (re)starts the idle flush timer. Caller must hold c.mu.
func (c *Consumer) arm() {
	if c.timer == nil {
		c.timer = time.AfterFunc(FlushIdle, c.flush)
	} else {
		c.timer.Reset(FlushIdle)
	}
}

// flush commits the buffered block as a snapshot and resets state. Called from
// three places — a non-matching log line, the idle AfterFunc, and Shutdown —
// so it locks internally and is safe to call with nothing buffered.
func (c *Consumer) flush() {
	c.mu.Lock()
	if !c.inBlock && len(c.buffer) == 0 {
		c.mu.Unlock()
		return
	}
	rows := c.buffer
	character := c.character
	observedAt := c.lastMatchAt
	c.buffer = nil
	c.character = ""
	c.inBlock = false
	c.section = ""
	if c.timer != nil {
		c.timer.Stop()
		c.timer = nil
	}
	c.mu.Unlock()

	if len(rows) == 0 {
		// Header(s) with no rows — nothing worth committing.
		return
	}
	if character == "" {
		slog.Debug("lockout: skipped snapshot — no active character", "rows", len(rows))
		return
	}
	if err := c.store.Snapshot(character, rows, observedAt); err != nil {
		slog.Warn("lockout: snapshot failed", "character", character, "rows", len(rows), "err", err)
		return
	}
	slog.Info("lockout: snapshot committed", "character", character, "rows", len(rows))
	c.mu.Lock()
	cb := c.onSnapshot
	c.mu.Unlock()
	if cb != nil {
		cb(character)
	}
}

// Shutdown flushes any in-progress block and stops the idle timer. Call before
// the process exits so an /sll that landed seconds before shutdown isn't lost.
// Safe to call multiple times.
func (c *Consumer) Shutdown() {
	c.flush()
}
