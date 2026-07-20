package factiontracker

import (
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

// ConsideredMergeFunc reconciles one faction's backfill-recovered /con
// reading into durable storage. Only ever touches last_bucket/
// last_considered_at (and clears last_consider_suspect — see
// BackfillHandler's doc comment); better/worse/estimated_net/unresolved are
// never touched by backfill at all, they're exclusively the live session
// tracker's concern. Reports whether anything changed.
type ConsideredMergeFunc func(factionID int, factionName, bucket string, consideredAt time.Time) (changed bool, err error)

// BackfillHandler replays a character's log looking only for /con lines
// (EventConsidered) to set an approximate faction-standing baseline — it
// deliberately does NOT attempt to reconstruct the better/worse/estimated-
// net tally from historical kills or quest turn-ins the way the live
// session tracker does; that would require re-deriving fine-grained
// point-in-time state a bare replay can't reliably attribute, for
// relatively little payoff over just knowing "where do I currently stand."
// Resolves each considered NPC to its primary faction and keeps only the
// most recent reading per faction across the whole log.
//
// last_consider_suspect is never set true here: illusion suppression checks
// the *live*, currently-active buff-timer state at the moment a /con line
// is parsed, which a static log replay has no way to reconstruct for a past
// moment (see LIMITATIONS.md §15.4).
type BackfillHandler struct {
	resolvePrimary NPCPrimaryFactionResolver
	merge          ConsideredMergeFunc

	latest  map[int]consideredReading // keyed by faction id
	changed int
}

type consideredReading struct {
	factionName string
	bucket      string
	at          time.Time
}

// NewBackfillHandler returns a handler that resolves considered NPCs via
// resolvePrimary (the same resolver the live tracker uses) and merges the
// latest reading per faction via merge at Finalize.
func NewBackfillHandler(resolvePrimary NPCPrimaryFactionResolver, merge ConsideredMergeFunc) *BackfillHandler {
	return &BackfillHandler{resolvePrimary: resolvePrimary, merge: merge, latest: map[int]consideredReading{}}
}

// HandleEvent buffers the latest /con reading per resolved faction.
func (h *BackfillHandler) HandleEvent(ev logparser.LogEvent) {
	if ev.Type != logparser.EventConsidered || h.resolvePrimary == nil {
		return
	}
	data, ok := ev.Data.(logparser.ConsideredData)
	if !ok || data.Bucket == "" {
		return
	}
	factionID, factionName, ok := h.resolvePrimary(data.TargetName)
	if !ok {
		return
	}
	if cur, exists := h.latest[factionID]; !exists || ev.Timestamp.After(cur.at) {
		h.latest[factionID] = consideredReading{factionName: factionName, bucket: string(data.Bucket), at: ev.Timestamp}
	}
}

// HandleLine is unused — /con readings arrive as parsed events.
func (h *BackfillHandler) HandleLine(time.Time, string) {}

// Finalize merges every faction's latest recovered reading into storage.
func (h *BackfillHandler) Finalize() {
	for factionID, r := range h.latest {
		if changed, err := h.merge(factionID, r.factionName, r.bucket, r.at); err == nil && changed {
			h.changed++
		}
	}
}

// Inserted reports how many faction baselines were created or advanced.
func (h *BackfillHandler) Inserted() int { return h.changed }
