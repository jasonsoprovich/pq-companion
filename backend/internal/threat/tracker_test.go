package threat

import (
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

// fakeSpells is an in-memory SpellSource for tests.
type fakeSpells map[string]*db.Spell

func (f fakeSpells) GetSpellByExactName(name string) (*db.Spell, error) {
	return f[name], nil
}

// spellWithInstantHate builds a minimal spell carrying a single SE_InstantHate
// effect of the given value.
func spellWithInstantHate(name string, hate int) *db.Spell {
	sp := &db.Spell{Name: name}
	sp.EffectIDs[0] = spaInstantHate
	sp.EffectBaseValues[0] = hate
	return sp
}

func hit(target string, dmg int, ts time.Time) logparser.LogEvent {
	return logparser.LogEvent{
		Type:      logparser.EventCombatHit,
		Timestamp: ts,
		Data:      logparser.CombatHitData{Actor: "You", Target: target, Damage: dmg},
	}
}

func cast(spell string, ts time.Time) logparser.LogEvent {
	return logparser.LogEvent{
		Type:      logparser.EventSpellCast,
		Timestamp: ts,
		Data:      logparser.SpellCastData{SpellName: spell},
	}
}

// hateFor returns the tracked hate for a mob, or -1 if untracked.
func hateFor(s ThreatState, mob string) int64 {
	for _, m := range s.Mobs {
		if m.Name == mob {
			return m.Hate
		}
	}
	return -1
}

func TestDamageAccumulatesPerMob(t *testing.T) {
	tr := NewTracker(nil, nil, nil)
	t0 := time.Now()
	tr.Handle(hit("a gnoll", 100, t0))
	tr.Handle(hit("a gnoll", 50, t0.Add(time.Second)))
	tr.Handle(hit("an orc", 30, t0.Add(2*time.Second)))

	s := tr.GetState()
	if got := hateFor(s, "a gnoll"); got != 150 {
		t.Errorf("a gnoll hate = %d, want 150", got)
	}
	if got := hateFor(s, "an orc"); got != 30 {
		t.Errorf("an orc hate = %d, want 30", got)
	}
	if !s.InCombat {
		t.Error("InCombat = false, want true")
	}
}

func TestIncomingAndThirdPartyDamageIgnored(t *testing.T) {
	tr := NewTracker(nil, nil, nil)
	t0 := time.Now()
	// NPC hitting you — must not generate player threat.
	tr.Handle(logparser.LogEvent{
		Type:      logparser.EventCombatHit,
		Timestamp: t0,
		Data:      logparser.CombatHitData{Actor: "a gnoll", Target: "You", Damage: 80},
	})
	// Another player's hit.
	tr.Handle(logparser.LogEvent{
		Type:      logparser.EventCombatHit,
		Timestamp: t0,
		Data:      logparser.CombatHitData{Actor: "Someone", Target: "a gnoll", Damage: 80},
	})
	if s := tr.GetState(); s.InCombat {
		t.Errorf("InCombat = true, want false (no You-sourced damage)")
	}
}

func TestKillClearsOneMob(t *testing.T) {
	tr := NewTracker(nil, nil, nil)
	t0 := time.Now()
	tr.Handle(hit("a gnoll", 100, t0))
	tr.Handle(hit("an orc", 40, t0))
	tr.Handle(logparser.LogEvent{
		Type:      logparser.EventKill,
		Timestamp: t0.Add(time.Second),
		Data:      logparser.KillData{Killer: "You", Target: "a gnoll"},
	})
	s := tr.GetState()
	if got := hateFor(s, "a gnoll"); got != -1 {
		t.Errorf("a gnoll still tracked after kill (hate=%d)", got)
	}
	if got := hateFor(s, "an orc"); got != 40 {
		t.Errorf("an orc hate = %d, want 40 (unaffected by other kill)", got)
	}
}

func TestZoneAndDeathClearAll(t *testing.T) {
	for _, tc := range []struct {
		name string
		ev   logparser.LogEvent
	}{
		{"zone", logparser.LogEvent{Type: logparser.EventZone, Data: logparser.ZoneData{ZoneName: "somewhere"}}},
		{"death", logparser.LogEvent{Type: logparser.EventDeath, Data: logparser.DeathData{SlainBy: "a gnoll"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tr := NewTracker(nil, nil, nil)
			t0 := time.Now()
			tr.Handle(hit("a gnoll", 100, t0))
			tr.Handle(hit("an orc", 40, t0))
			ev := tc.ev
			ev.Timestamp = t0.Add(time.Second)
			tr.Handle(ev)
			if s := tr.GetState(); s.InCombat {
				t.Errorf("InCombat = true after %s, want all cleared", tc.name)
			}
		})
	}
}

func TestInstantHateSpellAddsToCurrentTarget(t *testing.T) {
	spells := fakeSpells{
		"Terror of Terris": spellWithInstantHate("Terror of Terris", 510),
		"Jolt":             spellWithInstantHate("Jolt", -500),
	}
	tr := NewTracker(nil, NewCalculator(spells), nil)
	t0 := time.Now()

	// Engage so there's a current (last-engaged) mob to attribute the cast to.
	tr.Handle(hit("a gnoll", 100, t0))
	tr.Handle(cast("Terror of Terris", t0.Add(time.Second)))
	if got := hateFor(tr.GetState(), "a gnoll"); got != 610 {
		t.Errorf("hate after Terror = %d, want 610 (100 + 510)", got)
	}

	// Jolt sheds aggro: total drops by 500 → 110.
	tr.Handle(cast("Jolt", t0.Add(2*time.Second)))
	if got := hateFor(tr.GetState(), "a gnoll"); got != 110 {
		t.Errorf("hate after Jolt = %d, want 110 (610 - 500)", got)
	}
}

func TestAggroShedFlooredAtZero(t *testing.T) {
	spells := fakeSpells{"Jolt": spellWithInstantHate("Jolt", -500)}
	tr := NewTracker(nil, NewCalculator(spells), nil)
	t0 := time.Now()
	tr.Handle(hit("a gnoll", 100, t0))
	tr.Handle(cast("Jolt", t0.Add(time.Second))) // raw total -400
	if got := hateFor(tr.GetState(), "a gnoll"); got != 0 {
		t.Errorf("displayed hate = %d, want 0 (negative raw total floored)", got)
	}
}

func TestCastWithoutTargetIgnored(t *testing.T) {
	spells := fakeSpells{"Terror of Terris": spellWithInstantHate("Terror of Terris", 510)}
	tr := NewTracker(nil, NewCalculator(spells), nil)
	// No prior engagement and no pipe target → nothing to attribute to.
	tr.Handle(cast("Terror of Terris", time.Now()))
	if s := tr.GetState(); s.InCombat {
		t.Error("InCombat = true, want false (cast with no target dropped)")
	}
}

func TestHatemodScalesDamage(t *testing.T) {
	tr := NewTracker(nil, nil, func() int { return 50 })
	tr.Handle(hit("a gnoll", 100, time.Now()))
	if got := hateFor(tr.GetState(), "a gnoll"); got != 150 {
		t.Errorf("hate with +50%% hatemod = %d, want 150", got)
	}
}

func TestPipeTargetDrivesHighlight(t *testing.T) {
	tr := NewTracker(nil, nil, nil)
	t0 := time.Now()
	tr.Handle(hit("a gnoll", 100, t0))
	tr.Handle(hit("an orc", 30, t0.Add(time.Second)))
	// Without a pipe target the highlight is the most recently engaged (an orc).
	if s := tr.GetState(); s.Target == nil || s.Target.Name != "an orc" {
		t.Fatalf("default highlight = %v, want an orc", s.Target)
	}
	// Selecting the gnoll via the pipe re-points the highlight even though it
	// has less hate.
	tr.SetPipeTarget("a gnoll")
	s := tr.GetState()
	if s.Target == nil || s.Target.Name != "a gnoll" {
		t.Fatalf("highlight after SetPipeTarget = %v, want a gnoll", s.Target)
	}
	if !s.Target.IsTarget {
		t.Error("Target.IsTarget = false, want true")
	}
}
