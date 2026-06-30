// Package rolltracker groups EQ /random rolls into per-range sessions so
// the UI can show live tallies of who rolled what for which raid drop.
//
// EQ logs every /random result as two consecutive lines:
//
//	**A Magic Die is rolled by <Name>.
//	**It could have been any number from 0 to <Max>, but this time it turned up a <Value>.
//
// Raiders commonly pre-announce different drops with different upper
// bounds (e.g. "Cowl of Mortality 333", "Massive Dragonclaw Shard 444")
// so that simultaneously-ongoing rolls can be distinguished by their
// Max. The tracker mirrors that convention: each unique range (Min–Max)
// gets its own session, and rolls are bucketed into the matching
// session. The Min is almost always 0 (the EQ default for "/random N"),
// so most sessions are "0–N"; a player who rolls with an explicit
// non-zero floor ("/random 222 611") gets a separate 222–611 session
// instead of being lumped in with the 0–611 rolls.
package rolltracker

import (
	"fmt"
	"time"
)

// WSEventRolls is the WebSocket event type broadcast on every roll-state
// change (new roll, session stopped, settings changed, cleared).
const WSEventRolls = "overlay:rolls"

// WinnerRule selects how a session's winner is chosen.
type WinnerRule string

const (
	// WinnerHighest selects the highest roll value as the winner — the
	// most common guild rule.
	WinnerHighest WinnerRule = "highest"
	// WinnerLowest selects the lowest roll value — used by some guilds for
	// "low roll wins" loot rules.
	WinnerLowest WinnerRule = "lowest"
)

// Mode selects how sessions get closed.
type Mode string

const (
	// ModeManual leaves sessions open until the user clicks Stop or the
	// session goes stale (no rolls for staleAfter).
	ModeManual Mode = "manual"
	// ModeTimer auto-stops every new session AutoStopSeconds after its
	// first roll. Officers use this when they want a fixed bidding
	// window per drop ("/random 45s window").
	ModeTimer Mode = "timer"
)

// DefaultAutoStopSeconds is the timer-mode window length applied to new
// sessions when the client hasn't supplied one. 45s matches the common
// guild "you have N seconds to roll" call-out.
const DefaultAutoStopSeconds = 45

// Roll is one logged dice roll within a session.
type Roll struct {
	// Roller is the player name as EQ logged it.
	Roller string `json:"roller"`
	// Value is the rolled number.
	Value int `json:"value"`
	// Timestamp is the EQ-log timestamp on the roll-result line.
	Timestamp time.Time `json:"timestamp"`
	// Duplicate is true when this is not the first roll this player made
	// in the session. Duplicates are kept in the list (so users see them)
	// but excluded from winner calculation — most guilds treat the first
	// roll as binding.
	Duplicate bool `json:"duplicate"`
}

// Session is the set of rolls collected for a single dice range
// (Min–Max).
type Session struct {
	// ID is a process-local monotonic identifier. Lets the UI target a
	// specific session for Stop/Remove even when multiple sessions share
	// the same range (e.g. two rolls on identical-bound drops in one raid).
	ID uint64 `json:"id"`
	// Min is the lower bound of the dice range — the "0" in "any number
	// from 0 to 333". Almost always 0 (the EQ default for "/random N"),
	// but a player who types "/random 222 611" rolls 222–611, which the
	// tracker keeps in its own session rather than mixing it in with the
	// 0–611 rolls (a non-zero floor gives an unfair edge under a
	// highest-wins rule).
	Min int `json:"min"`
	// Max is the upper bound of the dice range — the "333" in "any
	// number from 0 to 333". Together with Min it buckets new rolls into
	// the right active session.
	Max int `json:"max"`
	// StartedAt is when the first roll for this session arrived.
	StartedAt time.Time `json:"started_at"`
	// LastRollAt is the timestamp of the most recently received roll.
	LastRollAt time.Time `json:"last_roll_at"`
	// ItemName is the loot item this roll is for, e.g. "Robe of the
	// Lost Circle". Empty until the user labels the session (or a future
	// best-effort auto-parse fills it in). Purely descriptive — it does
	// not affect bucketing or winner selection, but it's surfaced in the
	// UI and the copy-to-paste summary so raid leaders can announce the
	// result with the item attached.
	ItemName string `json:"item_name"`
	// Active is true while new rolls may still arrive. The user toggles
	// this off via the Stop button — once stopped, late rolls (e.g. a
	// player rolling on a different drop that happens to share a Max)
	// open a fresh session at the same Max.
	Active bool `json:"active"`
	// AutoStopAt is the wall-clock time the session will auto-close in
	// timer mode. Zero when in manual mode or after the timer has
	// already fired. The UI uses this to render a countdown badge.
	AutoStopAt time.Time `json:"auto_stop_at,omitempty"`
	// Rolls is every roll received for this session in arrival order.
	Rolls []Roll `json:"rolls"`

	// autoSuggested records that the best-effort loot-item auto-suggest has
	// already had its one shot at this session, so a later matching chat
	// line doesn't repeatedly re-fill (or fight a label the user typed).
	// Unexported: internal bookkeeping, never serialized.
	autoSuggested bool
}

