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
	// ID is the spell name — used as the unique key (one timer per spell name).
	ID       string `json:"id"`
	SpellName string `json:"spell_name"`
	SpellID   int    `json:"spell_id"`

	Category Category `json:"category"`

	// CastAt is the log timestamp when "You begin casting X" was seen.
	CastAt time.Time `json:"cast_at"`
	// StartsAt is when the spell lands (CastAt + cast time from DB).
	StartsAt time.Time `json:"starts_at"`
	// ExpiresAt is when the buff is expected to fade.
	ExpiresAt time.Time `json:"expires_at"`

	DurationSeconds  float64 `json:"duration_seconds"`
	RemainingSeconds float64 `json:"remaining_seconds"`
}

// TimerState is the full payload broadcast via WebSocket and returned by the
// REST API. Timers are sorted by remaining time ascending (most urgent first).
type TimerState struct {
	Timers      []ActiveTimer `json:"timers"`
	LastUpdated time.Time     `json:"last_updated"`
}
