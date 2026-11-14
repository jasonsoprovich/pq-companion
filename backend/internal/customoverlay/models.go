// Package customoverlay runs user-built overlays: small windows made of
// widgets (fields, a ranked table, a line feed, a counter, a countdown) whose
// contents are driven by regex rules over log lines. Guilds use them for
// bespoke raid tools (a DKP bid board reading the loot master's guild-chat
// calls, sign-up tallies…) that are too guild-specific to build into the app.
//
// Everything is declarative: an overlay is a JSON definition the app renders
// with its own widgets, so a shared definition can't run code. Patterns are
// Go RE2 only (linear time), so an imported pattern can't stall the log
// pipeline the way a backtracking one could.
package customoverlay

import "errors"

// WSEventState is broadcast with a full State snapshot whenever any custom
// overlay's definition or contents change.
const WSEventState = "customoverlays:state"

// WidgetType identifies how a widget renders and which ops can target it.
type WidgetType string

const (
	WidgetFields    WidgetType = "fields"    // named values, e.g. Item / Status
	WidgetTable     WidgetType = "table"     // rows upserted by a key
	WidgetFeed      WidgetType = "feed"      // the last N formatted lines
	WidgetCounter   WidgetType = "counter"   // one integer
	WidgetCountdown WidgetType = "countdown" // a timer started by a rule
)

// OpType is one thing a matching rule does to the overlay's contents.
type OpType string

const (
	OpSetField       OpType = "set_field"       // Widget, Field, Value
	OpClearField     OpType = "clear_field"     // Widget, Field ("" = every field)
	OpUpsertRow      OpType = "upsert_row"      // Widget, Key, Cells
	OpRemoveRow      OpType = "remove_row"      // Widget, Key
	OpAddLine        OpType = "add_line"        // Widget, Value
	OpCounterAdd     OpType = "counter_add"     // Widget, Amount
	OpCounterSet     OpType = "counter_set"     // Widget, Value (integer text)
	OpCountdownStart OpType = "countdown_start" // Widget, Value (duration text; "" = widget default)
	OpCountdownStop  OpType = "countdown_stop"  // Widget
	OpResetWidget    OpType = "reset_widget"    // Widget
	OpResetAll       OpType = "reset_all"       // —
)

// Table merge modes: what an upsert does when the key already has a row.
const (
	MergeReplace = "replace" // new cells replace the row (default)
	MergeHighest = "highest" // keep the row unless MergeColumn's new value is >= the old
	MergeAdd     = "add"     // MergeColumn becomes old + new; other cells are replaced
)

// Channel helper values for Rule.Channel. Any other non-empty value is
// treated as a named chat channel (lowercased, slot number dropped), e.g.
// "dkp" for "Soandso tells Dkp:3, '…'".
const (
	ChannelAny     = "any" // any chat channel, including tells
	ChannelGuild   = "guild"
	ChannelRaid    = "raid"
	ChannelGroup   = "group"
	ChannelTell    = "tell"
	ChannelSay     = "say"
	ChannelOOC     = "ooc"
	ChannelShout   = "shout"
	ChannelAuction = "auction"
)

// Rule.Direction values. Empty means both.
const (
	DirectionIn  = "in"  // someone else spoke
	DirectionOut = "out" // the local character spoke
)

// Limits keep a pasted or imported definition from bloating the engine.
const (
	maxWidgets      = 20
	maxRules        = 50
	maxOpsPerRule   = 10
	maxFields       = 20
	maxColumns      = 8
	maxPatternLen   = 1000
	maxTemplateLen  = 500
	maxNameLen      = 80
	defaultMaxRows  = 50
	limitMaxRows    = 200
	defaultMaxLines = 20
	limitMaxLines   = 200
	maxCellLen      = 200
)

// ColorRule colors a field's value when it contains Contains
// (case-insensitive), e.g. "SOLD" → green. First match wins.
type ColorRule struct {
	Contains string `json:"contains"`
	Color    string `json:"color"`
}

// Field is one named value in a fields widget.
type Field struct {
	Key    string      `json:"key"`
	Label  string      `json:"label"`
	Colors []ColorRule `json:"colors,omitempty"`
}

// Column is one table column. A table's first column is its key column: an
// upsert's Key fills it.
type Column struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// Widget is one block in the overlay, rendered top to bottom in order.
type Widget struct {
	ID    string     `json:"id"`
	Type  WidgetType `json:"type"`
	Title string     `json:"title,omitempty"`

	// fields
	Fields []Field `json:"fields,omitempty"`

	// table
	Columns     []Column `json:"columns,omitempty"`
	SortBy      string   `json:"sort_by,omitempty"` // column key; "" = arrival order
	SortDesc    bool     `json:"sort_desc,omitempty"`
	Merge       string   `json:"merge,omitempty"`        // replace | highest | add
	MergeColumn string   `json:"merge_column,omitempty"` // column compared/added for highest/add
	MaxRows     int      `json:"max_rows,omitempty"`     // oldest rows drop past this

	// feed
	MaxLines int `json:"max_lines,omitempty"`

	// countdown: duration used when a countdown_start op has no Value.
	Seconds float64 `json:"seconds,omitempty"`
}

