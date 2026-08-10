package progress

import (
	"log/slog"
	"sync"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

// Consumer turns level/AA/spell/skill log events into journal rows for the
// active character. Skill-up events are recorded here independently of the
// Skill Tracker (internal/skills) — that package keeps only the latest value
// per skill, which can't answer "how many times did this change in the last
// 30 days."
type Consumer struct {
	store      *Store
	activeChar func() string

	mu            sync.Mutex
	onUpdate      func(Event)
	lastActiveDay map[string]string // character -> last date (YYYY-MM-DD) already marked, to skip redundant writes
}

// NewConsumer constructs a consumer wired to store. activeChar returns the
// current in-game character — these log lines carry no name, so they're
// attributed to whoever's log is being tailed.
func NewConsumer(store *Store, activeChar func() string) *Consumer {
	return &Consumer{store: store, activeChar: activeChar, lastActiveDay: map[string]string{}}
}

// SetOnUpdate registers a callback fired after each newly-recorded event,
// used to broadcast a WebSocket event.
func (c *Consumer) SetOnUpdate(fn func(Event)) {
	c.mu.Lock()
	c.onUpdate = fn
	c.mu.Unlock()
}

// Handle processes one parsed log event, recording it if it's a progression
// milestone type.
func (c *Consumer) Handle(ev logparser.LogEvent) {
	character := ""
	if c.activeChar != nil {
		character = c.activeChar()
	}
	if character == "" {
		return
	}

	out, ok := toEvent(character, ev)
	if !ok {
		return
	}

	inserted, err := c.store.AppendEvent(out)
	if err != nil {
		slog.Warn("progress: append event", "character", character, "kind", out.Kind, "err", err)
		return
	}
	if !inserted {
		return
	}
	c.mu.Lock()
	fn := c.onUpdate
	c.mu.Unlock()
	if fn != nil {
		fn(out)
	}
}

// HandleLine marks the active character's log as having activity on ts's
// calendar day — this is the "days logged into the game" signal behind the
// recap's Active Days stat, independent of whether any progression
// milestone happened that day. Cheap to call on every line: a per-character
// in-memory cache skips the DB write once today's date has already been
// recorded.
func (c *Consumer) HandleLine(ts time.Time, _ string) {
	character := ""
	if c.activeChar != nil {
		character = c.activeChar()
	}
	if character == "" {
		return
	}
	if ts.IsZero() {
		ts = time.Now()
	}
	date := ts.Local().Format("2006-01-02")

	c.mu.Lock()
	already := c.lastActiveDay[character] == date
	if !already {
		c.lastActiveDay[character] = date
	}
	c.mu.Unlock()
	if already {
		return
	}

	if err := c.store.MarkActiveDay(character, ts); err != nil {
		slog.Warn("progress: mark active day", "character", character, "err", err)
	}
}

// toEvent maps a parsed-event-stream LogEvent to a journal Event, or reports
// ok=false for event types this package doesn't track.
func toEvent(character string, ev logparser.LogEvent) (Event, bool) {
	ts := ev.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}
	switch ev.Type {
	case logparser.EventLevelChange:
		d, ok := ev.Data.(logparser.LevelChangeData)
		if !ok {
			return Event{}, false
		}
		return Event{Character: character, At: ts, Kind: KindLevel, Value: d.Level, Delta: d.Delta}, true
	case logparser.EventAAGain:
		d, ok := ev.Data.(logparser.AAGainData)
		if !ok {
			return Event{}, false
		}
		return Event{Character: character, At: ts, Kind: KindAA, Value: d.Points}, true
	case logparser.EventSpellScribed:
		d, ok := ev.Data.(logparser.SpellScribedData)
		if !ok {
			return Event{}, false
		}
		return Event{Character: character, At: ts, Kind: KindSpell, Detail: d.SpellName}, true
	case logparser.EventSkillUp:
		d, ok := ev.Data.(logparser.SkillUpData)
		if !ok {
			return Event{}, false
		}
		return Event{Character: character, At: ts, Kind: KindSkill, Detail: d.SkillName, Value: d.Rank}, true
	default:
		return Event{}, false
	}
}
