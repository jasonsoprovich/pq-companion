// Package chatwindow runs Chat Windows: user-made overlay windows that each
// show a filtered slice of the log. A window picks its sources (chat
// channels, and optionally every other log line), keeps lines its keep rules
// match (with a highlight color and an optional pin at the top of the
// window), and drops lines its hide rules match. Keep rules always beat hide
// rules, so a raid leader's callout survives a broad spam filter.
//
// The engine keeps each window's recent lines in memory, refilled from chat
// history on startup, and delivers them to the renderer as batched appends
// tagged with a per-window (epoch, seq). Anything other than an append
// (pin dismissal or expiry, re-filtering after a rule change, replay) bumps
// the window's epoch and sends the whole window again, so every open view
// converges on the same contents.
package chatwindow

import "errors"

// WebSocket events.
const (
	// WSEventState carries the full State: every definition and buffer.
	// Sent on create, update, delete, reorder and replay start/stop.
	WSEventState = "chatwindows:state"
	// WSEventWindow carries one Window in full after a change that isn't an
	// append (pin dismissed or expired, buffer re-filtered or backfilled).
	WSEventWindow = "chatwindows:window"
	// WSEventLines carries batched appends (see LinesEvent).
	WSEventLines = "chatwindows:lines"
)

// Source channel keys. Any other non-empty key is a named chat channel by its
// lowercased base name ("general" for General:1).
const (
	ChannelTell    = "tell"
	ChannelGuild   = "guild"
	ChannelGroup   = "group"
	ChannelRaid    = "raid"
	ChannelSay     = "say"
	ChannelOOC     = "ooc"
	ChannelShout   = "shout"
	ChannelAuction = "auction"
)

// ChannelOther is a Line's channel when it isn't chat.
const ChannelOther = "other"

// Rule match modes.
const (
	MatchSimple = "simple" // plain text with {placeholders} and * wildcards
	MatchRegex  = "regex"  // an RE2 regex
)

// Limits keep a pasted or imported definition from bloating the pipeline.
const (
	MaxWindows      = 20
	MaxRules        = 50 // per list (keep, hide)
	MaxNameLen      = 80
	MaxSpeakers     = 50
	DefaultMaxLines = 300
	LimitMaxLines   = 1000
	MaxPins         = 10
	MaxPinMinutes   = 60
	MinFontScale    = 0.5
	MaxFontScale    = 3.0
	// Tell conversation tabs: how many stay open before the least recently
	// active read one is closed.
	DefaultMaxTellTabs = 8
	MinMaxTellTabs     = 2
	LimitMaxTellTabs   = 30
)

// Layout values: one list for every channel, or a tab per channel.
const (
	LayoutCombined = "combined"
	LayoutTabs     = "tabs"
)

// SelfColorNone turns off the separate color for your own lines.
const SelfColorNone = "none"

// Sources picks which log lines a window considers at all.
type Sources struct {
	// Channels are chat channel keys (ChannelGuild, …, or a named channel).
	Channels []string `json:"channels"`
	// AllNamed takes every named channel, including ones not listed.
	AllNamed bool `json:"all_named,omitempty"`
	// Other takes every log line that isn't chat (combat, spells, system).
	Other bool `json:"other,omitempty"`
}

// Rule is one keep or hide rule.
type Rule struct {
	ID      string `json:"id"`
	Name    string `json:"name,omitempty"`
	Enabled bool   `json:"enabled"`
	// Channel, when set, limits the rule to chat lines on that channel and
	// matches the message body instead of the whole line.
	Channel string `json:"channel,omitempty"`
	// Speakers limits the rule to chat lines from these players
	// (case-insensitive). Empty = anyone.
	Speakers []string `json:"speakers,omitempty"`
	// Mode is MatchSimple or MatchRegex. Text is the Simple text or regex.
	// Empty Text matches every line the rule's Channel/Speakers allow.
	Mode string `json:"mode"`
	Kind string `json:"kind,omitempty"` // Simple only: starts | contains | exact
	Text string `json:"text,omitempty"`
	// Keep rules only: highlight color ("" = none) and minutes to pin the
	// line at the top of the window (0 = don't pin).
	Color      string `json:"color,omitempty"`
	PinMinutes int    `json:"pin_minutes,omitempty"`
}

// Appearance is how one window looks. Font scale and opacity live on the
// window, not in the global overlay settings, because each chat window has a
// different job (a big raid-callout window, a small tells window).
type Appearance struct {
	FontScale      float64 `json:"font_scale"`
	Opacity        float64 `json:"opacity"`
	ShowTimestamps bool    `json:"show_timestamps"`
	ShowChannel    bool    `json:"show_channel"`
	// ChannelColors overrides the default text color per channel key.
	ChannelColors map[string]string `json:"channel_colors,omitempty"`
	// SelfColor colors your own lines: "" = the default (white),
	// SelfColorNone = the channel color, or a #rrggbb color.
	SelfColor string `json:"self_color,omitempty"`
	// Layout is LayoutCombined ("" means the same) or LayoutTabs.
	Layout string `json:"layout,omitempty"`
	// TellTabs gives each tell conversation its own tab (tabs layout only),
	// with at most MaxTellTabs open (0 = DefaultMaxTellTabs).
	TellTabs    bool `json:"tell_tabs,omitempty"`
	MaxTellTabs int  `json:"max_tell_tabs,omitempty"`
}

