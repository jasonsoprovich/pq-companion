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

// TestPlayerVsPlayerDamageDoesNotSeedFight verifies that pure player-vs-player
// hits with no NPC involvement (e.g. a duel observed from a third character)
// do not seed a fight. The looksLikeNPC heuristic admits NPC names so that
// healers/enchanters/pet classes get a working DPS meter even when they
// don't deal direct damage; this regression-tests the negative case.
func TestPlayerVsPlayerDamageDoesNotSeedFight(t *testing.T) {
	tr := newTestTracker(t)
	now := time.Now()

	tr.Handle(hitEvent("Kildrey", "Hawthor", 60, now))
	tr.Handle(hitEvent("Hawthor", "Kildrey", 50, now.Add(time.Second)))

	st := tr.GetState()
	if st.InCombat {
		t.Fatal("expected no fight from pure player-vs-player damage")
	}
	if st.CurrentFight != nil {
		t.Fatal("expected CurrentFight to remain nil")
	}
}

// TestNPCVsAllyDamageSeedsFight verifies that an NPC attacking a group/raid
// member (not "You") still seeds a fight, so a cleric or enchanter who
// hasn't dealt damage themselves still sees the DPS meter populate when
// their group is in combat.
func TestNPCVsAllyDamageSeedsFight(t *testing.T) {
	tr := newTestTracker(t)
	now := time.Now()

	tr.Handle(hitEvent("an undead pirate", "Kildrey", 50, now))
	tr.Handle(hitEvent("Kildrey", "an undead pirate", 80, now.Add(time.Second)))

	st := tr.GetState()
	if !st.InCombat {
		t.Fatal("expected a fight from group-member combat with an NPC")
	}
	if st.CurrentFight == nil || st.CurrentFight.PrimaryTarget != "an undead pirate" {
		t.Fatalf("expected primary target 'an undead pirate'; got fight=%+v", st.CurrentFight)
	}
}

// TestPrimaryTargetIsAlwaysAnNPC verifies that PrimaryTarget is picked from the
// confirmed-NPC set (incoming ∪ youTargets), not from raw target counts. This
// guards against the bug where a raid boss AoE'd a guildmate many times and
// the guildmate's name became the fight title.
func TestPrimaryTargetIsAlwaysAnNPC(t *testing.T) {
	tr := newTestTracker(t)
	now := time.Now()

	// You hit the boss once — establishes the boss as a confirmed NPC.
	tr.Handle(hitEvent("You", "Aten Ha Ra", 100, now))
	// Boss AoEs Kildrey several times (Kildrey is a guildmate, not an NPC).
	// Without the fix Kildrey would become PrimaryTarget by raw target count.
	tr.Handle(hitEvent("Aten Ha Ra", "Kildrey", 60, now.Add(time.Second)))
	tr.Handle(hitEvent("Aten Ha Ra", "Kildrey", 60, now.Add(2*time.Second)))
	tr.Handle(hitEvent("Aten Ha Ra", "Kildrey", 60, now.Add(3*time.Second)))

	st := tr.GetState()
	if st.CurrentFight == nil {
		t.Fatal("expected an active fight")
	}
	if got := st.CurrentFight.PrimaryTarget; got != "Aten Ha Ra" {
		t.Fatalf("expected PrimaryTarget=%q, got %q", "Aten Ha Ra", got)
	}
}

// TestPrimaryTargetFallsBackToIncomingNPC verifies that an NPC who only ever
// hits "You" (and is never the target of any outgoing attack we observe) is
// still picked as PrimaryTarget — there are no targetCounts entries for it
// since target=="You" routes to the incoming map instead.
func TestPrimaryTargetFallsBackToIncomingNPC(t *testing.T) {
	tr := newTestTracker(t)
	now := time.Now()

	tr.Handle(hitEvent("a fire elemental", "You", 200, now))
	tr.Handle(hitEvent("a fire elemental", "You", 180, now.Add(time.Second)))

	st := tr.GetState()
	if st.CurrentFight == nil {
		t.Fatal("expected an active fight (incoming damage seeds a fight)")
	}
	if got := st.CurrentFight.PrimaryTarget; got != "a fire elemental" {
		t.Fatalf("expected PrimaryTarget=%q, got %q", "a fire elemental", got)
	}
}

func petOwnerEvent(pet, owner string, ts time.Time) logparser.LogEvent {
	return logparser.LogEvent{
		Type:      logparser.EventPetOwner,
		Timestamp: ts,
		Data:      logparser.PetOwnerData{Pet: pet, Owner: owner},
	}
}

func findCombatant(combatants []EntityStats, name string) *EntityStats {
	for i := range combatants {
		if combatants[i].Name == name {
			return &combatants[i]
		}
	}
	return nil
}

