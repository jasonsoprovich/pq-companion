package lockout

import (
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

// BackfillHandler replays a character's log to populate lockout data seen
// before the app was tracking it live. It satisfies backfill.Handler and
// mirrors Consumer's two live sources — kill notices and `/sll` blocks —
// minus the goroutine/timer bookkeeping the live path needs, since the
// engine already replays lines strictly in file order.
//
// The two sources commit differently, matching how Consumer treats them
// live: kill notices upsert a single row (Store.UpsertEntryIfNewer); a
// completed `/sll` block replaces the character's whole snapshot
// (Store.Snapshot), which is also what preserves duplicate-named targets
// (distinct raid instances) via position — a per-row upsert keyed by name
// would incorrectly collapse them. Both paths are freshness-gated so
// backfilling an old or rotated log can never regress lockout data a live
// session already recorded more recently.
type BackfillHandler struct {
	store     *Store
	character string
	inserted  int

	inBlock    bool
	section    Section
	buffer     []Entry
	observedAt time.Time

	// inTimeBlock tracks a '#timelockout' block separately from the /sll
	// inBlock/buffer/section state above — Time rows commit one at a time via
	// UpsertEntryIfNewer (see HandleLine), not as a buffered block, since a
	// block-level staleness check (MaxObservedAt) would compare a Time sync
	// against the character's LOOT lockout freshness, an unrelated section.
	inTimeBlock bool
}

// NewBackfillHandler returns a handler that attributes lockouts to character.
func NewBackfillHandler(store *Store, character string) *BackfillHandler {
	return &BackfillHandler{store: store, character: character}
}

// HandleEvent is a no-op; lockouts are recognised directly from raw lines.
func (h *BackfillHandler) HandleEvent(logparser.LogEvent) {}

// HandleLine mirrors Consumer.HandleLine's line classification order:
// kill notice, then `/sll` header, then `/sll` row (buffered only while a
// block is open), then anything else ends the open block.
func (h *BackfillHandler) HandleLine(ts time.Time, msg string) {
	if h.character == "" || msg == "" {
		return
	}
	if name, remaining, ok := ParseIncurred(msg); ok {
		wrote, err := h.store.UpsertEntryIfNewer(h.character, SectionLoot, name, ts.Add(remaining), ts)
		if err == nil && wrote {
			h.inserted++
		}
		return
	}
	if section, ok := IsHeader(msg); ok {
		// A second header (Loot → Legacy) within the same block continues the
		// same buffer under the new section — it does not commit what's been
		// buffered so far. See Consumer.handleHeader.
		h.inTimeBlock = false
		h.inBlock = true
		h.section = section
		h.observedAt = ts
		return
	}
	if row, ok := ParseRow(msg); ok {
		if h.inBlock {
			e := Entry{Section: h.section, TargetName: row.TargetName}
			if !row.Available {
				e.ExpiresAt = ts.Add(row.Remaining).Unix()
			}
			h.buffer = append(h.buffer, e)
			h.observedAt = ts
		}
		return
	}
	if IsTimeHeader(msg) {
		// End any open /sll block first, then switch to per-row Time upserts.
		h.commitBlock()
		h.inTimeBlock = true
		return
	}
	if row, ok := ParseTimeRow(msg); ok {
		if h.inTimeBlock {
			expiresAt := time.Unix(0, 0)
			if !row.Available {
				expiresAt = ts.Add(row.Remaining)
			}
			wrote, err := h.store.UpsertEntryIfNewer(h.character, SectionTime, row.TargetName, expiresAt, ts)
			if err == nil && wrote {
				h.inserted++
			}
		}
		return
	}
	if IsTimeNoiseLine(msg) && h.inTimeBlock {
		return
	}
	h.inTimeBlock = false
	h.commitBlock()
}

// commitBlock replaces the character's snapshot with the buffered `/sll`
// rows, unless the block is older than the freshest lockout data already on
// file for this character — which would mean a live session (or a later
// point in this same backfill) has already moved past it.
func (h *BackfillHandler) commitBlock() {
	rows := h.buffer
	observedAt := h.observedAt
	h.inBlock = false
	h.buffer = nil
	h.section = ""
	if len(rows) == 0 {
		return
	}

	if maxObs, ok, err := h.store.MaxObservedAt(h.character); err == nil && ok && observedAt.Unix() < maxObs {
		return
	}
	if err := h.store.Snapshot(h.character, rows, observedAt); err == nil {
		h.inserted += len(rows)
	}
}

// Finalize commits any `/sll` block still open when the log ends — the
// fixture (and real `/sll` output) has no trailing sentinel line to end it.
func (h *BackfillHandler) Finalize() {
	h.commitBlock()
}

// Inserted reports how many lockout rows were created or updated.
func (h *BackfillHandler) Inserted() int { return h.inserted }
