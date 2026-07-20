package factiontracker

import (
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

// MergeFunc reconciles one backfill-computed tally into durable storage,
// reporting whether anything actually changed. See
// character.MergeBackfillFactionTally for the idempotency rule this must
// implement — replay-derived counts only ever grow (or hold) a persisted
// row's counters, never regress them.
type MergeFunc func(tally Tally) (changed bool, err error)

// BackfillHandler replays a character's log through a fresh Engine (no
// PersistFunc, no illusion provider) and merges the resulting tallies into
// storage once at Finalize. Satisfies backfill.Handler.
//
// No illusion provider is ever set: illusion suppression works by checking
// the *currently* active buff timers at the moment of a live /con, and a
// static log replay has no reconstructed timer state for past moments — so
// backfilled /con readings can never be flagged suspect, even if the
// character genuinely was illusioned at the time. Documented in
// LIMITATIONS.md.
type BackfillHandler struct {
	engine  *Engine
	merge   MergeFunc
	changed int
}

// NewBackfillHandler returns a handler that replays events through a fresh
// Engine using the given resolvers (same ones the live tracker uses) and
// merges the result via merge at Finalize.
func NewBackfillHandler(
	resolve NPCFactionResolver,
	resolvePrimary NPCPrimaryFactionResolver,
	resolveFactionID FactionIDResolver,
	resolveDialogue QuestDialogueResolver,
	merge MergeFunc,
) *BackfillHandler {
	e := NewEngine(nil, resolve)
	e.SetPrimaryFactionResolver(resolvePrimary)
	e.SetFactionIDResolver(resolveFactionID)
	e.SetQuestDialogueResolver(resolveDialogue)
	e.SetCharacter(0, nil)
	return &BackfillHandler{engine: e, merge: merge}
}

// HandleEvent feeds the parsed event through the replay engine.
func (h *BackfillHandler) HandleEvent(ev logparser.LogEvent) { h.engine.Handle(ev) }

// HandleLine is unused — every faction-relevant signal arrives as a parsed event.
func (h *BackfillHandler) HandleLine(time.Time, string) {}

// Finalize merges every faction the replay touched into storage.
func (h *BackfillHandler) Finalize() {
	for _, t := range h.engine.State().Tallies {
		if changed, err := h.merge(t); err == nil && changed {
			h.changed++
		}
	}
}

// Inserted reports how many faction rows were created or changed.
func (h *BackfillHandler) Inserted() int { return h.changed }
