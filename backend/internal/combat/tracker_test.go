package combat

import (
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// newTestTracker returns a Tracker wired to a running ws.Hub suitable for tests.
func newTestTracker(t *testing.T) *Tracker {
	t.Helper()
	hub := ws.NewHub()
	go hub.Run()
	return NewTracker(hub)
}

func hitEvent(actor, target string, damage int, ts time.Time) logparser.LogEvent {
	return logparser.LogEvent{
		Type:      logparser.EventCombatHit,
		Timestamp: ts,
		Data: logparser.CombatHitData{
			Actor:  actor,
			Skill:  "slash",
			Target: target,
			Damage: damage,
		},
	}
}

func zoneEvent(ts time.Time) logparser.LogEvent {
	return logparser.LogEvent{
		Type:      logparser.EventZone,
		Timestamp: ts,
		Data:      logparser.ZoneData{ZoneName: "East Commonlands"},
	}
}

func TestNoFightInitially(t *testing.T) {
	tr := newTestTracker(t)
	st := tr.GetState()
	if st.InCombat {
		t.Fatal("expected InCombat=false before any events")
	}
	if st.CurrentFight != nil {
		t.Fatal("expected no CurrentFight before any events")
	}
	if st.SessionDamage != 0 {
		t.Fatalf("expected SessionDamage=0, got %d", st.SessionDamage)
	}
}

func TestFightStartsOnFirstHit(t *testing.T) {
	tr := newTestTracker(t)
	now := time.Now()

	tr.Handle(hitEvent("You", "a gnoll", 100, now))

	st := tr.GetState()
	if !st.InCombat {
		t.Fatal("expected InCombat=true after hit")
	}
	if st.CurrentFight == nil {
		t.Fatal("expected CurrentFight to be set")
	}
	if st.CurrentFight.TotalDamage != 100 {
		t.Fatalf("expected TotalDamage=100, got %d", st.CurrentFight.TotalDamage)
	}
}

func TestMultipleHitsAccumulate(t *testing.T) {
	tr := newTestTracker(t)
	now := time.Now()

	tr.Handle(hitEvent("You", "a gnoll", 100, now))
	tr.Handle(hitEvent("You", "a gnoll", 200, now.Add(time.Second)))
	tr.Handle(hitEvent("You", "a gnoll", 50, now.Add(2*time.Second)))

	st := tr.GetState()
	if st.CurrentFight.TotalDamage != 350 {
		t.Fatalf("expected TotalDamage=350, got %d", st.CurrentFight.TotalDamage)
	}

	// Find the "You" entity.
	var playerStats *EntityStats
	for i := range st.CurrentFight.Combatants {
		if st.CurrentFight.Combatants[i].Name == "You" {
			playerStats = &st.CurrentFight.Combatants[i]
			break
		}
	}
	if playerStats == nil {
		t.Fatal("expected 'You' in Combatants")
	}
	if playerStats.HitCount != 3 {
		t.Fatalf("expected HitCount=3, got %d", playerStats.HitCount)
	}
	if playerStats.MaxHit != 200 {
		t.Fatalf("expected MaxHit=200, got %d", playerStats.MaxHit)
	}
}

func TestIncomingDamageTracked(t *testing.T) {
	tr := newTestTracker(t)
	now := time.Now()

	tr.Handle(hitEvent("You", "a gnoll", 200, now))
	tr.Handle(hitEvent("a gnoll", "You", 30, now.Add(time.Second)))

	st := tr.GetState()
	// TotalDamage is outgoing-only; incoming (NPC hitting player) is excluded.
	if st.CurrentFight.TotalDamage != 200 {
		t.Fatalf("expected TotalDamage=200 (outgoing), got %d", st.CurrentFight.TotalDamage)
	}
	if st.CurrentFight.YouDamage != 200 {
		t.Fatalf("expected YouDamage=200, got %d", st.CurrentFight.YouDamage)
	}
	// Combatants only includes outgoing damage dealers.
	if len(st.CurrentFight.Combatants) != 1 {
		t.Fatalf("expected 1 outgoing combatant (You), got %d", len(st.CurrentFight.Combatants))
	}
	if st.CurrentFight.Combatants[0].Name != "You" {
		t.Fatalf("expected combatant 'You', got %q", st.CurrentFight.Combatants[0].Name)
	}
}

func TestZoneForcesEndOfFight(t *testing.T) {
	tr := newTestTracker(t)
	now := time.Now()

	tr.Handle(hitEvent("You", "a gnoll", 100, now))
	tr.Handle(zoneEvent(now.Add(time.Second)))

	st := tr.GetState()
	if st.InCombat {
		t.Fatal("expected InCombat=false after zone change")
	}
	if len(st.RecentFights) != 1 {
		t.Fatalf("expected 1 recent fight, got %d", len(st.RecentFights))
	}
	if st.RecentFights[0].TotalDamage != 100 {
		t.Fatalf("expected recent fight TotalDamage=100, got %d", st.RecentFights[0].TotalDamage)
	}
}

func TestSessionAggregatesAccumulate(t *testing.T) {
	tr := newTestTracker(t)
	now := time.Now()

	// First fight.
	tr.Handle(hitEvent("You", "a gnoll", 300, now))
	tr.Handle(zoneEvent(now.Add(time.Second)))

	// Second fight.
	tr.Handle(hitEvent("You", "a skeleton", 200, now.Add(10*time.Second)))
	tr.Handle(zoneEvent(now.Add(11*time.Second)))

	st := tr.GetState()
	if st.SessionDamage != 500 {
		t.Fatalf("expected SessionDamage=500, got %d", st.SessionDamage)
	}
	if len(st.RecentFights) != 2 {
		t.Fatalf("expected 2 recent fights, got %d", len(st.RecentFights))
	}
}

func TestCombatantsSortedByDamageDescending(t *testing.T) {
	tr := newTestTracker(t)
	now := time.Now()

	// Two outgoing damage dealers — the higher-damage one should rank first.
	tr.Handle(hitEvent("You", "a gnoll", 50, now))
	tr.Handle(hitEvent("Guildmate", "a gnoll", 200, now.Add(time.Second)))

	st := tr.GetState()
	if len(st.CurrentFight.Combatants) < 2 {
		t.Fatal("expected at least 2 combatants")
	}
	if st.CurrentFight.Combatants[0].TotalDamage < st.CurrentFight.Combatants[1].TotalDamage {
		t.Fatal("expected combatants sorted descending by total damage")
	}
	// TotalDamage is the sum of all outgoing combatants.
	if st.CurrentFight.TotalDamage != 250 {
		t.Fatalf("expected TotalDamage=250, got %d", st.CurrentFight.TotalDamage)
	}
}

func TestThirdPartyDamageTracked(t *testing.T) {
	tr := newTestTracker(t)
	now := time.Now()

	tr.Handle(hitEvent("You", "a gnoll", 100, now))
	tr.Handle(hitEvent("Guildmate", "a gnoll", 150, now.Add(time.Second)))
	tr.Handle(hitEvent("a gnoll", "You", 40, now.Add(2*time.Second)))

	st := tr.GetState()
	// TotalDamage = all outgoing = 100 + 150
	if st.CurrentFight.TotalDamage != 250 {
		t.Fatalf("expected TotalDamage=250, got %d", st.CurrentFight.TotalDamage)
	}
	// YouDamage = only the player's outgoing
	if st.CurrentFight.YouDamage != 100 {
		t.Fatalf("expected YouDamage=100, got %d", st.CurrentFight.YouDamage)
	}
	// 2 outgoing combatants (You + Guildmate), NPC excluded
	if len(st.CurrentFight.Combatants) != 2 {
		t.Fatalf("expected 2 outgoing combatants, got %d", len(st.CurrentFight.Combatants))
	}
}
