// Package factiontracker maintains a per-session tally of faction-standing
// changes for factions the user has wishlisted, inferred from EQ's kill and
// faction-change log lines.
//
// EQ never logs a faction's absolute value or the point amount of a change —
// only "Your faction standing with <Faction> got better/worse." with no
// number attached. When the line's timestamp lines up with a kill the
// tracker can resolve to a specific NPC, the NPC's quarm.db
// npc_faction_entries row supplies a best-effort point estimate; otherwise
// (quest turn-ins, hails, or an NPC the DB can't resolve) the event still
// counts toward the better/worse tally but contributes no estimate. State is
// session-only — there is no persisted baseline, and none is possible
// without server-side access to the character's real faction value.
package factiontracker

import "time"

// WSEventFactions is the WebSocket event type broadcast on every tally change.
const WSEventFactions = "overlay:factions"

// correlationWindow bounds how long a resolved kill's expected faction hits
// stay valid waiting for their "Your faction standing…" lines. Every sample
// checked while researching this feature fired the faction lines at the same
// second as the kill line, so this is generous headroom, not a tight fit.
const correlationWindow = 5 * time.Second

// maxPendingKills bounds the pending-kill backlog so a burst of unresolved
// kills (e.g. an NPC name the DB can't resolve, or fast trash clearing with
// tracking off) can't grow it unbounded.
const maxPendingKills = 50

// TrackedFaction is one faction the active character has wishlisted for
// session tracking.
type TrackedFaction struct {
	FactionID   int
	FactionName string
}

// Tally is the running session count for one tracked faction.
type Tally struct {
	FactionID   int    `json:"faction_id"`
	FactionName string `json:"faction_name"`
	// Better/Worse are raw counts of "got better"/"got worse" log lines
	// observed this session for this faction, regardless of whether a
	// point estimate could be attached.
	Better int `json:"better"`
	Worse  int `json:"worse"`
	// EstimatedNet sums the quarm.db npc_faction_entries point values for
	// every change that correlated to a resolved kill. Purely additive
	// across kill sources; not a claim about the character's absolute
	// faction value.
	EstimatedNet int `json:"estimated_net"`
	// Unresolved counts changes that could not be matched to a kill this
	// tracker could resolve to an NPC (quest turn-ins, hails, or an NPC name
	// not found/ambiguous in quarm.db) — direction-only, no estimate.
	Unresolved int `json:"unresolved"`
}

// State is the full tracker payload broadcast over WebSocket and returned by
// the REST API.
type State struct {
	Tallies []Tally `json:"tallies"`
}
