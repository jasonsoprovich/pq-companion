package loot

import (
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

// Consumer turns raw log lines into stored loot rows. Zone is tracked from the
// parsed-event stream (EventZone); loot lines are matched off the raw line
// stream (HandleLine).
type Consumer struct {
	store      *Store
	activeChar func() string

	mu       sync.Mutex
	zone     string
	onInsert func(Entry)
}

// NewConsumer constructs a consumer wired to store. activeChar returns the
// current in-game character (used to scope rows and attribute self-loot).
func NewConsumer(store *Store, activeChar func() string) *Consumer {
	return &Consumer{store: store, activeChar: activeChar}
}

// SetOnInsert registers a callback fired after each newly-stored loot row, used
// to broadcast a WebSocket event so the Loot Tracker updates live.
func (c *Consumer) SetOnInsert(fn func(Entry)) {
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

// HandleLine matches a raw log line as loot and stores it.
func (c *Consumer) HandleLine(ts time.Time, msg string) {
	p, ok := ParseLoot(strings.TrimRight(msg, "\r\n"))
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
	player := resolvePlayer(p, character)

	in := Input{Character: character, Player: player, Item: p.Item, Zone: zone, TS: ts}
	inserted, err := c.store.Insert(in)
	if err != nil {
		slog.Warn("loot: insert failed", "item", p.Item, "err", err)
		return
	}
	if inserted && onInsert != nil {
		onInsert(Entry{Character: character, Player: player, Item: p.Item, Zone: zone, TS: ts.Unix()})
	}
}

// resolvePlayer fills in the looter for self-loot: the active character, or
// "You" when no character is known.
func resolvePlayer(p Parsed, character string) string {
	if !p.Self {
		return p.Player
	}
	if character != "" {
		return CapitalizeName(character)
	}
	return "You"
}
