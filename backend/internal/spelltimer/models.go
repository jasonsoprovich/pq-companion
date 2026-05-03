// Package spelltimer tracks active EverQuest spell timers (buffs, debuffs, DoTs,
// mez, stuns) by watching parsed log events. Durations are calculated using
// EQ's server-tick formula (1 tick = 6 seconds). Timer state is broadcast
// over WebSocket and exposed via REST.
package spelltimer

import "time"

// WSEventTimers is the WebSocket event type emitted when timer state changes.
const WSEventTimers = "overlay:timers"

// eqTickSeconds is the duration of one EverQuest server tick in seconds.
const eqTickSeconds = 6.0

// defaultCasterLevel is used for duration calculations when character level is
// not configured. Project Quarm's level cap is 60, so most formulas hit their
// cap and the result is accurate for a max-level character.
const defaultCasterLevel = 60

// Category classifies an active spell timer by its game mechanic role.
type Category string

const (
	CategoryBuff   Category = "buff"
	CategoryDebuff Category = "debuff"
	CategoryMez    Category = "mez"
	CategoryDot    Category = "dot"
	CategoryStun   Category = "stun"
)

// ActiveTimer represents one spell that is currently ticking down.
type ActiveTimer struct {
	// ID is the unique key in the engine's timer map. For spell-derived
	// timers it's "<spell name>@<target name>"; for trigger-driven timers
	// (no associated target) it's "<trigger name>@".
	ID        string `json:"id"`
	SpellName string `json:"spell_name"`
	SpellID   int    `json:"spell_id"`
	// Icon is spells_new.new_icon for the source spell, used by the UI to
	// render a gembook-style icon next to each timer bar. 0 for
	// trigger-driven timers that don't have a resolved spell.
	Icon int `json:"icon,omitempty"`

	// TargetName is the recipient of the spell. For self-cast / buffs on
	// the active player it's the player's character name (the engine
	// resolves "you" / cast_on_you events to the active character so the
	// key is consistent regardless of which character's log we read).
	// Empty for trigger-driven timers that don't carry a target.
	TargetName string `json:"target_name"`

	Category Category `json:"category"`

	// CastAt is the log timestamp when the spell took effect (post PR1-3 this
	// is the same as StartsAt; pre-PR1-3 it was the begin-cast timestamp).
	CastAt time.Time `json:"cast_at"`
	// StartsAt is when the spell lands and the timer begins counting down.
	StartsAt time.Time `json:"starts_at"`
	// ExpiresAt is when the buff is expected to fade.
	ExpiresAt time.Time `json:"expires_at"`

	DurationSeconds  float64 `json:"duration_seconds"`
	RemainingSeconds float64 `json:"remaining_seconds"`

	// DisplayThresholdSecs is a per-timer override for the user-configured
	// global display threshold. > 0 means "only show me when remaining
	// time falls at or below this value"; 0 means "let the frontend resolve
	// against the global default for my category". Set on a per-trigger
	// basis (Trigger.DisplayThresholdSecs); spell-landed-driven timers
	// always emit 0 here so a config change updates them retroactively.
	DisplayThresholdSecs int `json:"display_threshold_secs"`
}

// TimerState is the full payload broadcast via WebSocket and returned by the
// REST API. Timers are sorted by remaining time ascending (most urgent first).
type TimerState struct {
	Timers      []ActiveTimer `json:"timers"`
	LastUpdated time.Time     `json:"last_updated"`
}
