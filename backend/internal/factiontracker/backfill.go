package factiontracker

import (
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

// ConsideredMergeFunc reconciles one faction's backfill-recovered /con
// reading into durable storage. Only ever touches last_bucket/
// last_considered_at (and clears last_consider_suspect). Reports whether
// anything changed.
type ConsideredMergeFunc func(factionID int, factionName, bucket string, consideredAt time.Time) (changed bool, err error)

// RangeMergeFunc reconciles one faction's backfill-recovered standing
// estimate (Range*, MaxStep) into durable storage, keeping whichever
// estimate has the newer anchor. Reports whether anything changed.
type RangeMergeFunc func(t Tally) (changed bool, err error)

// BackfillConfig wires a BackfillHandler to the same resolvers the live
// engine uses, so a replayed line resolves exactly as it would have live.
type BackfillConfig struct {
	CharacterID     int
	ResolveKill     NPCFactionResolver
	ResolvePrimary  NPCPrimaryFactionResolver
	ResolveID       FactionIDResolver
	ResolveDialogue QuestDialogueResolver
	ResolveInfo     FactionInfoResolver
	MergeConsider   ConsideredMergeFunc
	MergeRange      RangeMergeFunc
	// OnDone runs after Finalize has merged everything, e.g. to reload the
	// live engine from storage. Optional.
	OnDone func()
}

// BackfillHandler replays a character's log through a scratch Engine (same
// resolvers, nothing persisted or broadcast) and merges two things per
// faction at Finalize:
//   - the most recent /con reading (last_bucket);
//   - the standing estimate the replay ends with: anchored by the last
//     "could not possibly get any…" line or /con, then walked through every
//     faction change after it, exactly as the live tracker does.
//
// The better/worse/estimated-net/unresolved counts are deliberately NOT
// merged — they only build up live, so backfill can't double them.
//
// A replay can't know the player's state at a past moment, so a /con that
// contradicts the estimate is ignored when an override could have produced
// it (indifferent, threatening, amiable, dubious — see Tally.applyCon). One
// that starts an estimate, or comes from an illusion or Alliance, is still
// taken at face value (and last_consider_suspect is never set). The next
// clean live /con corrects it.
//
// Archives overlap the following file by about 30 days, so events are
// replayed oldest-first and any event older than the newest one already
// replayed is dropped — otherwise the overlap's changes would apply twice.
type BackfillHandler struct {
	cfg    BackfillConfig
	engine *Engine

	highWater time.Time
	skipping  bool
	changed   int
}

// NewBackfillHandler returns a handler for cfg.CharacterID.
func NewBackfillHandler(cfg BackfillConfig) *BackfillHandler {
	e := NewEngine(nil, cfg.ResolveKill)
	e.resolvePrimary = cfg.ResolvePrimary
	e.resolveFactionID = cfg.ResolveID
	e.resolveDialogue = cfg.ResolveDialogue
	e.resolveInfo = cfg.ResolveInfo
	e.distrustOverrides = true
	e.SetCharacter(cfg.CharacterID, nil)
	return &BackfillHandler{cfg: cfg, engine: e}
}

// HandleEvent replays the faction-relevant events into the scratch engine.
func (h *BackfillHandler) HandleEvent(ev logparser.LogEvent) {
	switch ev.Type {
	case logparser.EventKill, logparser.EventFactionChanged,
		logparser.EventConsidered, logparser.EventNPCDialogue:
	default:
		return
	}
	// Drop the archive overlap: once time goes backwards, skip until it
	// passes the newest event seen (lines in that exact second included,
	// since they can't be told apart from the duplicates).
	switch {
	case ev.Timestamp.Before(h.highWater):
		h.skipping = true
		return
	case ev.Timestamp.Equal(h.highWater) && h.skipping:
		return
	case ev.Timestamp.After(h.highWater):
		h.highWater = ev.Timestamp
		h.skipping = false
	}
	h.engine.Handle(ev)
}

// HandleLine is unused — everything arrives as parsed events.
func (h *BackfillHandler) HandleLine(time.Time, string) {}

// Finalize merges every faction's latest /con reading and final standing
// estimate into storage, then runs OnDone.
func (h *BackfillHandler) Finalize() {
	for _, t := range h.engine.State().Tallies {
		if t.FactionID == 0 {
			continue
		}
		changed := false
		if h.cfg.MergeConsider != nil && t.LastBucket != "" && t.LastConsideredAt != nil {
			if c, err := h.cfg.MergeConsider(t.FactionID, t.FactionName, t.LastBucket, *t.LastConsideredAt); err == nil && c {
				changed = true
			}
		}
		if h.cfg.MergeRange != nil && t.RangeKnown {
			if c, err := h.cfg.MergeRange(t); err == nil && c {
				changed = true
			}
		}
		if changed {
			h.changed++
		}
	}
	if h.cfg.OnDone != nil {
		h.cfg.OnDone()
	}
}

// Inserted reports how many factions had a reading or estimate created or
// advanced.
func (h *BackfillHandler) Inserted() int { return h.changed }
