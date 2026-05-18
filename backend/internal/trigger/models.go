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

// ActionPosition is the on-screen placement of an overlay_text alert.
// Coordinates are in the trigger overlay window's local space (top-left
// origin), in CSS pixels. Nil means the renderer should use the default
// stacking layout.
type ActionPosition struct {
	X int `json:"x"`
	Y int `json:"y"`
}

// Action describes a single effect fired when a trigger matches a log line.
type Action struct {
	Type         ActionType `json:"type"`
	Text         string     `json:"text"`          // overlay text or TTS speech text
	DurationSecs int        `json:"duration_secs"` // seconds to show overlay text (0 = default 5s)
	Color        string     `json:"color"`         // optional hex color for overlay text
	SoundPath    string     `json:"sound_path"`    // local file path for play_sound action
	Volume       float64    `json:"volume"`        // playback volume 0.0–1.0 (0 = default 1.0)
	Voice        string     `json:"voice"`         // TTS voice name (empty = system default)

	// Position pins this overlay_text action to a fixed location instead of
	// the default stacking layout. Nil = stack with other alerts.
	Position *ActionPosition `json:"position,omitempty"`
	// FontSize overrides the default overlay font size in CSS pixels.
	// 0 = use the renderer default.
	FontSize int `json:"font_size,omitempty"`
}

// TimerAlertType identifies the kind of audio alert fired when a timer-bound
// trigger crosses one of its configured "fading soon" thresholds.
type TimerAlertType string

const (
	TimerAlertTypePlaySound    TimerAlertType = "play_sound"
	TimerAlertTypeTextToSpeech TimerAlertType = "text_to_speech"
)

// TimerAlert is a per-trigger "fading soon" notification that fires when the
// trigger's spell timer crosses a remaining-seconds threshold. Each trigger
// can carry an arbitrary list (e.g. 300s + 60s for a long buff, 10s for a
// mez). Empty list = no fading alert.
type TimerAlert struct {
	ID          string         `json:"id"`
	Seconds     int            `json:"seconds"`
	Type        TimerAlertType `json:"type"`
	SoundPath   string         `json:"sound_path"`
	Volume      int            `json:"volume"` // 0–100
	TTSTemplate string         `json:"tts_template"`
	Voice       string         `json:"voice"`
	TTSVolume   int            `json:"tts_volume"` // 0–100
}

// SourceLog and SourcePipe are the legal values for Trigger.Source.
const (
	SourceLog  = "log"
	SourcePipe = "pipe"
)

// PipeConditionKind identifies which kind of ZealPipe event a pipe-source
// trigger matches.
type PipeConditionKind string

const (
	// PipeKindTargetHPBelow fires when the target's HP percentage transitions
	// from above HPThreshold to at-or-below. Refires only after HP rises back
	// above threshold and drops again — no spam at every pipe tick.
	PipeKindTargetHPBelow PipeConditionKind = "target_hp_below"
	// PipeKindTargetName fires when the player's target name becomes the
	// configured TargetName (case-sensitive). Dedupes on consecutive identical
	// matches.
	PipeKindTargetName PipeConditionKind = "target_name"
	// PipeKindBuffLanded fires when SpellName appears in the player's buff
	// slots (transition from absent to present).
	PipeKindBuffLanded PipeConditionKind = "buff_landed"
	// PipeKindBuffFaded fires when SpellName disappears from the player's
	// buff slots (transition from present to absent).
	PipeKindBuffFaded PipeConditionKind = "buff_faded"
	// PipeKindPipeCommand fires when the player types `/pipe <Text>` in-game
	// and the text matches (case-sensitive, exact). One-shot per /pipe entry.
	PipeKindPipeCommand PipeConditionKind = "pipe_command"
)

// PipeCondition is the typed match definition for a pipe-source trigger.
// Fields are populated based on Kind — see the PipeKind* constants. JSON
// shape: { "kind": "...", "hp_threshold": 20, "spell_name": "...", ... }
type PipeCondition struct {
	Kind        PipeConditionKind `json:"kind"`
	HPThreshold int               `json:"hp_threshold,omitempty"` // PipeKindTargetHPBelow: 0–100
	SpellName   string            `json:"spell_name,omitempty"`   // PipeKindBuffLanded / PipeKindBuffFaded
	TargetName  string            `json:"target_name,omitempty"`  // PipeKindTargetName
	Text        string            `json:"text,omitempty"`         // PipeKindPipeCommand
}

