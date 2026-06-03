package tells

import (
	"strings"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

// BackfillHandler replays a character's log to store past tells. It satisfies
// backfill.Handler. Zone is tracked from the parsed-event stream so each tell
// gets the same zone stamp it would have had live; inserts are dedup-guarded
// by the store so re-running a backfill is idempotent.
type BackfillHandler struct {
	store     *Store
	character string
	zone      string
	inserted  int
}

// NewBackfillHandler returns a handler that attributes stored tells to
// character.
func NewBackfillHandler(store *Store, character string) *BackfillHandler {
	return &BackfillHandler{store: store, character: character}
}

// HandleEvent tracks zone changes so tells are stamped with the zone in effect.
func (h *BackfillHandler) HandleEvent(ev logparser.LogEvent) {
	if ev.Type != logparser.EventZone {
		return
	}
	if zd, ok := ev.Data.(logparser.ZoneData); ok {
		h.zone = zd.ZoneName
	}
}

// HandleLine stores any direct tell on the line.
func (h *BackfillHandler) HandleLine(ts time.Time, msg string) {
	p, ok := ParseTell(strings.TrimRight(msg, "\r\n"))
	if !ok {
		return
	}
	ins, err := h.store.Insert(Input{
		Character: h.character,
		Peer:      p.Peer,
		Direction: p.Direction,
		Message:   p.Message,
		Zone:      h.zone,
		TS:        ts,
	})
	if err == nil && ins {
		h.inserted++
	}
}

// Finalize is a no-op; tells are inserted line-by-line with no buffered state.
func (h *BackfillHandler) Finalize() {}

// Inserted reports how many new tells were stored.
func (h *BackfillHandler) Inserted() int { return h.inserted }
