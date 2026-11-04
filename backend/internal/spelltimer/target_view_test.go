package spelltimer

import (
	"strings"
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
)

func TestNamedTargetToken(t *testing.T) {
	slow := &db.Spell{Name: "Turgur's Insects", TargetType: 5}
	tests := []struct {
		name    string
		target  string // pipe target
		landsOn string
		settle  bool
		want    string
	}{
		{"named mob, settled target", "Lord Bob", "Lord Bob", true, "Lord Bob#id:77"},
		{"named mob, not settled", "Lord Bob", "Lord Bob", false, "Lord Bob"},
		{"named mob, other target", "Sir Jim", "Lord Bob", true, "Lord Bob"},
		{"generic mob never takes the target id", "a gnoll", "a gnoll", true, "a gnoll"},
		{"the-prefixed mob is generic", "the ghost of Bob", "the ghost of Bob", true, "the ghost of Bob"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newTestEngine()
			if tc.settle {
				settlePipeTarget(e, tc.target, 77)
			} else {
				e.HandlePipeTarget(tc.target)
				e.SetPipeTargetID(intPtr(77))
			}
			e.mu.Lock()
			got := e.keyTargetTokenLocked(slow, tc.landsOn, time.Now())
			e.mu.Unlock()
			if got != tc.want {
				t.Fatalf("token = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNamedTargetTokenSkipsBuffs(t *testing.T) {
	e := newTestEngine()
	settlePipeTarget(e, "Soandso", 12)
	e.mu.Lock()
	got := e.keyTargetTokenLocked(&db.Spell{Name: "Haste", GoodEffect: 1, TargetType: 5}, "Soandso", time.Now())
	e.mu.Unlock()
	if got != "Soandso" {
		t.Fatalf("token = %q", got)
	}
}

func TestClearTargetAndSnapshotTargetID(t *testing.T) {
	now := time.Now()
	e := newTestEngine()
	// key carries the spawn id; the displayed target name never does.
	add := func(spell, key string, cat Category) {
		target := key
		if i := strings.Index(key, targetIDKeySep); i >= 0 {
			target = key[:i]
		}
		k := timerKey(spell, key)
		e.timers[k] = &ActiveTimer{ID: k, SpellName: spell, TargetName: target,
			Category: cat, StartsAt: now, ExpiresAt: now.Add(time.Minute)}
	}
	add("Tashanian", "a gnoll#id:5", CategoryDebuff)
	add("Tashanian", "a gnoll#id:6", CategoryDebuff)
	add("Malosini", "a gnoll", CategoryDebuff) // unproven
	add("Snare", "an orc", CategoryDebuff)
	add("Aegolism", "Osui", CategoryBuff)

	ids := map[string]int{}
	for _, tm := range e.snapshot(now).Timers {
		ids[tm.ID] = tm.TargetID
	}
	if ids[timerKey("Tashanian", "a gnoll#id:5")] != 5 || ids[timerKey("Malosini", "a gnoll")] != 0 {
		t.Fatalf("snapshot target ids = %v", ids)
	}

	if n := e.ClearTarget("A Gnoll", 5); n != 2 {
		t.Fatalf("removed %d, want 2 (id 5 + unproven)", n)
	}
	if _, ok := e.timers[timerKey("Tashanian", "a gnoll#id:6")]; !ok {
		t.Fatal("other same-named spawn's timer was cleared")
	}
	if n := e.ClearTarget("a gnoll", 0); n != 1 {
		t.Fatalf("no-id clear removed %d, want 1", n)
	}
	if len(e.timers) != 2 {
		t.Fatalf("left %d timers, want orc snare + buff", len(e.timers))
	}
}