// WindowDef is the user-editable definition of one chat window.
type WindowDef struct {
	ID      string `json:"id,omitempty"` // empty in a share
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	// Characters limits the window to these characters' logs; empty = all.
	// Personal: never part of a shared export.
	Characters []string   `json:"characters,omitempty"`
	Sources    Sources    `json:"sources"`
	Keep       []Rule     `json:"keep"`
	Hide       []Rule     `json:"hide"`
	Appearance Appearance `json:"appearance"`
	MaxLines   int        `json:"max_lines"`
}

// Line is one log line as a window stores it. Chat lines carry their parsed
// parts, and Text is the standard form built from them (see chatText), so
// a live line and the same line rebuilt from chat history look and match the
// same. Other lines keep their raw text.
type Line struct {
	Seq       int64  `json:"seq"`
	TS        int64  `json:"ts"` // unix ms
	Character string `json:"character,omitempty"`
	Channel   string `json:"channel"` // a channel key, or ChannelOther
	Direction string `json:"direction,omitempty"`
	Speaker   string `json:"speaker,omitempty"`
	// Peer is a tell's other player (the recipient of an outgoing tell).
	Peer    string `json:"peer,omitempty"`
	Message string `json:"message,omitempty"`
	Text    string `json:"text"`
	// Color is the highlight color from the keep rule that kept it.
	Color string `json:"color,omitempty"`
}

// Pin keeps a line in the strip at the top of the window until Until. It
// carries its own copy of the line, so a pinned callout outlives the line
// scrolling out of the buffer.
type Pin struct {
	Seq   int64 `json:"seq"`
	Until int64 `json:"until"` // unix ms
	Line  Line  `json:"line"`
}

// WindowState is a window's live contents.
type WindowState struct {
	Epoch int64  `json:"epoch"`
	Seq   int64  `json:"seq"`   // last seq handed out
	Lines []Line `json:"lines"` // oldest first
	Pins  []Pin  `json:"pins"`
	// Replay is true while a Log Feed replay fills the window, so clients
	// can tell replayed lines from live ones.
	Replay bool `json:"replay,omitempty"`
}

// Window is a definition plus its live contents.
type Window struct {
	Def   WindowDef   `json:"def"`
	State WindowState `json:"state"`
}

// State is the payload of GET /api/chat-windows and WSEventState.
type State struct {
	Windows []Window `json:"windows"`
	Now     int64    `json:"now"` // server unix ms, for pin expiry math
}

// LinesBatch is one window's appends in a LinesEvent. Lines are in seq
// order and continue from the client's last seq for that epoch; Pins is the
// window's full pin list after the batch.
type LinesBatch struct {
	ID    string `json:"id"`
	Epoch int64  `json:"epoch"`
	Lines []Line `json:"lines"`
	Pins  []Pin  `json:"pins"`
	// Replay: these lines come from a Log Feed replay (see WindowState).
	Replay bool `json:"replay,omitempty"`
}

// LinesEvent is the WSEventLines payload.
type LinesEvent struct {
	Windows []LinesBatch `json:"windows"`
	Now     int64        `json:"now"`
}

// Verdict values for a tested line.
const (
	VerdictKept     = "kept"     // shown, kept by a keep rule
	VerdictShown    = "shown"    // shown, no rule involved
	VerdictHidden   = "hidden"   // dropped by a hide rule
	VerdictFiltered = "filtered" // not one of the window's sources
)

// TestStep is one tested line and what the window would do with it.
type TestStep struct {
	Line    string `json:"line"`
	Text    string `json:"text"`
	Verdict string `json:"verdict"`
	Rule    string `json:"rule,omitempty"` // the deciding rule's name or id
	Color   string `json:"color,omitempty"`
	Pin     bool   `json:"pin,omitempty"`
}

// TestResult is the outcome of a dry run against an unsaved definition.
type TestResult struct {
	Steps []TestStep `json:"steps"`
	Def   *WindowDef `json:"def,omitempty"`
	Error string     `json:"error,omitempty"`
}

// Suggestion is a rule built from a stored line by "Hide/Keep lines like
// this", with how many of the window's buffered lines it matches.
type Suggestion struct {
	Rule    Rule `json:"rule"`
	Matches int  `json:"matches"`
}

// ErrNotFound is returned for an unknown window or line.
var ErrNotFound = errors.New("chat window not found")
