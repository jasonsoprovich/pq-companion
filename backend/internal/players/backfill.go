package players

import (
	"log/slog"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

// BackfillConsumer replays a character's log to populate the (global) player
// sightings table from past /who output. It satisfies backfill.Handler and
// mirrors the live Consumer's /who buffering, but writes through the
// timestamp-aware, idempotent BackfillUpsert and broadcasts nothing.
//
// The character argument is ignored: player sightings are global (who you've
// encountered), not owned by the logging character.
type BackfillConsumer struct {
	store   *Store
	zone    string
	pending []SightingInput
	changed int
}

// NewBackfillConsumer returns a backfill handler for the player tracker.
func NewBackfillConsumer(store *Store) *BackfillConsumer {
	return &BackfillConsumer{store: store}
}

// HandleEvent buffers /who rows and flushes them with the reported zone.
func (c *BackfillConsumer) HandleEvent(ev logparser.LogEvent) {
	switch ev.Type {
	case logparser.EventZone:
		if zd, ok := ev.Data.(logparser.ZoneData); ok {
			c.flush(c.zone)
			c.zone = zd.ZoneName
		}
	case logparser.EventWhoEntry:
		if wd, ok := ev.Data.(logparser.WhoEntryData); ok {
			c.pending = append(c.pending, SightingInput{
				Name:       wd.Name,
				Level:      wd.Level,
				Class:      wd.Class,
				Race:       wd.Race,
				Guild:      wd.Guild,
				Anonymous:  wd.Anonymous,
				Zone:       c.zone,
				ObservedAt: ev.Timestamp,
			})
		}
	case logparser.EventWhoSummary:
		if ws, ok := ev.Data.(logparser.WhoSummaryData); ok {
			c.zone = ws.Zone
			c.flush(ws.Zone)
		}
	}
}

// HandleLine is unused — player data comes entirely from parsed /who events.
func (c *BackfillConsumer) HandleLine(_ time.Time, _ string) {}

func (c *BackfillConsumer) flush(zone string) {
	for _, in := range c.pending {
		in.Zone = zone
		if ok, err := c.store.BackfillUpsert(in); err != nil {
			slog.Warn("players: backfill upsert failed", "name", in.Name, "err", err)
		} else if ok {
			c.changed++
		}
	}
	c.pending = c.pending[:0]
}

// Finalize flushes any /who rows left buffered at end-of-log.
func (c *BackfillConsumer) Finalize() { c.flush(c.zone) }

// Inserted reports how many sightings were created or updated.
func (c *BackfillConsumer) Inserted() int { return c.changed }