// Op is one change a matching rule makes. Text fields are templates: {1},
// $1 and {name} insert capture groups; {speaker}, {channel}, {message} and
// {line} describe the matched line; {c}/{char}/{self} is the active
// character.
type Op struct {
	Type   OpType            `json:"type"`
	Widget string            `json:"widget,omitempty"`
	Field  string            `json:"field,omitempty"`
	Value  string            `json:"value,omitempty"`
	Key    string            `json:"key,omitempty"`
	Cells  map[string]string `json:"cells,omitempty"`
	Amount int               `json:"amount,omitempty"`
}

// Rule matches log lines and applies its ops, in order, on every match.
// Every enabled rule that matches a line runs (not just the first).
type Rule struct {
	ID      string `json:"id"`
	Name    string `json:"name,omitempty"`
	Enabled bool   `json:"enabled"`

	// Channel, when set, only considers chat lines on that channel and runs
	// Pattern against the chat message body rather than the whole line.
	Channel   string   `json:"channel,omitempty"`
	Direction string   `json:"direction,omitempty"` // "" | in | out
	Speakers  []string `json:"speakers,omitempty"`  // case-insensitive; empty = anyone

	// Simple, when set, is the plain-text form of the pattern; Validate
	// builds Pattern from it (see SimpleMatch).
	Simple *SimpleMatch `json:"simple,omitempty"`
	// Pattern is an RE2 regex. Empty is allowed only with a Channel (match
	// every message on it).
	Pattern string `json:"pattern,omitempty"`
	Ops     []Op   `json:"ops"`
}

// OverlayDef is the user-editable definition of one custom overlay.
type OverlayDef struct {
	ID      string `json:"id,omitempty"` // empty in a share
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	// Characters limits the overlay to these characters' logs; empty = all.
	// Personal: never part of a shared export.
	Characters []string `json:"characters,omitempty"`
	Widgets    []Widget `json:"widgets"`
	Rules      []Rule   `json:"rules"`
}

// Row is one table row.
type Row struct {
	Key       string            `json:"key"`
	Cells     map[string]string `json:"cells"`
	Seq       int64             `json:"seq"`        // arrival order (first insert)
	UpdatedAt int64             `json:"updated_at"` // unix ms
}

// FeedLine is one feed entry.
type FeedLine struct {
	Text string `json:"text"`
	TS   int64  `json:"ts"` // unix ms
}

// WidgetState is the live contents of one widget. Only the members for the
// widget's type are used.
type WidgetState struct {
	Fields map[string]string `json:"fields,omitempty"`
	Rows   []Row             `json:"rows,omitempty"`  // sorted for display in snapshots
	Lines  []FeedLine        `json:"lines,omitempty"` // newest first
	Count  int               `json:"count,omitempty"`
	EndsAt int64             `json:"ends_at,omitempty"` // countdown: unix ms; 0 = stopped
}

// OverlayState is the live contents of one overlay, keyed by widget ID.
type OverlayState struct {
	Widgets map[string]*WidgetState `json:"widgets"`
	NextSeq int64                   `json:"next_seq"`
}

// Overlay is a definition plus its live state.
type Overlay struct {
	Def       OverlayDef   `json:"def"`
	State     OverlayState `json:"state"`
	UpdatedAt int64        `json:"updated_at"` // unix ms
}

// State is the payload of GET /api/custom-overlays and the WS event.
type State struct {
	Overlays []Overlay `json:"overlays"`
	Now      int64     `json:"now"` // server unix ms, for countdown math
}

// TestStep is one test line and the IDs of the rules it matched.
type TestStep struct {
	Line    string   `json:"line"`
	Matched []string `json:"matched"`
}

// TestResult is the outcome of a dry run of some lines against a definition.
// Def is the validated definition (generated IDs and Simple-mode patterns
// filled in), so the editor can show what will be saved.
type TestResult struct {
	Steps []TestStep   `json:"steps"`
	State OverlayState `json:"state"`
	Def   *OverlayDef  `json:"def,omitempty"`
	Error string       `json:"error,omitempty"`
}

// ErrNotFound is returned for an unknown overlay ID.
var ErrNotFound = errors.New("custom overlay not found")
