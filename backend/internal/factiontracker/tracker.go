package factiontracker

import (
	"strings"
	"sync"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// NPCFactionHit is one faction's point delta for an NPC's npc_faction_id, as
// looked up in quarm.db (npc_faction_entries joined to faction_list).
type NPCFactionHit struct {
	FactionID   int
	FactionName string
	Value       int
}

// NPCFactionResolver resolves a killed mob's display name (as it appears in
// the kill log line) to its quarm.db faction hits, best-effort. Returns
// ok=false when the name can't be resolved to an NPC or the NPC carries no
// faction hits. Injected so the tracker stays decoupled from the game DB.
type NPCFactionResolver func(mobName string) (hits []NPCFactionHit, ok bool)

// pendingKill is a resolved kill's expected faction deltas, waiting to be
// matched against the EventFactionChanged lines that should follow within
// correlationWindow. hits is keyed by lowercased faction name; a matched
// entry is deleted so a multi-faction kill's lines each consume their own
// faction once.
type pendingKill struct {
	at   time.Time
	hits map[string]int
}

// Engine tracks session faction-standing changes for wishlisted factions,
// inferred from the EQ log feed. Safe for concurrent use.
type Engine struct {
	hub     *ws.Hub
	resolve NPCFactionResolver

	mu      sync.Mutex
	order   []string          // tracked faction keys (lowercased name), wishlist order
	tallies map[string]*Tally // key: strings.ToLower(FactionName)
	pending []pendingKill
}

// NewEngine returns an initialized Engine with no tracked factions. Call
// SetTracked once the active character's faction wishlist is known.
func NewEngine(hub *ws.Hub, resolve NPCFactionResolver) *Engine {
	return &Engine{
		hub:     hub,
		resolve: resolve,
		tallies: make(map[string]*Tally),
	}
}

// SetTracked replaces the set of factions being tracked, in display order.
// Existing tallies are preserved for factions that remain tracked; factions
// no longer present are dropped. Called on startup and whenever the active
// character or its faction wishlist changes.
func (e *Engine) SetTracked(factions []TrackedFaction) {
	e.mu.Lock()
	newTallies := make(map[string]*Tally, len(factions))
	newOrder := make([]string, 0, len(factions))
	for _, f := range factions {
		key := strings.ToLower(f.FactionName)
		if existing, ok := e.tallies[key]; ok {
			existing.FactionID = f.FactionID
			existing.FactionName = f.FactionName
			newTallies[key] = existing
		} else {
			newTallies[key] = &Tally{FactionID: f.FactionID, FactionName: f.FactionName}
		}
		newOrder = append(newOrder, key)
	}
	e.tallies = newTallies
	e.order = newOrder
	state := e.stateLocked()
	e.mu.Unlock()
	e.broadcast(state)
}

// Reset zeroes every tracked faction's tally and drops pending kill
// correlations, without changing which factions are tracked — the
// equivalent of starting a new tracking session at the current camp.
func (e *Engine) Reset() {
	e.mu.Lock()
	for _, t := range e.tallies {
		t.Better, t.Worse, t.EstimatedNet, t.Unresolved = 0, 0, 0, 0
	}
	e.pending = nil
	state := e.stateLocked()
	e.mu.Unlock()
	e.broadcast(state)
}

// State returns a snapshot of the current session tallies.
func (e *Engine) State() State {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.stateLocked()
}

// Handle processes a parsed log event, updating tallies and pending-kill
// correlations as needed.
func (e *Engine) Handle(ev logparser.LogEvent) {
	switch ev.Type {
	case logparser.EventKill:
		data, ok := ev.Data.(logparser.KillData)
		if !ok || data.Target == "" {
			return
		}
		e.handleKill(data.Target, ev.Timestamp)
	case logparser.EventFactionChanged:
		data, ok := ev.Data.(logparser.FactionChangedData)
		if !ok {
			return
		}
		e.handleFactionChanged(data.Faction, data.Direction, ev.Timestamp)
	}
}

// handleKill resolves the slain mob to its DB faction hits and stashes them
// as a pending correlation. No-op (and no lock taken) if nothing tracked
// could possibly match, since the resolver call is the expensive part.
func (e *Engine) handleKill(target string, ts time.Time) {
	if e.resolve == nil {
		return
	}
	hits, ok := e.resolve(target)
	if !ok || len(hits) == 0 {
		return
	}
	m := make(map[string]int, len(hits))
	for _, h := range hits {
		// A zero-value entry never produces a "got better/worse" line (the
		// server doesn't log a no-op change), so it can never be matched —
		// skip it rather than let it dilute the pending map.
		if h.Value == 0 {
			continue
		}
		m[strings.ToLower(h.FactionName)] = h.Value
	}
	if len(m) == 0 {
		return
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	e.pending = append(e.pending, pendingKill{at: ts, hits: m})
	e.gcPendingLocked(ts)
}

// handleFactionChanged tallies a "got better/worse" line for a tracked
// faction and, if a pending kill within correlationWindow has a matching
// signed hit for this faction, attributes its point value as an estimate.
// Non-tracked factions are dropped immediately without touching the pending
// backlog or acquiring more than the map-lookup lock.
func (e *Engine) handleFactionChanged(factionName, direction string, ts time.Time) {
	key := strings.ToLower(factionName)

	e.mu.Lock()
	tally, tracked := e.tallies[key]
	if !tracked {
		e.mu.Unlock()
		return
	}
	if direction == "better" {
		tally.Better++
	} else {
		tally.Worse++
	}

	e.gcPendingLocked(ts)
	matched := false
	// Newest-first so a burst of identical rapid kills (e.g. a fast-respawning
	// script encounter) consumes its own most recent hit rather than an older
	// one that a later line might still need.
	for i := len(e.pending) - 1; i >= 0; i-- {
		val, ok := e.pending[i].hits[key]
		if !ok {
			continue
		}
		if (direction == "better" && val > 0) || (direction == "worse" && val < 0) {
			tally.EstimatedNet += val
			delete(e.pending[i].hits, key)
			matched = true
			break
		}
	}
	if !matched {
		tally.Unresolved++
	}
	state := e.stateLocked()
	e.mu.Unlock()
	e.broadcast(state)
}

// gcPendingLocked drops pending kills older than correlationWindow relative
// to now, and caps the backlog at maxPendingKills. Must be called with mu
// held.
func (e *Engine) gcPendingLocked(now time.Time) {
	cutoff := now.Add(-correlationWindow)
	kept := e.pending[:0]
	for _, p := range e.pending {
		if p.at.After(cutoff) {
			kept = append(kept, p)
		}
	}
	e.pending = kept
	if len(e.pending) > maxPendingKills {
		e.pending = e.pending[len(e.pending)-maxPendingKills:]
	}
}

func (e *Engine) stateLocked() State {
	out := make([]Tally, 0, len(e.order))
	for _, key := range e.order {
		if t, ok := e.tallies[key]; ok {
			out = append(out, *t)
		}
	}
	return State{Tallies: out}
}

func (e *Engine) broadcast(state State) {
	if e.hub == nil {
		return
	}
	e.hub.Broadcast(ws.Event{Type: WSEventFactions, Data: state})
}
