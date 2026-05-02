// Package trigger implements a regex-based trigger engine that matches
// EverQuest log lines and fires configured actions (overlay text, history log).
package trigger

import (
	"errors"
	"time"
)

// WSEventTriggerFired is the WebSocket event type emitted when a trigger fires.
const WSEventTriggerFired = "trigger:fired"

// ActionType identifies the kind of action a trigger fires.
type ActionType string

const (
	// ActionOverlayText displays text in the trigger overlay window.
	ActionOverlayText ActionType = "overlay_text"
	// ActionPlaySound plays a local audio file.
	ActionPlaySound ActionType = "play_sound"
	// ActionTextToSpeech speaks text aloud using the system TTS engine.
	ActionTextToSpeech ActionType = "text_to_speech"
)

// TimerType identifies the overlay a trigger-driven timer should appear on.
type TimerType string

const (
	// TimerTypeNone means the trigger does not start a spell timer on match.
	TimerTypeNone TimerType = "none"
	// TimerTypeBuff starts a timer on the Buff overlay when the trigger fires.
	TimerTypeBuff TimerType = "buff"
	// TimerTypeDetrimental starts a timer on the Detrimental overlay.
	TimerTypeDetrimental TimerType = "detrimental"
)

// Action describes a single effect fired when a trigger matches a log line.
type Action struct {
	Type         ActionType `json:"type"`
	Text         string     `json:"text"`          // overlay text or TTS speech text
	DurationSecs int        `json:"duration_secs"` // seconds to show overlay text (0 = default 5s)
	Color        string     `json:"color"`         // optional hex color for overlay text
	SoundPath    string     `json:"sound_path"`    // local file path for play_sound action
	Volume       float64    `json:"volume"`        // playback volume 0.0–1.0 (0 = default 1.0)
	Voice        string     `json:"voice"`         // TTS voice name (empty = system default)
}

// Trigger is a user-defined log line matcher with associated actions.
type Trigger struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Enabled   bool      `json:"enabled"`
	Pattern   string    `json:"pattern"` // regexp matched against the message portion of log lines
	Actions   []Action  `json:"actions"`
	PackName  string    `json:"pack_name"` // empty for user-created triggers; pack name for built-in packs
	CreatedAt time.Time `json:"created_at"`

	// Timer integration — when TimerType is buff or detrimental, a match starts
	// a countdown timer on the corresponding overlay. WornOffPattern optionally
	// clears the timer before its natural expiry.
	TimerType         TimerType `json:"timer_type"`
	TimerDurationSecs int       `json:"timer_duration_secs"`
	WornOffPattern    string    `json:"worn_off_pattern"`
	SpellID           int       `json:"spell_id"` // optional — 0 means not linked to a specific DB spell

	// DisplayThresholdSecs is a per-trigger override for the global buff /
	// detrimental display threshold. When > 0, the timer this trigger
	// creates is only shown in the overlay once its remaining time falls
	// at or below this value. 0 (default) means "use the global default
	// for my category"; the frontend resolves against the SpellTimer
	// settings in user config.
	DisplayThresholdSecs int `json:"display_threshold_secs"`
}

// TriggerFired is the payload of a WSEventTriggerFired WebSocket event and a
// history entry. It describes a single trigger match that occurred at runtime.
type TriggerFired struct {
	TriggerID   string    `json:"trigger_id"`
	TriggerName string    `json:"trigger_name"`
	MatchedLine string    `json:"matched_line"`
	Actions     []Action  `json:"actions"`
	FiredAt     time.Time `json:"fired_at"`
}

// TriggerPack is a portable collection of triggers used for import/export and
// built-in packs.
type TriggerPack struct {
	PackName    string    `json:"pack_name"`
	Description string    `json:"description"`
	Triggers    []Trigger `json:"triggers"`
}

// ErrNotFound is returned when a requested trigger does not exist in the store.
var ErrNotFound = errors.New("trigger not found")
