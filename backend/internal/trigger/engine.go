package trigger

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"sync"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

const historyMaxSize = 200

// TimerSink is the minimal interface the trigger engine needs to start and
// stop externally-driven spell timers. It is implemented by
// *spelltimer.Engine; kept abstract here to avoid an import cycle.
//
// displayThresholdSecs is forwarded to the engine so a per-trigger
// threshold (Trigger.DisplayThresholdSecs) can override the global
// buff/detrim defaults the user sets in Settings. 0 means "use the
// category default."
//
// alerts is the per-trigger fading-soon configuration (Trigger.TimerAlerts)
// pre-marshalled to JSON. Sink stores it opaquely on the active timer and
// re-emits it on the WS payload; the frontend parses and acts on it.
type TimerSink interface {
	StartExternal(name, category string, durationSecs, displayThresholdSecs int, startedAt time.Time, alerts json.RawMessage)
	StopExternal(name string)
}

// compiled pairs a Trigger with its pre-compiled patterns for efficient matching.
type compiled struct {
	trigger  *Trigger
	re       *regexp.Regexp
	wornOff  *regexp.Regexp // non-nil only when the trigger has a worn-off pattern
	timerKey string         // cached spelltimer key when timer_type != none
}

// Engine loads triggers from the store and tests every incoming log line
// against them, firing actions and broadcasting events on match.
type Engine struct {
	store      *Store
	hub        *ws.Hub
	sink       TimerSink
	activeChar func() string // returns active character name, "" if unknown

	mu       sync.RWMutex
	compiled []compiled

	histMu  sync.Mutex
	history []TriggerFired // ring buffer, newest appended last
}

// NewEngine creates an Engine backed by store. Call Reload before routing
// lines. sink may be nil when timer integration is disabled (e.g. in tests).
// activeChar returns the currently active character name; nil disables
// per-character filtering (used by tests).
func NewEngine(store *Store, hub *ws.Hub, sink TimerSink, activeChar func() string) *Engine {
	return &Engine{store: store, hub: hub, sink: sink, activeChar: activeChar}
}

// Reload re-reads all enabled triggers from the store and recompiles their
// patterns. Must be called after any CRUD mutation to keep the engine in sync.
func (e *Engine) Reload() {
	triggers, err := e.store.List()
	if err != nil {
		slog.Error("trigger: reload failed", "err", err)
		return
	}

	var cs []compiled
	for _, t := range triggers {
		if !t.Enabled {
			continue
		}
		re, err := regexp.Compile(t.Pattern)
		if err != nil {
			slog.Warn("trigger: invalid pattern, skipping", "id", t.ID, "name", t.Name, "err", err)
			continue
		}
		c := compiled{trigger: t, re: re}
		if t.WornOffPattern != "" {
			if wornRe, err := regexp.Compile(t.WornOffPattern); err == nil {
				c.wornOff = wornRe
			} else {
				slog.Warn("trigger: invalid worn-off pattern", "id", t.ID, "name", t.Name, "err", err)
			}
		}
		if t.TimerType == TimerTypeBuff || t.TimerType == TimerTypeDetrimental {
			c.timerKey = timerKeyFor(t)
		}
		cs = append(cs, c)
	}

	e.mu.Lock()
	e.compiled = cs
	e.mu.Unlock()

	slog.Info("trigger: reloaded", "active", len(cs))
}

// Handle tests a raw log line message against all enabled triggers.
// timestamp is when the line was logged; message is the text after the EQ
// timestamp prefix (i.e. the bare log message, without brackets).
func (e *Engine) Handle(timestamp time.Time, message string) {
	e.mu.RLock()
	cs := e.compiled
	e.mu.RUnlock()

	active := ""
	if e.activeChar != nil {
		active = e.activeChar()
	}

	for _, c := range cs {
		if !triggerAppliesTo(c.trigger, active) {
			continue
		}
		if c.re.MatchString(message) {
			e.fire(c, message, timestamp)
		}
		if c.wornOff != nil && c.wornOff.MatchString(message) {
			if e.sink != nil && c.timerKey != "" {
				e.sink.StopExternal(c.timerKey)
			}
		}
	}
}

// triggerAppliesTo reports whether the trigger should fire for the given
// active character. Empty Characters list = applies to any character (legacy
// safety fallback). Empty active = trigger fires regardless (no character
// detected yet — preserves boot-time behavior).
func triggerAppliesTo(t *Trigger, active string) bool {
	if len(t.Characters) == 0 || active == "" {
		return true
	}
	for _, name := range t.Characters {
		if name == active {
			return true
		}
	}
	return false
}

// GetHistory returns a copy of the recent trigger firing history, newest last.
func (e *Engine) GetHistory() []TriggerFired {
	e.histMu.Lock()
	defer e.histMu.Unlock()
	result := make([]TriggerFired, len(e.history))
	copy(result, e.history)
	return result
}

// ── internal ─────────────────────────────────────────────────────────────────

func (e *Engine) fire(c compiled, matchedLine string, firedAt time.Time) {
	t := c.trigger
	event := TriggerFired{
		TriggerID:   t.ID,
		TriggerName: t.Name,
		MatchedLine: matchedLine,
		Actions:     t.Actions,
		FiredAt:     firedAt,
	}

	e.histMu.Lock()
	e.history = append(e.history, event)
	if len(e.history) > historyMaxSize {
		e.history = e.history[len(e.history)-historyMaxSize:]
	}
	e.histMu.Unlock()

	e.hub.Broadcast(ws.Event{Type: WSEventTriggerFired, Data: event})
	slog.Debug("trigger fired", "trigger", t.Name, "line", matchedLine)

	if e.sink != nil && c.timerKey != "" && t.TimerDurationSecs > 0 {
		var alertJSON json.RawMessage
		if len(t.TimerAlerts) > 0 {
			if buf, err := json.Marshal(t.TimerAlerts); err == nil {
				alertJSON = buf
			}
		}
		e.sink.StartExternal(c.timerKey, timerCategory(t.TimerType), t.TimerDurationSecs, t.DisplayThresholdSecs, firedAt, alertJSON)
	}
}

// timerKeyFor returns the spelltimer key for a trigger. Prefers the trigger
// name so user-configured timers are stable across edits of the pattern.
func timerKeyFor(t *Trigger) string {
	if t.Name != "" {
		return t.Name
	}
	return t.ID
}

// timerCategory maps a trigger's TimerType onto a spelltimer category string.
// Kept in string form to avoid depending on the spelltimer package here.
func timerCategory(tt TimerType) string {
	switch tt {
	case TimerTypeBuff:
		return "buff"
	case TimerTypeDetrimental:
		return "debuff"
	}
	return ""
}

// NewID generates a short random hex identifier suitable for trigger IDs.
func NewID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return hex.EncodeToString(b), nil
}