// TestPetOwnerStampedFromCharmBind verifies that a pet damaging an NPC after
// the "My leader is X" announcement gets OwnerName=X, so the frontend can
// roll its damage up under the owning player.
func TestPetOwnerStampedFromCharmBind(t *testing.T) {
	tr := newTestTracker(t)
	now := time.Now()

	// Charm binds the pet to Kildrey before the fight starts.
	tr.Handle(petOwnerEvent("Kebartik", "Kildrey", now))

	// You engage the boss; pet (Kebartik) and Kildrey both deal damage.
	tr.Handle(hitEvent("You", "Aten Ha Ra", 100, now.Add(time.Second)))
	tr.Handle(hitEvent("Kildrey", "Aten Ha Ra", 200, now.Add(2*time.Second)))
	tr.Handle(hitEvent("Kebartik", "Aten Ha Ra", 50, now.Add(3*time.Second)))

	st := tr.GetState()
	if st.CurrentFight == nil {
		t.Fatal("expected an active fight")
	}
	pet := findCombatant(st.CurrentFight.Combatants, "Kebartik")
	if pet == nil {
		t.Fatal("expected Kebartik in combatants")
	}
	if pet.OwnerName != "Kildrey" {
		t.Errorf("expected Kebartik OwnerName=Kildrey, got %q", pet.OwnerName)
	}
	owner := findCombatant(st.CurrentFight.Combatants, "Kildrey")
	if owner == nil {
		t.Fatal("expected Kildrey in combatants")
	}
	if owner.OwnerName != "" {
		t.Errorf("expected Kildrey OwnerName empty (player, not pet), got %q", owner.OwnerName)
	}
}

// TestPetOwnerDerivedFromPossessiveName verifies that an entity whose name
// follows the "Owner`s warder" pattern is stamped with the owner even when no
// "My leader is X" line was ever seen — covers magician/necro/beastlord
// summoned pets that don't always announce a leader.
func TestPetOwnerDerivedFromPossessiveName(t *testing.T) {
	tr := newTestTracker(t)
	now := time.Now()

	tr.Handle(hitEvent("You", "a temple skirmisher", 100, now))
	tr.Handle(hitEvent("Grimrose`s warder", "a temple skirmisher", 80, now.Add(time.Second)))

	st := tr.GetState()
	if st.CurrentFight == nil {
		t.Fatal("expected an active fight")
	}
	pet := findCombatant(st.CurrentFight.Combatants, "Grimrose`s warder")
	if pet == nil {
		t.Fatal("expected Grimrose`s warder in combatants")
	}
	if pet.OwnerName != "Grimrose" {
		t.Errorf("expected OwnerName=Grimrose, got %q", pet.OwnerName)
	}
}

// TestPetWithUnknownOwnerHasNoStamp verifies that a pet-named entity whose
// owner is neither in petOwners nor derivable from the name is left with an
// empty OwnerName so the UI keeps it as a separate row.
func TestPetWithUnknownOwnerHasNoStamp(t *testing.T) {
	tr := newTestTracker(t)
	now := time.Now()

	tr.Handle(hitEvent("You", "a gnoll", 100, now))
	tr.Handle(hitEvent("Lobarn", "a gnoll", 50, now.Add(time.Second)))

	st := tr.GetState()
	if st.CurrentFight == nil {
		t.Fatal("expected an active fight")
	}
	ent := findCombatant(st.CurrentFight.Combatants, "Lobarn")
	if ent == nil {
		t.Fatal("expected Lobarn in combatants")
	}
	if ent.OwnerName != "" {
		t.Errorf("expected empty OwnerName for unknown pet, got %q", ent.OwnerName)
	}
}

// TestCharmBreakClearsPetOwnerMapping verifies that once a former pet starts
// hitting the player, the owner mapping is dropped — subsequent damage by
// that entity should not roll up under the old owner.
func TestCharmBreakClearsPetOwnerMapping(t *testing.T) {
	tr := newTestTracker(t)
	now := time.Now()

	// Charm Kebartik to Kildrey, fight a boss.
	tr.Handle(petOwnerEvent("Kebartik", "Kildrey", now))
	tr.Handle(hitEvent("You", "Aten Ha Ra", 100, now.Add(time.Second)))
	tr.Handle(hitEvent("Kebartik", "Aten Ha Ra", 50, now.Add(2*time.Second)))

	// Charm breaks: Kebartik turns and hits You. Mapping should clear.
	tr.Handle(hitEvent("Kebartik", "You", 80, now.Add(3*time.Second)))

	// Now Kebartik attacks the boss again post-break (e.g. AI re-targeted).
	tr.Handle(hitEvent("Kebartik", "Aten Ha Ra", 40, now.Add(4*time.Second)))

	st := tr.GetState()
	if st.CurrentFight == nil {
		t.Fatal("expected an active fight")
	}
	// Kebartik is now in the confirmed-NPC set (it hit You) and gets filtered
	// from the combatants list — verify it does not appear there.
	if pet := findCombatant(st.CurrentFight.Combatants, "Kebartik"); pet != nil {
		t.Fatalf("expected Kebartik to be filtered after charm break, got %+v", pet)
	}
}

