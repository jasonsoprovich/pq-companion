// Package factiontracker maintains a per-character tally of faction-standing
// changes for every faction the character has ever killed toward or
// /con'd — not just the ones pinned to the wishlist, the same "track
// everything encountered" approach as the Lockout and Player trackers.
// Pinning (character_faction_wishlist) is purely a display favorite,
// resolved by the caller; it never gates what the engine records.
//
// EQ never logs a faction's absolute value or the point amount of a change —
// only "Your faction standing with <Faction> got better/worse." with no
// number attached. When the line's timestamp lines up with a kill the
// tracker can resolve to a specific NPC, the NPC's quarm.db
// npc_faction_entries row supplies a best-effort point estimate; otherwise
// (quest turn-ins, hails, or an NPC the DB can't resolve) the event still
// counts toward the better/worse tally but contributes no estimate.
//
// The running tally persists across restarts and character switches (the
// caller is responsible for loading/saving it — see PersistFunc), but it is
// still never a claim about the character's real faction value: there is no
// way to read that from the server, only to estimate its direction of drift.
// The /con bucket (see logparser.FactionBucket) is the one piece of ground
// truth EQ gives us, which is why the tracker also records the most recent
// disposition reading for a faction's NPCs alongside the estimate.
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

// Tally is the running tally for one faction the character has ever killed
// toward or /con'd, persisted per character across restarts. Tally itself
// doubles as the seed type passed to SetCharacter — there is no separate
// "tracked faction" concept; every faction with any recorded activity gets
// one of these.
type Tally struct {
	FactionID   int    `json:"faction_id"`
	FactionName string `json:"faction_name"`
	// Better/Worse are raw counts of "got better"/"got worse" log lines
	// observed for this faction, regardless of whether a point estimate
	// could be attached.
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
	// LastBucket is the most recent /con disposition bucket read for one of
	// this faction's primary-faction NPCs (see logparser.FactionBucket), or
	// "" if never considered. This is the one piece of ground truth EQ
	// exposes — everything else on this struct is an estimate.
	LastBucket string `json:"last_bucket,omitempty"`
	// LastConsideredAt is when LastBucket was read, nil if never set. A
	// pointer (rather than the time.Time zero value) so omitempty actually
	// omits it in JSON when unset — encoding/json's omitempty has no effect
	// on non-pointer struct fields.
	LastConsideredAt *time.Time `json:"last_considered_at,omitempty"`
	// LastConsiderSuspect flags that LastBucket may be wrong because the
	// player had an illusion active at the time of the reading — illusions
	// change how NPCs perceive (and therefore /con) the player.
	LastConsiderSuspect bool `json:"last_consider_suspect,omitempty"`
}

// State is the full tracker payload broadcast over WebSocket and returned by
// the REST API.
type State struct {
	Tallies []Tally `json:"tallies"`
}
