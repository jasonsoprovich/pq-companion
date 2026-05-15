package players

import (
	"log/slog"
	"sync"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

// Consumer turns log events into Sighting upserts. It tracks the active zone
// across EventZone messages so each EventWhoEntry can be tagged with the zone
// the user was in at the time of the line — the logparser itself is stateless
// about zone, so this state lives here.
type Consumer struct {
	store *Store

	mu   sync.RWMutex
	zone string
}

// NewConsumer constructs a consumer wired to the given store.
func NewConsumer(store *Store) *Consumer {
	return &Consumer{store: store}
}

// Handle is the entry point for the shared logparser event stream. It picks
// up EventZone (state update) and EventWhoEntry (upsert) and ignores
// everything else.
func (c *Consumer) Handle(ev logparser.LogEvent) {
	switch ev.Type {
	case logparser.EventZone:
		zd, ok := ev.Data.(logparser.ZoneData)
		if !ok {
			return
		}
		c.mu.Lock()
		c.zone = zd.ZoneName
		c.mu.Unlock()
	case logparser.EventWhoEntry:
		wd, ok := ev.Data.(logparser.WhoEntryData)
		if !ok {
			return
		}
		c.mu.RLock()
		zone := c.zone
		c.mu.RUnlock()
		in := SightingInput{
			Name:       wd.Name,
			Level:      wd.Level,
			Class:      wd.Class,
			Race:       wd.Race,
			Guild:      wd.Guild,
			Anonymous:  wd.Anonymous,
			Zone:       zone,
			ObservedAt: ev.Timestamp,
		}
		if err := c.store.Upsert(in); err != nil {
			slog.Warn("players: upsert failed", "name", wd.Name, "err", err)
		}
	}
}