// Trigger is a user-defined matcher with associated actions. Today it matches
// either log lines (Source=="log", regex on Pattern) or ZealPipe events
// (Source=="pipe", typed match on PipeCondition).
type Trigger struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Enabled   bool      `json:"enabled"`
	Pattern   string    `json:"pattern"` // regexp matched against the message portion of log lines (Source=="log" only)
	Actions   []Action  `json:"actions"`
	PackName  string    `json:"pack_name"` // empty for user-created triggers; pack name for built-in packs
	CreatedAt time.Time `json:"created_at"`

	// Source selects which input stream the trigger matches against:
	//   "log"  (default, omitted in JSON) — Pattern regex on log lines.
	//   "pipe" — PipeCondition kind/fields matched against ZealPipe events.
	// Existing rows with no `source` column default to "log".
	Source string `json:"source,omitempty"`

	// PipeCondition is the typed match definition for Source=="pipe" triggers.
	// Kind discriminates between target_hp_below / target_name / buff_landed /
	// buff_faded / pipe_command. Empty for log-source triggers.
	PipeCondition *PipeCondition `json:"pipe_condition,omitempty"`

	// Timer integration — when TimerType is buff or detrimental, a match starts
	// a countdown timer on the corresponding overlay. WornOffPattern optionally
	// clears the timer before its natural expiry.
	TimerType         TimerType `json:"timer_type"`
	TimerDurationSecs int       `json:"timer_duration_secs"`
	WornOffPattern    string    `json:"worn_off_pattern"`
	SpellID           int       `json:"spell_id"` // optional — 0 means not linked to a specific DB spell

	// CooldownSecs spawns a second timer alongside the buff/duration timer to
	// track the spell or discipline's reuse cooldown (recast_time in
	// spells_new). Counts down on the buff overlay with a " CD" suffix in the
	// label and a "ready" TTS at 1s remaining. 0 = no cooldown timer.
	// Independent of TimerType — works even when the primary trigger is an
	// overlay-only alert (Lay on Hands, Harvest, etc.).
	CooldownSecs int `json:"cooldown_secs,omitempty"`

	// DisplayThresholdSecs is a per-trigger override for the global buff /
	// detrimental display threshold. When > 0, the timer this trigger
	// creates is only shown in the overlay once its remaining time falls
	// at or below this value. 0 (default) means "use the global default
	// for my category"; the frontend resolves against the SpellTimer
	// settings in user config.
	DisplayThresholdSecs int `json:"display_threshold_secs"`

	// Characters lists the character names this trigger fires for. Empty =
	// fires for any active character (legacy + safety fallback). The frontend
	// presents this as toggleable chips in the edit modal.
	Characters []string `json:"characters"`

	// TimerAlerts are the per-trigger "fading soon" thresholds. Each entry
	// fires an audio cue (play_sound or TTS) when the timer this trigger
	// creates crosses the configured remaining-seconds value. Empty = no
	// fading notification (the timer still counts down silently).
	TimerAlerts []TimerAlert `json:"timer_alerts"`

	// ExcludePatterns are regexes that suppress the trigger when any of them
	// also match the same log line. The intended use is filtering a broad
	// primary pattern: e.g. an "incoming tell" trigger whose pattern is
	// `\w+ tells you,` excludes pet responses (, Master.') and NPC merchant
	// canned phrases (That'll be, I'll give you, etc.) so genuine player
	// tells are the only matches that fire actions.
	ExcludePatterns []string `json:"exclude_patterns"`
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
	// Class is the EQ class index this pack targets (0=Warrior … 14=Beastlord).
	// nil means class-agnostic (e.g. GroupAwareness, user-authored packs that
	// don't specify a class). On import, class-specific packs default their
	// Characters lists to the matching-class characters only, instead of every
	// known character. Pointer so missing-in-JSON is distinguishable from
	// explicit 0 (=Warrior).
	Class    *int      `json:"class,omitempty"`
	Triggers []Trigger `json:"triggers"`
}

// EQ class indices used by TriggerPack.Class and character.Character.Class.
const (
	ClassWarrior      = 0
	ClassCleric       = 1
	ClassPaladin      = 2
	ClassRanger       = 3
	ClassShadowknight = 4
	ClassDruid        = 5
	ClassMonk         = 6
	ClassBard         = 7
	ClassRogue        = 8
	ClassShaman       = 9
	ClassNecromancer  = 10
	ClassWizard       = 11
	ClassMagician     = 12
	ClassEnchanter    = 13
	ClassBeastlord    = 14
)

// ClassPtr returns a pointer to the given class index, for setting
// TriggerPack.Class concisely.
func ClassPtr(c int) *int { return &c }

// ErrNotFound is returned when a requested trigger does not exist in the store.
var ErrNotFound = errors.New("trigger not found")