// ProfileMode selects whether rolls are grouped into tiered contests or
// shown as today's flat per-range sessions.
type ProfileMode string

const (
	// ProfileSimple is the default: each /random range is its own session,
	// winner picked by the global highest/lowest rule. No grouping.
	ProfileSimple ProfileMode = "simple"
	// ProfileTiered groups multiple ranges into one contest with ranked
	// tiers (e.g. "111 pick / 122 upgrade / 133 alt"), the winner taken
	// from the highest-priority tier that received any rolls.
	ProfileTiered ProfileMode = "tiered"
)

// ProfileScheme selects how a tiered profile maps a roll's upper bound to a
// tier and to a contest.
type ProfileScheme string

const (
	// SchemeSuffix derives the tier from max%Divisor and the contest group
	// from max/Divisor — so "111/122/133" are three tiers of item group 1
	// and "211/222/233" are item group 2. One profile handles any number
	// of items. This is the Breakfast Club "1xx" convention.
	SchemeSuffix ProfileScheme = "suffix"
	// SchemeExact matches the tier on the exact max value and groups by
	// time (one contest at a time) — the pickup "need 111 / greed 222"
	// convention where a single item is rolled at once.
	SchemeExact ProfileScheme = "exact"
)

// ProfileTier is one bracket within a tiered profile. Its priority is its
// index in RollProfile.Tiers: index 0 outranks everything below it.
type ProfileTier struct {
	// Match is compared against the derived tier key — max%Divisor under
	// the suffix scheme, or the raw max under the exact scheme.
	Match int `json:"match"`
	// Label is the human name for the bracket (e.g. "Pick", "Greed").
	Label string `json:"label"`
}

// RollProfile defines how incoming rolls are grouped into tiered contests.
// The backend stores, validates, and broadcasts it but does not interpret
// it — grouping and tier-winner selection are derived client-side, the same
// way the flat winner rule already is. The zero value is treated as simple.
type RollProfile struct {
	Mode    ProfileMode   `json:"mode"`
	Scheme  ProfileScheme `json:"scheme,omitempty"`
	Divisor int           `json:"divisor,omitempty"`
	Tiers   []ProfileTier `json:"tiers,omitempty"`
}

// Validate reports whether the profile is well-formed, normalizing the zero
// value to simple. Returns the normalized profile and an error describing
// the first problem found.
func (p RollProfile) Validate() (RollProfile, error) {
	if p.Mode == "" || p.Mode == ProfileSimple {
		return RollProfile{Mode: ProfileSimple}, nil
	}
	if p.Mode != ProfileTiered {
		return p, fmt.Errorf("mode must be %q or %q", ProfileSimple, ProfileTiered)
	}
	if p.Scheme != SchemeSuffix && p.Scheme != SchemeExact {
		return p, fmt.Errorf("scheme must be %q or %q", SchemeSuffix, SchemeExact)
	}
	if p.Scheme == SchemeSuffix && p.Divisor <= 0 {
		p.Divisor = 100
	}
	if len(p.Tiers) == 0 {
		return p, fmt.Errorf("a tiered profile needs at least one tier")
	}
	seen := make(map[int]bool, len(p.Tiers))
	for _, t := range p.Tiers {
		if t.Label == "" {
			return p, fmt.Errorf("every tier needs a label")
		}
		if seen[t.Match] {
			return p, fmt.Errorf("duplicate tier match value %d", t.Match)
		}
		seen[t.Match] = true
	}
	return p, nil
}

// State is the full tracker payload broadcast over WebSocket and returned
// from GET /api/rolls.
type State struct {
	// Sessions is the active + recently-stopped sessions in start order,
	// newest first.
	Sessions []Session `json:"sessions"`
	// WinnerRule is the current global winner-selection rule. The UI
	// derives per-session winners from this; the backend never stores a
	// frozen winner per session because flipping the rule should
	// re-rank all sessions instantly.
	WinnerRule WinnerRule `json:"winner_rule"`
	// Mode is how sessions get closed (manual vs. timer).
	Mode Mode `json:"mode"`
	// AutoStopSeconds is the timer-mode session length applied to every
	// new session. Ignored when Mode == ModeManual.
	AutoStopSeconds int `json:"auto_stop_seconds"`
	// Profile is the active grouping profile. The UI uses it to fold the
	// flat Sessions list into tiered contests; "simple" (the default) means
	// no grouping.
	Profile RollProfile `json:"profile"`
}
