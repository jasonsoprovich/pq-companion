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
)

// Action describes a single effect fired when a trigger matches a log line.
type Action struct {
	Type         ActionType `json:"type"`
	Text         string     `json:"text"`          // text to display in the overlay
	DurationSecs int        `json:"duration_secs"` // seconds to show the overlay text (0 = default 5s)
	Color        string     `json:"color"`         // optional hex color string (e.g. "#ff4444")
}

// Trigger is a user-defined log line matcher with associated actions.
type Trigger struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Enabled   bool      `json:"enabled"`
	Pattern   string    `json:"pattern"`   // regexp matched against the message portion of log lines
	Actions   []Action  `json:"actions"`
	PackName  string    `json:"pack_name"` // empty for user-created triggers; pack name for built-in packs
	CreatedAt time.Time `json:"created_at"`
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
