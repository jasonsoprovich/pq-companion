package regen

import (
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

const period = 6 * time.Second

var anchor = time.Date(2026, 11, 3, 20, 0, 0, 0, time.UTC)

func fixedPhase(now time.Time) (time.Time, bool) {
	d := now.Sub(anchor) % period
	if d == 0 {
		return now, true
	}
	return now.Add(period - d), true
}

// sim plays readings every 500 ms over ticks ticks. At each tick boundary HP
// and mana gain hpRegen/manaRegen; hook may change the vitals or tracker
// mid-tick.
type sim struct {
	t                  *Tracker
	now                time.Time
	hp, maxHP          int
	mana, maxMana      int
	hpRegen, manaRegen int
}

func newSim() *sim {
	return &sim{
		t:   NewTracker(nil, fixedPhase, period),
		now: anchor.Add(100 * time.Millisecond),
		hp:  500, maxHP: 2000, mana: 300, maxMana: 1500,
		hpRegen: 12, manaRegen: 20,
	}
}

func (s *sim) run(ticks int, hook func(tick int, at time.Time)) {
	end := s.now.Add(time.Duration(ticks) * period)
	tick := 0
	for s.now.Before(end) {
		prev := s.now
		s.now = s.now.Add(500 * time.Millisecond)
		if s.now.Sub(anchor)/period != prev.Sub(anchor)/period {
			tick++
			s.hp = min(s.hp+s.hpRegen, s.maxHP)
			s.mana = min(s.mana+s.manaRegen, s.maxMana)
		}
		if hook != nil {
			hook(tick, s.now)
		}
		s.t.setVitalsAt("Osui", s.hp, s.maxHP, s.mana, s.maxMana, s.now)
	}
}

func TestRegenMeasuresCleanTicks(t *testing.T) {
	s := newSim()
	s.run(8, nil)
	snap := s.t.Snapshot("osui")
	if snap.HPPerTick == nil || *snap.HPPerTick != 12 || snap.ManaPerTick == nil || *snap.ManaPerTick != 20 {
		t.Fatalf("snapshot = %+v", snap)
	}
	if snap.HPTicks != medianTicks {
		t.Fatalf("hp ticks = %d", snap.HPTicks)
	}
}

func TestRegenSkipsDirtyTicks(t *testing.T) {
	s := newSim()
	hit := logparser.LogEvent{Type: logparser.EventCombatHit,
		Data: logparser.CombatHitData{Actor: "a gnoll", Target: "You", Damage: 40}}
	other := logparser.LogEvent{Type: logparser.EventCombatHit,
		Data: logparser.CombatHitData{Actor: "Bob", Target: "a gnoll", Damage: 40}}
	hits := 0
	s.run(12, func(tick int, at time.Time) {
		// Every other tick, take a hit mid-tick (and a heal that hides it,
		// so the vitals alone look like a big regen tick).
		if off := at.Sub(anchor) % period; tick%2 == 1 && off >= 3*time.Second && off < 3500*time.Millisecond {
			hits++
			s.t.handleAt(hit, at)
			s.hp += 200
		}
		s.t.handleAt(other, at) // other people's fights don't count
	})
	if hits < 4 {
		t.Fatalf("only %d hits", hits)
	}
	snap := s.t.Snapshot("Osui")
	if snap.HPPerTick == nil || *snap.HPPerTick != 12 {
		t.Fatalf("snapshot = %+v", snap)
	}
}

func TestRegenSkipsFullPoolsAndMaxChanges(t *testing.T) {
	s := newSim()
	s.hp, s.mana = 2000, 1500 // both full
	s.run(6, nil)
	if snap := s.t.Snapshot("Osui"); snap.HPTicks != 0 || snap.ManaTicks != 0 {
		t.Fatalf("full pools measured: %+v", snap)
	}

	s = newSim()
	s.run(10, func(_ int, _ time.Time) { s.maxHP += 1 }) // max HP keeps moving
	if snap := s.t.Snapshot("Osui"); snap.HPTicks != 0 || snap.ManaPerTick == nil {
		t.Fatalf("max change measured: %+v", snap)
	}
}

func TestRegenNeedsTickPhase(t *testing.T) {
	tr := NewTracker(nil, func(time.Time) (time.Time, bool) { return time.Time{}, false }, period)
	now := anchor
	for i := 0; i < 100; i++ {
		now = now.Add(500 * time.Millisecond)
		tr.setVitalsAt("Osui", 500+i, 2000, 0, 0, now)
	}
	if snap := tr.Snapshot("Osui"); snap.HPTicks != 0 {
		t.Fatalf("measured without a phase: %+v", snap)
	}
}

func TestMedian(t *testing.T) {
	for _, tc := range []struct {
		in   []int
		want int
	}{{[]int{5}, 5}, {[]int{4, 6}, 5}, {[]int{9, 1, 5}, 5}, {[]int{1, 2, 3, 40}, 3}} {
		if got := median(tc.in); got != tc.want {
			t.Errorf("median(%v) = %d, want %d", tc.in, got, tc.want)
		}
	}
}
