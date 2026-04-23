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

func zoneEventNamed(name string, ts time.Time) logparser.LogEvent {
	return logparser.LogEvent{
		Type:      logparser.EventZone,
		Timestamp: ts,
		Data:      logparser.ZoneData{ZoneName: name},
	}
}

func deathEvent(slainBy string, ts time.Time) logparser.LogEvent {
	return logparser.LogEvent{
		Type:      logparser.EventDeath,
		Timestamp: ts,
		Data:      logparser.DeathData{SlainBy: slainBy},
	}
}

func killEvent(killer, target string, ts time.Time) logparser.LogEvent {
	return logparser.LogEvent{
		Type:      logparser.EventKill,
		Timestamp: ts,
		Data:      logparser.KillData{Killer: killer, Target: target},
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

func TestKillEventEndsFightAtKillTime(t *testing.T) {
	tr := newTestTracker(t)
	start := time.Now()

	tr.Handle(hitEvent("You", "a gnoll", 300, start))
	tr.Handle(hitEvent("You", "a gnoll", 200, start.Add(2*time.Second)))
	killTS := start.Add(3 * time.Second)
	tr.Handle(killEvent("You", "a gnoll", killTS))

	st := tr.GetState()
	if st.InCombat {
		t.Fatal("expected InCombat=false after kill event")
	}
	if len(st.RecentFights) != 1 {
		t.Fatalf("expected 1 recent fight, got %d", len(st.RecentFights))
	}
	f := st.RecentFights[0]
	if f.TotalDamage != 500 {
		t.Fatalf("expected TotalDamage=500, got %d", f.TotalDamage)
	}
	// Duration should be ~3s (start to killTS), well under combatGap (6s).
	if f.Duration < 2.9 || f.Duration > 3.1 {
		t.Fatalf("expected fight duration ~3s, got %.3f", f.Duration)
	}
	if !f.EndTime.Equal(killTS) {
		t.Fatalf("expected EndTime=%v, got %v", killTS, f.EndTime)
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

// TestNPCExcludedWhenAttackingGroupMember verifies that an NPC is excluded from
// the DPS list even when it never attacks "You" directly — only a group member.
// This guards against the bug where NPCs using spells on group members appeared
// as DPS contributors because they were absent from the incoming map.
func TestNPCExcludedWhenAttackingGroupMember(t *testing.T) {
	tr := newTestTracker(t)
	now := time.Now()

	// Players attack "a gnoll" — puts "a gnoll" in targetCounts.
	tr.Handle(hitEvent("You", "a gnoll", 200, now))
	tr.Handle(hitEvent("Guildmate", "a gnoll", 150, now.Add(time.Second)))

	// "a gnoll" casts a spell on Guildmate (not on "You") — puts "a gnoll" in
	// outgoing since the target is not "You". Without the fix this would have
	// caused "a gnoll" to appear as a DPS contributor.
	tr.Handle(hitEvent("a gnoll", "Guildmate", 80, now.Add(2*time.Second)))

	st := tr.GetState()
	// Only "You" and "Guildmate" should appear — "a gnoll" must be excluded.
	if len(st.CurrentFight.Combatants) != 2 {
		t.Fatalf("expected 2 combatants (You + Guildmate), got %d", len(st.CurrentFight.Combatants))
	}
	for _, c := range st.CurrentFight.Combatants {
		if c.Name == "a gnoll" {
			t.Fatalf("NPC %q must not appear in the DPS list", c.Name)
		}
	}
}

func TestDeathRecorded(t *testing.T) {
	tr := newTestTracker(t)
	now := time.Now()

	tr.Handle(zoneEventNamed("The North Karana", now))
	tr.Handle(hitEvent("a gnoll", "You", 500, now.Add(time.Second)))
	tr.Handle(deathEvent("a gnoll", now.Add(2*time.Second)))

	st := tr.GetState()
	if st.DeathCount != 1 {
		t.Fatalf("expected DeathCount=1, got %d", st.DeathCount)
	}
	if len(st.Deaths) != 1 {
		t.Fatalf("expected 1 death record, got %d", len(st.Deaths))
	}
	d := st.Deaths[0]
	if d.SlainBy != "a gnoll" {
		t.Errorf("expected SlainBy=%q, got %q", "a gnoll", d.SlainBy)
	}
	if d.Zone != "The North Karana" {
		t.Errorf("expected Zone=%q, got %q", "The North Karana", d.Zone)
	}
	if !st.InCombat {
		// death ends the fight
	}
	if st.InCombat {
		t.Fatal("expected InCombat=false after death")
	}
}

func TestDeathWithNoKillerRecorded(t *testing.T) {
	tr := newTestTracker(t)
	now := time.Now()

	tr.Handle(zoneEventNamed("West Commonlands", now))
	tr.Handle(deathEvent("", now.Add(time.Second)))

	st := tr.GetState()
	if st.DeathCount != 1 {
		t.Fatalf("expected DeathCount=1, got %d", st.DeathCount)
	}
	if st.Deaths[0].SlainBy != "" {
		t.Errorf("expected empty SlainBy for anonymous death, got %q", st.Deaths[0].SlainBy)
	}
	if st.Deaths[0].Zone != "West Commonlands" {
		t.Errorf("expected Zone=%q, got %q", "West Commonlands", st.Deaths[0].Zone)
	}
}

func TestMultipleDeathsAccumulate(t *testing.T) {
	tr := newTestTracker(t)
	now := time.Now()

	tr.Handle(zoneEventNamed("Crushbone", now))
	tr.Handle(deathEvent("an orc pawn", now.Add(time.Second)))
	tr.Handle(zoneEventNamed("East Commonlands", now.Add(10*time.Second)))
	tr.Handle(deathEvent("a large snake", now.Add(20*time.Second)))

	st := tr.GetState()
	if st.DeathCount != 2 {
		t.Fatalf("expected DeathCount=2, got %d", st.DeathCount)
	}
	if st.Deaths[0].Zone != "Crushbone" {
		t.Errorf("expected first death in Crushbone, got %q", st.Deaths[0].Zone)
	}
	if st.Deaths[1].Zone != "East Commonlands" {
		t.Errorf("expected second death in East Commonlands, got %q", st.Deaths[1].Zone)
	}
}

// TestActiveFightDurationUsesLogTimestamps verifies that GetState() returns a
// fight duration based on log timestamps, not wall-clock time. This guards
// against the "300+ minute timer" bug where historical log replay caused
// startTime (log-domain) to be compared against time.Now() (wall-clock).
func TestActiveFightDurationUsesLogTimestamps(t *testing.T) {
	tr := newTestTracker(t)

	// Simulate a fight that happened in the past (5 hours ago in log time).
	logStart := time.Now().Add(-5 * time.Hour)
	tr.Handle(hitEvent("You", "a gnoll", 100, logStart))
	tr.Handle(hitEvent("You", "a gnoll", 200, logStart.Add(10*time.Second)))

	st := tr.GetState()
	if !st.InCombat {
		t.Fatal("expected InCombat=true")
	}
	// Duration must be ~10s (log time between first and last hit), not ~5h.
	if st.CurrentFight.Duration > 60 {
		t.Fatalf("expected fight duration ~10s, got %.1fs — likely using wall-clock vs log timestamp", st.CurrentFight.Duration)
	}
}

func TestZoneTrackedForDeath(t *testing.T) {
	tr := newTestTracker(t)
	now := time.Now()

	// Death before any zone event — zone should be empty string.
	tr.Handle(deathEvent("a bat", now))

	st := tr.GetState()
	if st.Deaths[0].Zone != "" {
		t.Errorf("expected empty zone before any zone event, got %q", st.Deaths[0].Zone)
	}
}
