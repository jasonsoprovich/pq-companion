package skills

import (
	"log/slog"
	"sync"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

// Update is the payload broadcast after a skill row changes, so the Skills tab
// can refresh live.
type Update struct {
	Character string `json:"character"`
	SkillID   int    `json:"skill_id"`
	SkillName string `json:"skill_name"`
	Value     int    `json:"value"`
}

// Consumer turns EventSkillUp events into stored skill rows for the active
// character.
type Consumer struct {
	store      *Store
	activeChar func() string

	mu       sync.Mutex
	onUpdate func(Update)
}

// NewConsumer constructs a consumer wired to store. activeChar returns the
// current in-game character — skill-up lines have no name, so they're
// attributed to whoever's log is being tailed.
func NewConsumer(store *Store, activeChar func() string) *Consumer {
	return &Consumer{store: store, activeChar: activeChar}
}

// SetOnUpdate registers a callback fired after each changed skill row, used to
// broadcast a WebSocket event.
func (c *Consumer) SetOnUpdate(fn func(Update)) {
	c.mu.Lock()
	c.onUpdate = fn
	c.mu.Unlock()
}

// Handle processes EventSkillUp from the parsed-event stream.
func (c *Consumer) Handle(ev logparser.LogEvent) {
	if ev.Type != logparser.EventSkillUp {
		return
	}
	d, ok := ev.Data.(logparser.SkillUpData)
	if !ok {
		return
	}
	character := ""
	if c.activeChar != nil {
		character = c.activeChar()
	}
	if character == "" {
		// No active character resolved yet — can't attribute the skill.
		return
	}

	ts := ev.Timestamp
	if ts.IsZero() {
		ts = time.Now()
	}
	skillID, _ := SkillID(d.SkillName)

	changed, err := c.store.Upsert(character, d.SkillName, skillID, d.Rank, ts)
	if err != nil {
		slog.Warn("skills: upsert failed", "skill", d.SkillName, "err", err)
		return
	}
	if !changed {
		return
	}

	c.mu.Lock()
	onUpdate := c.onUpdate
	c.mu.Unlock()
	if onUpdate != nil {
		onUpdate(Update{Character: character, SkillID: skillID, SkillName: d.SkillName, Value: d.Rank})
	}
}