func healEvent(actor, target string, amount int, ts time.Time) logparser.LogEvent {
	return logparser.LogEvent{
		Type:      logparser.EventHeal,
		Timestamp: ts,
		Data:      logparser.HealData{Actor: actor, Target: target, Amount: amount},
	}
}

func missEvent(actor, target string, ts time.Time) logparser.LogEvent {
	return logparser.LogEvent{
		Type:      logparser.EventCombatMiss,
		Timestamp: ts,
		Data:      logparser.CombatMissData{Actor: actor, Target: target, MissType: "miss"},
	}
}

// TestHealDuringFightExtendsActivity verifies that recording a heal during an
// active fight bumps the fight's last-activity timestamp so a quiet recovery
// window does not split the fight when the inactivity timer subsequently
// fires.
func TestHealDuringFightExtendsActivity(t *testing.T) {
	tr := newTestTracker(t)
	now := time.Now()

	tr.Handle(hitEvent("You", "Aten Ha Ra", 100, now))
	tr.Handle(healEvent("Cleric1", "You", 500, now.Add(10*time.Second)))

	tr.mu.Lock()
	if tr.active == nil {
		tr.mu.Unlock()
		t.Fatal("expected active fight after heal")
	}
	if got := tr.active.lastHit; !got.Equal(now.Add(10 * time.Second)) {
		tr.mu.Unlock()
		t.Errorf("lastHit = %v, want %v", got, now.Add(10*time.Second))
	}
	tr.mu.Unlock()
}

// TestMissExtendsActivity verifies that EventCombatMiss pushes the inactivity
// window even though no damage lands — important for tank-only fights where
// avoidance dominates.
func TestMissExtendsActivity(t *testing.T) {
	tr := newTestTracker(t)
	now := time.Now()

	tr.Handle(hitEvent("You", "a gnoll", 100, now))
	tr.Handle(missEvent("a gnoll", "You", now.Add(8*time.Second)))

	tr.mu.Lock()
	defer tr.mu.Unlock()
	if tr.active == nil {
		t.Fatal("expected active fight")
	}
	if got := tr.active.lastHit; !got.Equal(now.Add(8 * time.Second)) {
		t.Errorf("lastHit = %v, want %v", got, now.Add(8*time.Second))
	}
}

// TestSameEnemyReopenWithinMergeWindow verifies the inactivity-timer-fires-
// then-combat-resumes case (e.g. raid boss heal phase). After timerExpired
// archives the fight, a new hit on the same enemy within mergeWindow should
// reopen the fight rather than start a fresh one — recentFights stays at
// one entry, and combatant totals accumulate.
func TestSameEnemyReopenWithinMergeWindow(t *testing.T) {
	tr := newTestTracker(t)
	now := time.Now()

	tr.Handle(hitEvent("You", "Aten Ha Ra", 100, now))
	tr.mu.Lock()
	fightID := tr.active.id
	tr.mu.Unlock()

	// Inactivity timer fires (simulated directly).
	tr.timerExpired(fightID)

	st := tr.GetState()
	if len(st.RecentFights) != 1 {
		t.Fatalf("expected 1 archived fight after timer expiry, got %d", len(st.RecentFights))
	}

	// Re-engage the same boss within the merge window.
	tr.Handle(hitEvent("You", "Aten Ha Ra", 200, now.Add(20*time.Second)))

	st = tr.GetState()
	if len(st.RecentFights) != 0 {
		t.Errorf("expected merged fight to be popped from recentFights, got %d", len(st.RecentFights))
	}
	if st.CurrentFight == nil {
		t.Fatal("expected a reopened active fight")
	}
	if st.CurrentFight.TotalDamage != 300 {
		t.Errorf("expected merged TotalDamage=300, got %d", st.CurrentFight.TotalDamage)
	}
	// Session damage should reflect both halves combined (no double count).
	if st.SessionDamage != 0 {
		// Note: lastArchived's session contribution is rolled back on reopen,
		// and the merged fight has not re-archived yet, so SessionDamage
		// should be 0 until the merged fight ends.
		t.Errorf("expected session_damage to be rolled back to 0 pending re-archive, got %d", st.SessionDamage)
	}
}

