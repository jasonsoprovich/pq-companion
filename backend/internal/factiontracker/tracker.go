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

// NPCPrimaryFactionResolver resolves a /con'd NPC's display name to the name
// of its primary faction (npc_faction.primaryfaction) — the faction /con's
// disposition message actually reflects. Returns ok=false when the name
// can't be resolved or the NPC has no faction.
type NPCPrimaryFactionResolver func(npcName string) (factionName string, ok bool)

// IsIllusionedProvider reports whether the active character currently has an
// illusion effect active. Illusions change how NPCs perceive the player, so
// a /con reading taken while illusioned is flagged as suspect rather than
// trusted at face value.
type IsIllusionedProvider func() bool

// PersistFunc is called after every tally mutation with the full current
// state of that one faction's tally, so the caller can write it to durable
// storage. Called outside the engine's lock.
type PersistFunc func(characterID int, tally Tally)

// ClearPersistedFunc is called by Reset to wipe durable storage for the
// character — Reset means "discard this character's faction tracking
// history," not "start a fresh session," since tallies now persist across
// restarts.
type ClearPersistedFunc func(characterID int)

// pendingKill is a resolved kill's expected faction deltas, waiting to be
// matched against the EventFactionChanged lines that should follow within
// correlationWindow. hits is keyed by lowercased faction name; a matched
// entry is deleted so a multi-faction kill's lines each consume their own
// faction once.
type pendingKill struct {
	at   time.Time
	hits map[string]int
}

// Engine tracks per-character faction-standing changes for wishlisted
// factions, inferred from the EQ log feed. Safe for concurrent use.
type Engine struct {
	hub            *ws.Hub
	resolve        NPCFactionResolver
	resolvePrimary NPCPrimaryFactionResolver
	isIllusioned   IsIllusionedProvider
	persist        PersistFunc
	clearPersisted ClearPersistedFunc

	mu          sync.Mutex
	characterID int
	order       []string          // tracked faction keys (lowercased name), wishlist order
	tallies     map[string]*Tally // key: strings.ToLower(FactionName)
	pending     []pendingKill
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

// SetPrimaryFactionResolver registers the resolver used to correlate /con
// readings to a tracked faction. Optional — /con correlation is skipped
// entirely if never set.
func (e *Engine) SetPrimaryFactionResolver(fn NPCPrimaryFactionResolver) {
	e.mu.Lock()
	e.resolvePrimary = fn
	e.mu.Unlock()
}

// SetIllusionProvider registers the callback used to flag /con readings
// taken while illusioned. Optional — readings are never flagged suspect if
// never set.
func (e *Engine) SetIllusionProvider(fn IsIllusionedProvider) {
	e.mu.Lock()
	e.isIllusioned = fn
	e.mu.Unlock()
}

// SetPersistFunc registers the callback invoked after every tally mutation.
// Optional — the engine works in-memory-only if never set (e.g. tests).
func (e *Engine) SetPersistFunc(fn PersistFunc) {
	e.mu.Lock()
	e.persist = fn
	e.mu.Unlock()
}

// SetClearPersistedFunc registers the callback invoked by Reset to wipe
// durable storage. Optional.
func (e *Engine) SetClearPersistedFunc(fn ClearPersistedFunc) {
	e.mu.Lock()
	e.clearPersisted = fn
	e.mu.Unlock()
}

// SetTracked replaces the set of factions being tracked and the character
// they belong to, in display order, seeding each tally from f.Seed (the
// persisted state the caller loaded from storage — the zero value for a
// faction with no history yet). Called on startup and whenever the active
// character or its faction wishlist changes. Pending kill correlations are
// dropped: they're specific to whichever character's log produced them, and
// don't carry over to a different tracked set.
func (e *Engine) SetTracked(characterID int, factions []TrackedFaction) {
	e.mu.Lock()
	e.characterID = characterID
	newTallies := make(map[string]*Tally, len(factions))
	newOrder := make([]string, 0, len(factions))
	for _, f := range factions {
		key := strings.ToLower(f.FactionName)
		t := f.Seed
		t.FactionID = f.FactionID
		t.FactionName = f.FactionName
		newTallies[key] = &t
		newOrder = append(newOrder, key)
	}
	e.tallies = newTallies
	e.order = newOrder
	e.pending = nil
	state := e.stateLocked()
	e.mu.Unlock()
	e.broadcast(state)
}

// Reset zeroes every tracked faction's tally (including the /con reading)
// and drops pending kill correlations, then wipes durable storage for the
// character via ClearPersistedFunc — an explicit, user-initiated "discard my
// faction tracking history," not something that happens automatically on
// restart or character switch.
func (e *Engine) Reset() {
	e.mu.Lock()
	charID := e.characterID
	for _, t := range e.tallies {
		t.Better, t.Worse, t.EstimatedNet, t.Unresolved = 0, 0, 0, 0
		t.LastBucket, t.LastConsideredAt, t.LastConsiderSuspect = "", nil, false
	}
	e.pending = nil
	state := e.stateLocked()
	e.mu.Unlock()

	e.broadcast(state)
	if e.clearPersisted != nil {
		e.clearPersisted(charID)
	}
}

// State returns a snapshot of the current tallies.
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
	case logparser.EventConsidered:
		data, ok := ev.Data.(logparser.ConsideredData)
		if !ok || data.Bucket == "" {
			return
		}
		e.handleConsidered(data.TargetName, data.Bucket, ev.Timestamp)
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
	snapshot := *tally
	charID := e.characterID
	state := e.stateLocked()
	e.mu.Unlock()

	e.broadcast(state)
	if e.persist != nil {
		e.persist(charID, snapshot)
	}
}

// handleConsidered resolves a /con'd NPC to its primary faction and, if that
// faction is tracked, records the disposition bucket as the faction's latest
// reading — flagged suspect if the player was illusioned at the time.
func (e *Engine) handleConsidered(npcName string, bucket logparser.FactionBucket, ts time.Time) {
	if e.resolvePrimary == nil {
		return
	}
	factionName, ok := e.resolvePrimary(npcName)
	if !ok {
		return
	}
	key := strings.ToLower(factionName)

	suspect := false
	if e.isIllusioned != nil {
		suspect = e.isIllusioned()
	}

	e.mu.Lock()
	tally, tracked := e.tallies[key]
	if !tracked {
		e.mu.Unlock()
		return
	}
	tally.LastBucket = string(bucket)
	tally.LastConsideredAt = &ts
	tally.LastConsiderSuspect = suspect
	snapshot := *tally
	charID := e.characterID
	state := e.stateLocked()
	e.mu.Unlock()

	e.broadcast(state)
	if e.persist != nil {
		e.persist(charID, snapshot)
	}
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
