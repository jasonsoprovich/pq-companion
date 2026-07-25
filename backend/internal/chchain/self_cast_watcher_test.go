package chchain

import (
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

type fakeBroadcaster struct{ events []ws.Event }

func (f *fakeBroadcaster) Broadcast(e ws.Event) { f.events = append(f.events, e) }

func newSelfCastWatcher(b Broadcaster, enabled bool) *SelfCastWatcher {
	return NewSelfCastWatcher(b, func() config.CHChainSettings {
		return config.CHChainSettings{Enabled: enabled}
	})
}

func TestSelfCastWatcher_BroadcastsOnKnownHeal(t *testing.T) {
	for _, spell := range []string{"Complete Healing", "Tunare's Renewal", "Karana's Renewal"} {
		b := &fakeBroadcaster{}
		w := newSelfCastWatcher(b, true)

		ts := time.Unix(1, 0)
		w.HandleLine(ts, "You begin casting "+spell+".")

		if len(b.events) != 1 {
			t.Fatalf("%s: events = %v, want 1", spell, b.events)
		}
		if b.events[0].Type != WSEventSelfCast {
			t.Errorf("%s: type = %q, want %q", spell, b.events[0].Type, WSEventSelfCast)
		}
		ev, ok := b.events[0].Data.(SelfCastEvent)
		if !ok {
			t.Fatalf("%s: data = %T, want SelfCastEvent", spell, b.events[0].Data)
		}
		if ev.SpellName != spell || !ev.CastAt.Equal(ts) {
			t.Errorf("%s: got %+v", spell, ev)
		}
	}
}

func TestSelfCastWatcher_IgnoresUnrelatedLines(t *testing.T) {
	b := &fakeBroadcaster{}
	w := newSelfCastWatcher(b, true)

	for _, line := range []string{
		"Soandso begins to cast a spell.",    // bystander line, not self
		"You begin casting Minor Healing.",   // self cast, but not a chain heal
		"You begin casting.",                 // no spell name
		"Krayziefoo is completely healed.",   // unrelated
		"You have finished memorizing Fear.", // unrelated "You" line
	} {
		w.HandleLine(time.Unix(1, 0), line)
	}
	if len(b.events) != 0 {
		t.Errorf("events = %v, want none", b.events)
	}
}

func TestSelfCastWatcher_DisabledWhenChainDisabled(t *testing.T) {
	b := &fakeBroadcaster{}
	w := newSelfCastWatcher(b, false)

	w.HandleLine(time.Unix(1, 0), "You begin casting Complete Healing.")
	if len(b.events) != 0 {
		t.Errorf("events = %v, want none", b.events)
	}
}
