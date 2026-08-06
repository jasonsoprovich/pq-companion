package progress

import (
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

// BackfillHandler replays a character's log to populate past level/AA/spell/
// skill milestones. It satisfies backfill.Handler. Uses the same toEvent
// mapping as the live Consumer, and the store's UNIQUE constraint makes
// re-running idempotent.
type BackfillHandler struct {
	store     *Store
	character string
	inserted  int
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

// HandleLine is a no-op; progression milestones arrive as parsed events.
func (h *BackfillHandler) HandleLine(time.Time, string) {}

// Finalize is a no-op; events are inserted as they arrive.
func (h *BackfillHandler) Finalize() {}

// Inserted reports how many event rows were newly created.
func (h *BackfillHandler) Inserted() int { return h.inserted }
