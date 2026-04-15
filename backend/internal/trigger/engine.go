package trigger

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"regexp"
	"sync"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

const historyMaxSize = 200

// compiled pairs a Trigger with its pre-compiled regexp for efficient matching.
type compiled struct {
	trigger *Trigger
	re      *regexp.Regexp
}

// Engine loads triggers from the store and tests every incoming log line
// against them, firing actions and broadcasting events on match.
type Engine struct {
	store *Store
	hub   *ws.Hub

	mu       sync.RWMutex
	compiled []compiled

	histMu  sync.Mutex
	history []TriggerFired // ring buffer, newest appended last
}

// NewEngine creates an Engine backed by store. Call Reload before routing lines.
func NewEngine(store *Store, hub *ws.Hub) *Engine {
	return &Engine{store: store, hub: hub}
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
		cs = append(cs, compiled{trigger: t, re: re})
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

	for _, c := range cs {
		if c.re.MatchString(message) {
			e.fire(c.trigger, message, timestamp)
		}
	}
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

func (e *Engine) fire(t *Trigger, matchedLine string, firedAt time.Time) {
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
}

// NewID generates a short random hex identifier suitable for trigger IDs.
func NewID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	return hex.EncodeToString(b), nil
}
