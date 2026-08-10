package progress

import (
	"log/slog"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

// BackfillHandler replays a character's log to populate past level/AA/spell/
// skill milestones. It satisfies backfill.Handler. Uses the same toEvent
// mapping as the live Consumer, and the store's UNIQUE constraint makes
// re-running idempotent.
type BackfillHandler struct {
	store         *Store
	character     string
	inserted      int
	lastActiveDay string // last date (YYYY-MM-DD) already marked, to skip redundant writes
}

// NewBackfillHandler returns a handler that attributes events to character.
func NewBackfillHandler(store *Store, character string) *BackfillHandler {
	return &BackfillHandler{store: store, character: character}
}

// HandleEvent records a progression milestone from the parsed-event stream.
func (h *BackfillHandler) HandleEvent(ev logparser.LogEvent) {
	if h.character == "" {
		return
	}
	out, ok := toEvent(h.character, ev)
	if !ok {
		return
	}
	if inserted, err := h.store.AppendEvent(out); err == nil && inserted {
		h.inserted++
	}
}

// HandleLine marks the character's log as active on ts's calendar day —
// same signal as the live Consumer's HandleLine, replayed for historical
// log content so Active Days is backfillable too.
func (h *BackfillHandler) HandleLine(ts time.Time, _ string) {
	if h.character == "" {
		return
	}
	if ts.IsZero() {
		ts = time.Now()
	}
	date := ts.Local().Format("2006-01-02")
	if h.lastActiveDay == date {
		return
	}
	h.lastActiveDay = date
	if err := h.store.MarkActiveDay(h.character, ts); err != nil {
		slog.Warn("progress: backfill mark active day", "character", h.character, "err", err)
	}
}

// Finalize is a no-op; events are inserted as they arrive.
func (h *BackfillHandler) Finalize() {}

// Inserted reports how many event rows were newly created.
func (h *BackfillHandler) Inserted() int { return h.inserted }
