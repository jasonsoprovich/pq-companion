package chat

import (
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

// Consumer turns raw log lines into stored chat messages across all tracked
// channels. Zone is tracked from the parsed-event stream (EventZone); chat
// lines are matched off the raw line stream (HandleLine), the same hook the
// trigger engine uses.
type Consumer struct {
	store      *Store
	activeChar func() string

	mu       sync.Mutex
	zone     string
	onInsert func(Message)
}

// NewConsumer constructs a consumer wired to store. activeChar should return
// the current in-game character name (typically tailer.ActiveCharacter).
func NewConsumer(store *Store, activeChar func() string) *Consumer {
	return &Consumer{store: store, activeChar: activeChar}
}

// SetOnInsert registers a callback fired after each newly-stored message, used
// to broadcast a WebSocket event so the Chat History tab updates live.
func (c *Consumer) SetOnInsert(fn func(Message)) {
	c.mu.Lock()
	c.onInsert = fn
	c.mu.Unlock()
}

// HandleEvent tracks zone changes from the parsed-event stream.
func (c *Consumer) HandleEvent(ev logparser.LogEvent) {
	if ev.Type != logparser.EventZone {
		return
	}
	if zd, ok := ev.Data.(logparser.ZoneData); ok {
		c.mu.Lock()
		c.zone = zd.ZoneName
		c.mu.Unlock()
	}
}

// HandleLine matches a raw log line as chat and stores it.
func (c *Consumer) HandleLine(ts time.Time, msg string) {
	p, ok := ParseChat(strings.TrimRight(msg, "\r\n"))
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
		Channel:   p.Channel,
		Direction: p.Direction,
		Peer:      p.Peer,
		Message:   p.Message,
		Zone:      zone,
		TS:        ts,
	})
	if err != nil {
		slog.Warn("chat: insert failed", "channel", p.Channel, "err", err)
		return
	}
	if inserted && onInsert != nil {
		onInsert(Message{
			Character: character,
			Channel:   p.Channel,
			Direction: p.Direction,
			Peer:      p.Peer,
			Message:   p.Message,
			Zone:      zone,
			TS:        ts.Unix(),
		})
	}
}
