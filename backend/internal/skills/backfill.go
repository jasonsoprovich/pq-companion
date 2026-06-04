package skills

import (
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

// BackfillHandler replays a character's log to populate past skill values. It
// satisfies backfill.Handler. EventSkillUp lines are attributed to the
// backfilled character; the store's increase-only upsert keeps the highest
// observed value, so re-running is idempotent.
type BackfillHandler struct {
	store     *Store
	character string
	inserted  int
}

// NewBackfillHandler returns a handler that attributes skills to character.
func NewBackfillHandler(store *Store, character string) *BackfillHandler {
	return &BackfillHandler{store: store, character: character}
}

// HandleEvent records skill gains from the parsed-event stream.
func (h *BackfillHandler) HandleEvent(ev logparser.LogEvent) {
	if ev.Type != logparser.EventSkillUp {
		return
	}
	d, ok := ev.Data.(logparser.SkillUpData)
	if !ok || h.character == "" {
		return
	}
	ts := ev.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}
	skillID, _ := SkillID(d.SkillName)
	if changed, err := h.store.Upsert(h.character, d.SkillName, skillID, d.Rank, ts); err == nil && changed {
		h.inserted++
	}
}

// HandleLine is a no-op; skill gains arrive as parsed events.
func (h *BackfillHandler) HandleLine(time.Time, string) {}

// Finalize is a no-op; skills are upserted as events arrive.
func (h *BackfillHandler) Finalize() {}

// Inserted reports how many skill rows were created or updated.
func (h *BackfillHandler) Inserted() int { return h.inserted }
