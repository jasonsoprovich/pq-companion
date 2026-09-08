package mystats

import (
	"log/slog"
	"strings"
	"sync"
	"time"
)

// maxBlockSpan bounds how far apart the first and last line of one `/mystats`
// block may be. The command prints every line in the same log-second; 5s is
// generous slack for a stuttering client and still short enough that the next
// unrelated line reliably closes the block.
const maxBlockSpan = 5 * time.Second

// maxBufferLines caps a single buffered block so a run of block-shaped lines
// can never grow it without bound. A real block is ~15 lines.
const maxBufferLines = 80

// Update is broadcast after a new snapshot is stored so the Character Info page
// can refresh.
type Update struct {
	Character  string `json:"character"`
	SnapshotID int64  `json:"snapshot_id"`
	CapturedAt int64  `json:"captured_at"`
}

// Consumer watches the raw log line stream for `/mystats` output blocks and
// stores each completed block as a stat snapshot for the active character.
type Consumer struct {
	store      *Store
	activeChar func() string

	mu         sync.Mutex
	buf        []string
	blockStart time.Time
	onUpdate   func(Update)
}

// NewConsumer wires a consumer to store. activeChar returns the current in-game
// character — `/mystats` lines carry no name, so a completed block is
// attributed to whoever's log is being tailed.
func NewConsumer(store *Store, activeChar func() string) *Consumer {
	return &Consumer{store: store, activeChar: activeChar}
}

// SetOnUpdate registers a callback fired after each stored snapshot.
func (c *Consumer) SetOnUpdate(fn func(Update)) {
	c.mu.Lock()
	c.onUpdate = fn
	c.mu.Unlock()
}

// HandleLine feeds one raw log line (message text, no timestamp prefix) plus
// its parsed timestamp.
func (c *Consumer) HandleLine(ts time.Time, msg string) {
	msg = strings.TrimRight(msg, "\r\n")

	c.mu.Lock()
	var fire *Update

	switch {
	case msg == StartMarker:
		// A new block opens — finalize whatever was buffered first.
		if u := c.finalizeLocked(); u != nil {
			fire = u
		}
		c.buf = []string{msg}
		c.blockStart = ts

	case len(c.buf) > 0:
		withinSpan := ts.IsZero() || c.blockStart.IsZero() || ts.Sub(c.blockStart) <= maxBlockSpan
		if IsBlockLine(msg) && withinSpan && len(c.buf) < maxBufferLines {
			c.buf = append(c.buf, msg)
		} else {
			// This line ends the block. It is not part of it and is handled by
			// the other line consumers on its own; we just close ours.
			if u := c.finalizeLocked(); u != nil {
				fire = u
			}
		}
	}
	c.mu.Unlock()

	if fire != nil && c.onUpdate != nil {
		c.onUpdate(*fire)
	}
}

// Flush finalizes any in-progress block. Call it when the log stream goes idle
// (e.g. replay finished) so a trailing block isn't left unsaved.
func (c *Consumer) Flush() {
	c.mu.Lock()
	u := c.finalizeLocked()
	c.mu.Unlock()
	if u != nil && c.onUpdate != nil {
		c.onUpdate(*u)
	}
}

// finalizeLocked parses and stores the buffered block, clearing the buffer.
// Returns a non-nil Update only when a new row was actually inserted. Caller
// holds c.mu.
func (c *Consumer) finalizeLocked() *Update {
	if len(c.buf) == 0 {
		return nil
	}
	lines := c.buf
	start := c.blockStart
	c.buf = nil
	c.blockStart = time.Time{}

	snap, ok := Parse(lines)
	if !ok {
		return nil
	}
	character := ""
	if c.activeChar != nil {
		character = c.activeChar()
	}
	if character == "" {
		slog.Debug("mystats: dropping snapshot, no active character")
		return nil
	}
	capturedAt := start
	if capturedAt.IsZero() {
		capturedAt = time.Now()
	}
	raw := strings.Join(lines, "\n")

	id, inserted, err := c.store.Insert(character, capturedAt.Unix(), raw, snap)
	if err != nil {
		slog.Warn("mystats: store snapshot", "character", character, "err", err)
		return nil
	}
	if !inserted {
		return nil
	}
	slog.Info("mystats: stored stat snapshot", "character", character, "id", id)
	return &Update{Character: character, SnapshotID: id, CapturedAt: capturedAt.Unix()}
}
