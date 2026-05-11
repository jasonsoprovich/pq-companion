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
// Max. The tracker mirrors that convention: each unique Max gets its
// own session, and rolls are bucketed into the matching session.
package rolltracker

import "time"

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

// Session is the set of rolls collected for a single dice range (Max).
type Session struct {
	// ID is a process-local monotonic identifier. Lets the UI target a
	// specific session for Stop/Remove even when multiple sessions share
	// the same Max (e.g. two rolls on identical-bound drops in one raid).
	ID uint64 `json:"id"`
	// Max is the upper bound of the dice range — the "333" in "any
	// number from 0 to 333". Used to bucket new rolls into the right
	// active session.
	Max int `json:"max"`
	// StartedAt is when the first roll for this session arrived.
	StartedAt time.Time `json:"started_at"`
	// LastRollAt is the timestamp of the most recently received roll.
	LastRollAt time.Time `json:"last_roll_at"`
	// Active is true while new rolls may still arrive. The user toggles
	// this off via the Stop button — once stopped, late rolls (e.g. a
	// player rolling on a different drop that happens to share a Max)
	// open a fresh session at the same Max.
	Active bool `json:"active"`
	// Rolls is every roll received for this session in arrival order.
	Rolls []Roll `json:"rolls"`
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
}
