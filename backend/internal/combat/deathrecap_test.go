package combat

import (
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

func TestDeathRecap(t *testing.T) {
	tr := newTestTracker(t)
	base := time.Date(2026, 11, 3, 21, 0, 0, 0, time.UTC)
	at := func(s float64) time.Time { return base.Add(time.Duration(s * float64(time.Second))) }
	send := func(s float64, typ logparser.EventType, data any) {
		tr.Handle(logparser.LogEvent{Timestamp: at(s), Type: typ, Data: data})
	}
	ch := logparser.SpellLandedData{Kind: logparser.SpellLandedKindYou, Candidates: []logparser.SpellLandedCandidate{
		{SpellID: 13, SpellName: "Complete Healing"}, {SpellID: 1292, SpellName: "Complete Heal"}}}

	send(0, logparser.EventSpellLanded, ch)                                                                                // outside the window
	send(1, logparser.EventCombatHit, logparser.CombatHitData{Actor: "a gnoll", Target: "You", Skill: "bite", Damage: 50}) // outside
	send(15, logparser.EventSpellLanded, ch)
	send(20, logparser.EventCombatHit, logparser.CombatHitData{Actor: "Lord Vox", Target: "You", Skill: "claw", Damage: 900})
	send(21, logparser.EventCombatMiss, logparser.CombatMissData{Actor: "Lord Vox", Target: "You", MissType: "parry"})
	send(22, logparser.EventHeal, logparser.HealData{Actor: "Cleric", Target: "You", Amount: 400})
	send(22, logparser.EventHeal, logparser.HealData{Actor: "Cleric", Target: "Tank", Amount: 999}) // not on us
	send(23, logparser.EventCombatHit, logparser.CombatHitData{Actor: "Lord Vox", Target: "You", Skill: "spell", SpellName: "Frost Breath", Damage: 1500})
	send(24, logparser.EventDeath, logparser.DeathData{})

	st := tr.GetState()
	if len(st.Deaths) != 1 || st.Deaths[0].Recap == nil {
		t.Fatalf("deaths = %+v", st.Deaths)
	}
	r := st.Deaths[0].Recap
	if r.DamageTaken != 2400 || r.Hits != 2 || r.Misses != 1 || r.BiggestHit != 1500 {
		t.Fatalf("damage = %+v", r)
	}
	if r.HealingTaken != 400 || r.Heals != 1 || r.CHs != 1 {
		t.Fatalf("healing = %+v", r)
	}
	if r.Killer != "Lord Vox" {
		t.Fatalf("killer = %q, want the last attacker", r.Killer)
	}
	if r.SinceLastCH == nil || *r.SinceLastCH != 9 {
		t.Fatalf("since last CH = %v", r.SinceLastCH)
	}
	if len(r.Events) != 5 || r.Events[0].Offset != -9 || r.Events[4].Ability != "Frost Breath" {
		t.Fatalf("events = %+v", r.Events)
	}

	// A second death right after doesn't repeat the first one's events, and
	// a named killer wins over the last attacker.
	send(26, logparser.EventCombatHit, logparser.CombatHitData{Actor: "a gnoll", Target: "You", Skill: "bite", Damage: 10})
	send(27, logparser.EventDeath, logparser.DeathData{SlainBy: "Lord Vox"})
	r2 := tr.GetState().Deaths[1].Recap
	if len(r2.Events) != 1 || r2.Killer != "Lord Vox" || *r2.SinceLastCH != 12 {
		t.Fatalf("second recap = %+v", r2)
	}
}

func TestDeathRecapBufferBounded(t *testing.T) {
	tr := newTestTracker(t)
	now := time.Now()
	for i := 0; i < maxRecapEvents*2; i++ {
		tr.Handle(logparser.LogEvent{Timestamp: now, Type: logparser.EventCombatHit,
			Data: logparser.CombatHitData{Actor: "a gnoll", Target: "You", Skill: "bite", Damage: 1}})
	}
	if len(tr.recapBuf) != maxRecapEvents {
		t.Fatalf("buffer = %d, want %d", len(tr.recapBuf), maxRecapEvents)
	}
}

func TestDeathHistoryRoundTrip(t *testing.T) {
	s := newTestStore(t)
	now := time.Now()
	since := 4.5
	for i, who := range []string{"Osui", "Vortikai"} {
		d := DeathRecord{Timestamp: now.Add(time.Duration(i) * time.Minute), Zone: "Plane of Fear",
			SlainBy: "Cazic Thule", Character: who,
			Recap: &DeathRecap{DamageTaken: 3000, SinceLastCH: &since, Events: []RecapEvent{{Kind: RecapHit, Amount: 3000}}}}
		if _, err := s.SaveDeath(d); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.ListDeaths(DeathFilter{CharacterName: "Osui"})
	if err != nil || len(got) != 1 {
		t.Fatalf("list = %+v, %v", got, err)
	}
	if got[0].Recap == nil || got[0].Recap.DamageTaken != 3000 || *got[0].Recap.SinceLastCH != 4.5 {
		t.Fatalf("recap = %+v", got[0].Recap)
	}
	if n, _ := s.PruneOlderThan(now.Add(30 * time.Second)); n != 1 {
		t.Fatalf("pruned %d", n)
	}
	if n, _ := s.DeleteAllDeaths(); n != 1 {
		t.Fatalf("cleared %d", n)
	}
}
