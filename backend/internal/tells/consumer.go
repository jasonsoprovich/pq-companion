package tells

import (
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

// Consumer turns raw log lines into stored tells. Zone is tracked from the
// parsed-event stream (EventZone) so each tell can be stamped with the zone
// the character was in; the tell lines themselves are matched off the raw
// line stream (HandleLine), the same hook the trigger engine uses.
type Consumer struct {
	store      *Store
	activeChar func() string // current in-game character; "" leaves it blank

	mu       sync.Mutex
	zone     string
	onInsert func(Tell)
}

// NewConsumer constructs a consumer wired to store. activeChar should return
// the currently active character name (typically tailer.ActiveCharacter).
func NewConsumer(store *Store, activeChar func() string) *Consumer {
	return &Consumer{store: store, activeChar: activeChar}
}

// SetOnInsert registers a callback fired after each newly-stored tell. Used by
// the API layer to broadcast a WebSocket event so the Tell Tracker tab updates
// live. Duplicate lines (already stored) do not fire the callback.
func (c *Consumer) SetOnInsert(fn func(Tell)) {
	c.mu.Lock()
	c.onInsert = fn
	c.mu.Unlock()
}

// HandleEvent tracks zone changes from the parsed-event stream. Wire this into
// the tailer's event handler alongside the other consumers.
func (c *Consumer) HandleEvent(ev logparser.LogEvent) {
	if ev.Type != logparser.EventZone {
		return
	}
	zd, ok := ev.Data.(logparser.ZoneData)
	if !ok {
		return
	}
	c.mu.Lock()
	c.zone = zd.ZoneName
	c.mu.Unlock()
}

// HandleLine matches a raw log line as a tell and stores it. Wire this into the
// tailer's line handler alongside the trigger engine / other line consumers.
func (c *Consumer) HandleLine(ts time.Time, msg string) {
	p, ok := ParseTell(strings.TrimRight(msg, "\r\n"))
	if !ok {
		return
	}

	c.mu.Lock()
	zone := c.zone
	onInsert := c.onInsert
	c.mu.Unlock()

	character := ""
	if c.activeChar != nil {
		character = c.activeChar()
	}

	inserted, err := c.store.Insert(Input{
		Character: character,
		Peer:      p.Peer,
		Direction: p.Direction,
		Message:   p.Message,
		Zone:      zone,
		TS:        ts,
	})
	if err != nil {
		slog.Warn("tells: insert failed", "peer", p.Peer, "err", err)
		return
	}
	if inserted && onInsert != nil {
		onInsert(Tell{
			Character: character,
			Peer:      p.Peer,
			Direction: p.Direction,
			Message:   p.Message,
			Zone:      zone,
			TS:        ts.Unix(),
		})
	}
}