// TestSameEnemyDoesNotReopenAfterMergeWindow verifies that an identical mob
// name engaged after the merge window starts a fresh fight — distinct pulls
// of the same trash mob should stay separate.
func TestSameEnemyDoesNotReopenAfterMergeWindow(t *testing.T) {
	tr := newTestTracker(t)
	now := time.Now()

	tr.Handle(hitEvent("You", "a temple skirmisher", 100, now))
	tr.mu.Lock()
	fightID := tr.active.id
	tr.mu.Unlock()
	tr.timerExpired(fightID)

	// Re-engage a same-named mob well after mergeWindow elapsed.
	tr.Handle(hitEvent("You", "a temple skirmisher", 80, now.Add(2*time.Minute)))

	st := tr.GetState()
	if len(st.RecentFights) != 1 {
		t.Errorf("expected first fight to remain archived, got %d recent fights", len(st.RecentFights))
	}
	if st.CurrentFight == nil {
		t.Fatal("expected a fresh active fight")
	}
	if st.CurrentFight.TotalDamage != 80 {
		t.Errorf("expected fresh fight TotalDamage=80, got %d", st.CurrentFight.TotalDamage)
	}
}

// TestDifferentEnemyDoesNotReopen verifies that engaging a different mob
// within the merge window starts a fresh fight rather than merging with the
// previous archived fight — trash → boss → trash stays as three fights.
func TestDifferentEnemyDoesNotReopen(t *testing.T) {
	tr := newTestTracker(t)
	now := time.Now()

	tr.Handle(hitEvent("You", "a gnoll", 100, now))
	tr.mu.Lock()
	fightID := tr.active.id
	tr.mu.Unlock()
	tr.timerExpired(fightID)

	tr.Handle(hitEvent("You", "Aten Ha Ra", 200, now.Add(5*time.Second)))

	st := tr.GetState()
	if len(st.RecentFights) != 1 {
		t.Errorf("expected gnoll fight to stay archived, got %d recent fights", len(st.RecentFights))
	}
	if st.CurrentFight == nil || st.CurrentFight.PrimaryTarget != "Aten Ha Ra" {
		t.Fatalf("expected fresh boss fight, got %+v", st.CurrentFight)
	}
}

// TestKillBlocksReopen verifies that a kill event closes the fight definitively
// — even if a same-named mob is engaged within mergeWindow (re-pop of a trash
// mob), the dead mob's fight must not be merged into.
func TestKillBlocksReopen(t *testing.T) {
	tr := newTestTracker(t)
	now := time.Now()

	tr.Handle(hitEvent("You", "a gnoll", 100, now))
	tr.Handle(killEvent("You", "a gnoll", now.Add(time.Second)))

	if len(tr.GetState().RecentFights) != 1 {
		t.Fatalf("expected 1 archived fight after kill")
	}

	// Same-name re-pop within mergeWindow.
	tr.Handle(hitEvent("You", "a gnoll", 80, now.Add(10*time.Second)))

	st := tr.GetState()
	if len(st.RecentFights) != 1 {
		t.Errorf("expected the killed-gnoll fight to stay archived, got %d recent fights", len(st.RecentFights))
	}
	if st.CurrentFight == nil || st.CurrentFight.TotalDamage != 80 {
		t.Fatalf("expected a fresh fight on the new gnoll, got %+v", st.CurrentFight)
	}
}

// TestTrashBossTrashStaysSeparate is the regression case from the chunk-4
// motivation: pulling trash, then a boss, then more trash must produce three
// fights — none of the inactivity-timer expiries should merge unrelated mobs.
func TestTrashBossTrashStaysSeparate(t *testing.T) {
	tr := newTestTracker(t)
	now := time.Now()

	// Trash 1
	tr.Handle(hitEvent("You", "a temple skirmisher", 100, now))
	tr.mu.Lock()
	id1 := tr.active.id
	tr.mu.Unlock()
	tr.timerExpired(id1)

	// Boss
	tr.Handle(hitEvent("You", "Aten Ha Ra", 500, now.Add(20*time.Second)))
	tr.mu.Lock()
	id2 := tr.active.id
	tr.mu.Unlock()
	tr.timerExpired(id2)

	// Trash 2 — different name, well after the boss fight's mergeWindow
	tr.Handle(hitEvent("You", "a Shissar Arch Arcanist", 80, now.Add(2*time.Minute)))
	tr.mu.Lock()
	id3 := tr.active.id
	tr.mu.Unlock()
	tr.timerExpired(id3)

	st := tr.GetState()
	if len(st.RecentFights) != 3 {
		t.Errorf("expected 3 fights, got %d", len(st.RecentFights))
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
