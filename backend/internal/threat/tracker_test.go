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

// fakeNPCs is an in-memory NPCSource keyed by db name (underscores).
type fakeNPCs map[string]int

func (f fakeNPCs) GetNPCByName(name string) (*db.NPC, error) {
	hp, ok := f[name]
	if !ok {
		return nil, nil
	}
	return &db.NPC{Name: name, HP: hp}, nil
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
	tr := NewTracker(nil, NewCalculator(spells, nil), nil)
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
	tr := NewTracker(nil, NewCalculator(spells, nil), nil)
	t0 := time.Now()
	tr.Handle(hit("a gnoll", 100, t0))
	tr.Handle(cast("Jolt", t0.Add(time.Second))) // raw total -400
	if got := hateFor(tr.GetState(), "a gnoll"); got != 0 {
		t.Errorf("displayed hate = %d, want 0 (negative raw total floored)", got)
	}
}

func TestCastWithoutTargetIgnored(t *testing.T) {
	spells := fakeSpells{"Terror of Terris": spellWithInstantHate("Terror of Terris", 510)}
	tr := NewTracker(nil, NewCalculator(spells, nil), nil)
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

// ── Phase 3: standard hate, hate-mod buffs, heal, miss, feign ───────────────

// spellDetrimentalUtility is a no-damage detrimental spell (mez/slow/tash-like):
// GoodEffect 0, one non-damage/non-hate effect slot.
func spellDetrimentalUtility(name string) *db.Spell {
	sp := &db.Spell{Name: name, GoodEffect: 0}
	sp.EffectIDs[0] = 18 // arbitrary non-damage, non-hate effect
	sp.EffectBaseValues[0] = -10
	return sp
}

// spellDamage is a detrimental damage spell (SE_CurrentHP negative).
func spellDamage(name string) *db.Spell {
	sp := &db.Spell{Name: name, GoodEffect: 0}
	sp.EffectIDs[0] = spaCurrentHP
	sp.EffectBaseValues[0] = -200
	return sp
}

// spellHateModBuff is a beneficial self-buff with a SE_ChangeAggro modifier.
func spellHateModBuff(name string, pct, durationTicks int) *db.Spell {
	sp := &db.Spell{Name: name, GoodEffect: 1, BuffDuration: durationTicks}
	sp.EffectIDs[0] = spaChangeAggro
	sp.EffectBaseValues[0] = pct
	return sp
}

func miss(target string, ts time.Time) logparser.LogEvent {
	return logparser.LogEvent{
		Type:      logparser.EventCombatMiss,
		Timestamp: ts,
		Data:      logparser.CombatMissData{Actor: "You", Target: target, MissType: "miss"},
	}
}

func heal(amount int, ts time.Time) logparser.LogEvent {
	return logparser.LogEvent{
		Type:      logparser.EventHeal,
		Timestamp: ts,
		Data:      logparser.HealData{Actor: "You", Target: "You", Amount: amount},
	}
}

func TestStandardHateUtilitySpell(t *testing.T) {
	spells := fakeSpells{"Tashan": spellDetrimentalUtility("Tashan")}
	npcs := fakeNPCs{"a_gnoll": 3000} // HP/15 = 200
	tr := NewTracker(nil, NewCalculator(spells, npcs), nil)
	t0 := time.Now()
	tr.Handle(hit("a gnoll", 100, t0))
	tr.Handle(cast("Tashan", t0.Add(time.Second)))
	if got := hateFor(tr.GetState(), "a gnoll"); got != 300 {
		t.Errorf("hate after Tashan = %d, want 300 (100 + 3000/15)", got)
	}
}

func TestStandardHateFloorAndCap(t *testing.T) {
	spells := fakeSpells{"Debuff": spellDetrimentalUtility("Debuff")}
	t0 := time.Now()

	// HP/15 below the floor clamps up to 25.
	low := NewTracker(nil, NewCalculator(spells, fakeNPCs{"a_rat": 100}), nil)
	low.Handle(hit("a rat", 10, t0))
	low.Handle(cast("Debuff", t0.Add(time.Second)))
	if got := hateFor(low.GetState(), "a rat"); got != 35 {
		t.Errorf("floor case hate = %d, want 35 (10 + floor 25)", got)
	}

	// HP/15 above the cap clamps down to 1200.
	high := NewTracker(nil, NewCalculator(spells, fakeNPCs{"a_dragon": 300000}), nil)
	high.Handle(hit("a dragon", 100, t0))
	high.Handle(cast("Debuff", t0.Add(time.Second)))
	if got := hateFor(high.GetState(), "a dragon"); got != 1300 {
		t.Errorf("cap case hate = %d, want 1300 (100 + cap 1200)", got)
	}
}

func TestStandardHateSkippedWithoutNPCSource(t *testing.T) {
	spells := fakeSpells{"Tashan": spellDetrimentalUtility("Tashan")}
	tr := NewTracker(nil, NewCalculator(spells, nil), nil) // no NPC source
	t0 := time.Now()
	tr.Handle(hit("a gnoll", 100, t0))
	tr.Handle(cast("Tashan", t0.Add(time.Second)))
	if got := hateFor(tr.GetState(), "a gnoll"); got != 100 {
		t.Errorf("hate = %d, want 100 (no HP → no standard hate)", got)
	}
}

func TestDamageSpellCastAddsNoStandardHate(t *testing.T) {
	// A nuke's hate is its observed damage line, not the cast. The cast must add
	// nothing (no double counting) even though the mob HP is known.
	spells := fakeSpells{"Nuke": spellDamage("Nuke")}
	npcs := fakeNPCs{"a_gnoll": 3000}
	tr := NewTracker(nil, NewCalculator(spells, npcs), nil)
	t0 := time.Now()
	tr.Handle(hit("a gnoll", 100, t0))
	tr.Handle(cast("Nuke", t0.Add(time.Second)))
	if got := hateFor(tr.GetState(), "a gnoll"); got != 100 {
		t.Errorf("hate after nuke cast = %d, want 100 (damage counted from its own line only)", got)
	}
}

func TestHateModBuffScalesHate(t *testing.T) {
	spells := fakeSpells{
		"Glamorous Visage": spellHateModBuff("Glamorous Visage", -10, 100),
		"Voice of Terris":  spellHateModBuff("Voice of Terris", 10, 100),
	}
	t0 := time.Now()

	down := NewTracker(nil, NewCalculator(spells, nil), nil)
	down.Handle(cast("Glamorous Visage", t0))
	down.Handle(hit("a gnoll", 100, t0.Add(time.Second)))
	s := down.GetState()
	if got := hateFor(s, "a gnoll"); got != 90 {
		t.Errorf("hate with -10%% buff = %d, want 90", got)
	}
	if s.HatemodPct != -10 {
		t.Errorf("HatemodPct = %d, want -10", s.HatemodPct)
	}

	up := NewTracker(nil, NewCalculator(spells, nil), nil)
	up.Handle(cast("Voice of Terris", t0))
	up.Handle(hit("a gnoll", 100, t0.Add(time.Second)))
	if got := hateFor(up.GetState(), "a gnoll"); got != 110 {
		t.Errorf("hate with +10%% buff = %d, want 110", got)
	}
}

func TestHateModBuffStacksWithStatic(t *testing.T) {
	spells := fakeSpells{"Voice of Terris": spellHateModBuff("Voice of Terris", 10, 100)}
	tr := NewTracker(nil, NewCalculator(spells, nil), func() int { return 20 })
	t0 := time.Now()
	tr.Handle(cast("Voice of Terris", t0))
	tr.Handle(hit("a gnoll", 100, t0.Add(time.Second)))
	// 100 * (100 + 20 + 10) / 100 = 130
	if got := hateFor(tr.GetState(), "a gnoll"); got != 130 {
		t.Errorf("hate with +20 static +10 buff = %d, want 130", got)
	}
}

func TestHateModBuffClearedOnZone(t *testing.T) {
	spells := fakeSpells{"Glamorous Visage": spellHateModBuff("Glamorous Visage", -10, 100)}
	tr := NewTracker(nil, NewCalculator(spells, nil), nil)
	t0 := time.Now()
	tr.Handle(cast("Glamorous Visage", t0))
	tr.Handle(logparser.LogEvent{Type: logparser.EventZone, Timestamp: t0.Add(time.Second), Data: logparser.ZoneData{ZoneName: "x"}})
	tr.Handle(hit("a gnoll", 100, t0.Add(2*time.Second)))
	s := tr.GetState()
	if got := hateFor(s, "a gnoll"); got != 100 {
		t.Errorf("hate after zone cleared buff = %d, want 100", got)
	}
	if s.HatemodPct != 0 {
		t.Errorf("HatemodPct = %d, want 0 after zone", s.HatemodPct)
	}
}

func TestHealHateSpreadsToAllMobs(t *testing.T) {
	tr := NewTracker(nil, nil, nil)
	t0 := time.Now()
	tr.Handle(hit("a gnoll", 100, t0))
	tr.Handle(hit("an orc", 50, t0))
	// HealHate(300) = 1 + 2*300/3 = 201, added to BOTH mobs.
	tr.Handle(heal(300, t0.Add(time.Second)))
	s := tr.GetState()
	if got := hateFor(s, "a gnoll"); got != 301 {
		t.Errorf("a gnoll hate after heal = %d, want 301", got)
	}
	if got := hateFor(s, "an orc"); got != 251 {
		t.Errorf("an orc hate after heal = %d, want 251", got)
	}
}

func TestHealOutOfCombatIgnored(t *testing.T) {
	tr := NewTracker(nil, nil, nil)
	tr.Handle(heal(300, time.Now()))
	if s := tr.GetState(); s.InCombat {
		t.Error("InCombat = true, want false (heal with no hate list)")
	}
}

func TestMeleeMissHate(t *testing.T) {
	tr := NewTracker(nil, nil, nil)
	t0 := time.Now()
	tr.Handle(hit("a gnoll", 100, t0))                 // melee
	tr.Handle(hit("a gnoll", 50, t0.Add(time.Second))) // melee, avg now 75
	tr.Handle(miss("a gnoll", t0.Add(2*time.Second)))  // miss ≈ avg swing 75
	if got := hateFor(tr.GetState(), "a gnoll"); got != 225 {
		t.Errorf("hate after miss = %d, want 225 (100+50+75)", got)
	}
}

func TestMissBeforeAnyHitIgnored(t *testing.T) {
	tr := NewTracker(nil, nil, nil)
	tr.Handle(miss("a gnoll", time.Now()))
	if s := tr.GetState(); s.InCombat {
		t.Error("InCombat = true, want false (miss with no prior swing)")
	}
}

func TestFeignDeathClearsAll(t *testing.T) {
	tr := NewTracker(nil, nil, nil)
	t0 := time.Now()
	tr.Handle(hit("a gnoll", 100, t0))
	tr.Handle(hit("an orc", 40, t0))
	tr.Handle(logparser.LogEvent{Type: logparser.EventFeignDeath, Timestamp: t0.Add(time.Second)})
	if s := tr.GetState(); s.InCombat {
		t.Error("InCombat = true after feign death, want all cleared")
	}
}
