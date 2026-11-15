package spelltimer

import (
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

// A pipe death removes exactly the tagged instance it names and leaves a
// same-named instance with a different spawn id running.
func TestHandlePipeDeath_RemovesExactSpawnOnly(t *testing.T) {
	e := spawnIDEngine(t)
	dead := slowKeyOn(e, 701)
	alive := slowKeyOn(e, 702)

	e.HandlePipeDeath(701, "a bloodguard")

	if _, ok := e.timers[dead]; ok {
		t.Error("expected #701 removed")
	}
	if _, ok := e.timers[alive]; !ok {
		t.Error("expected #702 kept")
	}
}

// Untagged timers fall back to the name match a log kill uses, except a
// charm timer; target-less detrimentals are not swept.
func TestHandlePipeDeath_UntaggedNameMatch(t *testing.T) {
	e := spawnIDEngine(t)
	now := time.Now()
	add := func(key, target string, cat Category, charm bool) {
		e.timers[key] = &ActiveTimer{
			ID: key, SpellName: key, TargetName: target, Category: cat,
			IsCharm: charm, CastAt: now, StartsAt: now,
			ExpiresAt: now.Add(time.Minute),
		}
	}
	add("Tashanian@Knight of Decay", "Knight of Decay", CategoryDebuff, false)
	add("Charm@Knight of Decay", "Knight of Decay", CategoryDebuff, true)
	add("Orphan@", "", CategoryDebuff, false)
	add("Tashanian@Priest of Decay", "Priest of Decay", CategoryDebuff, false)

	e.HandlePipeDeath(900, "Knight of Decay")

	if _, ok := e.timers["Tashanian@Knight of Decay"]; ok {
		t.Error("expected untagged same-name timer removed")
	}
	for _, k := range []string{"Charm@Knight of Decay", "Orphan@", "Tashanian@Priest of Decay"} {
		if _, ok := e.timers[k]; !ok {
			t.Errorf("expected %q kept", k)
		}
	}
}

// When the pipe reports the death, the log's ambiguous kill line for it
// must not flag the surviving same-named instance MaybeDead — whichever of
// the two arrives first.
func TestHandlePipeDeath_SuppressesMaybeDead(t *testing.T) {
	for _, pipeFirst := range []bool{true, false} {
		e := spawnIDEngine(t)
		slowKeyOn(e, 801)
		alive := slowKeyOn(e, 802)
		kill := func() {
			e.Handle(logparser.LogEvent{
				Type: logparser.EventKill,
				Data: logparser.KillData{Killer: "Groupmate", Target: "a bloodguard"},
			})
		}
		if pipeFirst {
			e.HandlePipeDeath(801, "a bloodguard")
			kill()
		} else {
			kill()
			e.HandlePipeDeath(801, "a bloodguard")
		}
		e.mu.Lock()
		e.resolvePendingKillsLocked(time.Now().Add(killEvidenceGrace + time.Second))
		e.mu.Unlock()

		if e.timers[alive] == nil || e.timers[alive].MaybeDead {
			t.Errorf("pipeFirst=%v: expected #802 kept and not MaybeDead", pipeFirst)
		}
		if len(e.timers) != 1 {
			t.Errorf("pipeFirst=%v: expected 1 timer, got %v", pipeFirst, keysOf(e.timers))
		}
	}
}

// MaybeDead rows sort below live ones even when they'd expire sooner.
func TestSnapshot_MaybeDeadSinks(t *testing.T) {
	e := spawnIDEngine(t)
	now := time.Now()
	e.timers["a"] = &ActiveTimer{ID: "a", SpellName: "a", TargetName: "x",
		Category: CategoryDebuff, MaybeDead: true, CastAt: now, StartsAt: now,
		ExpiresAt: now.Add(10 * time.Second)}
	e.timers["b"] = &ActiveTimer{ID: "b", SpellName: "b", TargetName: "x",
		Category: CategoryDebuff, CastAt: now, StartsAt: now,
		ExpiresAt: now.Add(time.Minute)}
	got := e.snapshot(now).Timers
	if len(got) != 2 || got[0].ID != "b" || got[1].ID != "a" {
		t.Errorf("expected live timer first, got %v", got)
	}
}

// A slow that lands just before its mob dies reaches the engine from the
// log a poll after the pipe's death event. Its timer must not start on the
// dead spawn, where nothing would ever clear it; a same-named live spawn
// still gets its timer.
func TestOnSpellLanded_SkipsSpawnAlreadyDead(t *testing.T) {
	e := spawnIDEngine(t)
	land := func(targetID int) {
		e.SetPipeTargetID(intPtr(targetID))
		e.Handle(logparser.LogEvent{
			Type: logparser.EventSpellCast,
			Data: logparser.SpellCastData{SpellName: shasAdvantageName},
		})
		e.onSpellLanded(time.Now(), logparser.SpellLandedData{
			Kind:       logparser.SpellLandedKindOther,
			SpellName:  shasAdvantageName,
			TargetName: "a bloodguard",
		})
	}
	e.HandlePipeDeath(301, "a bloodguard")
	land(301)
	land(302)
	if len(e.timers) != 1 {
		t.Fatalf("expected only #302's timer, got %v", keysOf(e.timers))
	}
	if _, ok := e.timers[timerKey(shasAdvantageName, "a bloodguard"+targetIDKeySep+"302")]; !ok {
		t.Errorf("expected #302's timer, got %v", keysOf(e.timers))
	}

	// Seen alive again, the id is a new spawn: it takes timers.
	settlePipeTarget(e, "a bloodguard", 301)
	land(301)
	if _, ok := e.timers[timerKey(shasAdvantageName, "a bloodguard"+targetIDKeySep+"301")]; !ok {
		t.Errorf("expected #301's timer after it was seen alive, got %v", keysOf(e.timers))
	}
}

// An untagged detrimental on a named mob the pipe saw die in the same
// second isn't started; one on a generic mob is, since a namesake may be
// the one it landed on.
func TestOnSpellLanded_SkipsNamedMobAlreadyDead(t *testing.T) {
	e := spawnIDEngine(t)
	landed := time.Now().Truncate(time.Second)
	e.HandlePipeDeath(410, "Lord Vyemm")
	e.HandlePipeDeath(411, "a bloodguard")
	for _, target := range []string{"Lord Vyemm", "a bloodguard"} {
		e.onSpellLanded(landed, logparser.SpellLandedData{
			Kind:       logparser.SpellLandedKindOther,
			SpellName:  shasAdvantageName,
			TargetName: target,
		})
	}
	if _, ok := e.timers[timerKey(shasAdvantageName, "Lord Vyemm")]; ok {
		t.Error("expected no timer on the dead named mob")
	}
	if _, ok := e.timers[timerKey(shasAdvantageName, "a bloodguard")]; !ok {
		t.Errorf("expected the generic mob's timer, got %v", keysOf(e.timers))
	}
}

// The cast's target is the one the pipe showed when the cast began, not
// whatever is selected by the time the log's cast line is read: tabbing to
// a same-named mob mid-cast must not tag the slow with the new mob's id.
func TestSpellCast_UsesPipeTargetAtCastStart(t *testing.T) {
	e := spawnIDEngine(t)
	settlePipeTarget(e, "a bloodguard", 501)
	e.SetPipeCasting(shasAdvantageName)
	// Tab to the next bloodguard before the log line arrives.
	e.SetPipeTargetID(intPtr(502))
	e.Handle(logparser.LogEvent{
		Type: logparser.EventSpellCast,
		Data: logparser.SpellCastData{SpellName: shasAdvantageName},
	})
	e.onSpellLanded(time.Now(), logparser.SpellLandedData{
		Kind:       logparser.SpellLandedKindOther,
		SpellName:  shasAdvantageName,
		TargetName: "a bloodguard",
	})
	want := timerKey(shasAdvantageName, "a bloodguard"+targetIDKeySep+"501")
	if _, ok := e.timers[want]; !ok {
		t.Fatalf("expected the slow tagged #501, got %v", keysOf(e.timers))
	}
	e.HandlePipeDeath(501, "a bloodguard")
	if len(e.timers) != 0 {
		t.Errorf("expected #501's death to clear it, got %v", keysOf(e.timers))
	}
}
