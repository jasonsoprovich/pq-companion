package chat

import (
	"strings"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

// BackfillHandler replays a character's log to store past chat across all
// tracked channels. It satisfies backfill.Handler. Zone is tracked from the
// event stream; inserts are dedup-guarded so re-running is idempotent.
type BackfillHandler struct {
	store     *Store
	character string
	zone      string
	inserted  int
}

// NewBackfillHandler returns a handler that attributes stored chat to character.
func NewBackfillHandler(store *Store, character string) *BackfillHandler {
	return &BackfillHandler{store: store, character: character}
}

// HandleEvent tracks zone changes so messages are stamped with the right zone.
func (h *BackfillHandler) HandleEvent(ev logparser.LogEvent) {
	if ev.Type != logparser.EventZone {
		return
	}
	if zd, ok := ev.Data.(logparser.ZoneData); ok {
		h.zone = zd.ZoneName
	}
}

// HandleLine stores any chat line on the line.
func (h *BackfillHandler) HandleLine(ts time.Time, msg string) {
	p, ok := ParseChat(strings.TrimRight(msg, "\r\n"))
	if !ok {
		return
	}
	ins, err := h.store.Insert(Input{
		Character: h.character,
		Channel:   p.Channel,
		Direction: p.Direction,
		Peer:      p.Peer,
		Message:   p.Message,
		Zone:      h.zone,
		TS:        ts,
	})
	if err == nil && ins {
		h.inserted++
	}
}

// Finalize is a no-op; chat is inserted line-by-line with no buffered state.
func (h *BackfillHandler) Finalize() {}

// Inserted reports how many new messages were stored.
func (h *BackfillHandler) Inserted() int { return h.inserted }
