// Package itemdisplay tracks the item the player last opened an in-game item
// display window for, fed by the proposed Zeal item pipe message (see
// zealpipe.DecodeItemDisplay), and drives the Item Info overlay.
//
// The Zeal message is not in any released Zeal build yet, so the overlay is
// hidden until this tracker sees a real event: the first one sets
// Preferences.ZealItemDisplaySeen, which the frontend reads to reveal the
// overlay. Users on a Zeal without the message never see the feature.
package itemdisplay

import (
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// EventType is the WebSocket event broadcast for each item window opened.
const EventType = "item:display"

// Shown is one item display event as sent to the frontend.
type Shown struct {
	ItemID    int       `json:"item_id"`
	Name      string    `json:"name"`
	Character string    `json:"character,omitempty"`
	At        time.Time `json:"at"`
	// Simulated marks events from the developer simulate endpoint, which
	// don't count as proof that the installed Zeal sends the message.
	Simulated bool `json:"simulated,omitempty"`
}

// Tracker holds the most recent item and broadcasts each new one.
type Tracker struct {
	hub    *ws.Hub
	cfgMgr *config.Manager

	mu   sync.Mutex
	last *Shown
}

// NewTracker returns a Tracker that broadcasts on hub and records support in
// cfgMgr.
func NewTracker(hub *ws.Hub, cfgMgr *config.Manager) *Tracker {
	return &Tracker{hub: hub, cfgMgr: cfgMgr}
}

// Handle records an item display event from the Zeal pipe.
func (t *Tracker) Handle(itemID int, name, character string) {
	t.record(Shown{ItemID: itemID, Name: name, Character: character, At: time.Now()})
	t.markSupported()
}

// Simulate records a fake item display event, for building and testing the
// overlay without a Zeal build that sends the message.
func (t *Tracker) Simulate(itemID int, name, character string) Shown {
	s := Shown{ItemID: itemID, Name: name, Character: character, At: time.Now(), Simulated: true}
	t.record(s)
	return s
}

// Last returns the most recent item, or nil before the first event.
func (t *Tracker) Last() *Shown {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.last == nil {
		return nil
	}
	s := *t.last
	return &s
}

func (t *Tracker) record(s Shown) {
	s.Name = strings.TrimSpace(s.Name)
	t.mu.Lock()
	t.last = &s
	t.mu.Unlock()
	t.hub.Broadcast(ws.Event{Type: EventType, Data: s})
}

// markSupported persists the "Zeal sends item display events" flag the first
// time a real event arrives. Checked on every event rather than once per run,
// so a Settings save that wrote back a stale copy of the config gets corrected
// by the next item window.
func (t *Tracker) markSupported() {
	if t.cfgMgr.Get().Preferences.ZealItemDisplaySeen {
		return
	}
	if err := t.cfgMgr.Modify(func(c *config.Config) {
		c.Preferences.ZealItemDisplaySeen = true
	}); err != nil {
		slog.Warn("itemdisplay: persist zeal support flag", "err", err)
		return
	}
	slog.Info("itemdisplay: Zeal item display events detected, enabling Item Info overlay")
	t.hub.Broadcast(ws.Event{Type: "config:updated", Data: t.cfgMgr.Get()})
}
