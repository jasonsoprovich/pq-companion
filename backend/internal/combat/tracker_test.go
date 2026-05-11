package combat

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// newTestTracker returns a Tracker wired to a running ws.Hub suitable for tests.
// playerNameFn is nil so "You" rows render unchanged — the few tests that
// exercise the relabel path construct their own tracker with a non-nil one.
func newTestTracker(t *testing.T) *Tracker {
	t.Helper()
	hub := ws.NewHub()
	go hub.Run()
	return NewTracker(hub, nil)
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

func critEvent(actor string, dmg int, ts time.Time) logparser.LogEvent {
	return logparser.LogEvent{
		Type:      logparser.EventCritHit,
		Timestamp: ts,
		Data:      logparser.CritHitData{Actor: actor, Damage: dmg},
	}
}

func dotTickEvent(target string, dmg int, spell string, ts time.Time) logparser.LogEvent {
	return logparser.LogEvent{
		Type:      logparser.EventCombatHit,
		Timestamp: ts,
		Data: logparser.CombatHitData{
			Actor:     "You",
			Skill:     "dot",
			Target:    target,
			Damage:    dmg,
			SpellName: spell,
		},
	}
}

// requireCombatant locates a named entity in the current fight's combatant
// list, failing the test when missing. Wrapper around the existing
// findCombatant helper that asserts the current fight exists.
func requireCombatant(t *testing.T, st CombatState, name string) *EntityStats {
	t.Helper()
	if st.CurrentFight == nil {
		t.Fatalf("expected CurrentFight to be set when looking up %q", name)
	}
	c := findCombatant(st.CurrentFight.Combatants, name)
	if c == nil {
		t.Fatalf("combatant %q not found in %d combatants", name, len(st.CurrentFight.Combatants))
	}
	return c
}

// activeFightCount returns the number of currently-active per-NPC fights —
// a small accessor so tests don't reach into Tracker internals.
func activeFightCount(tr *Tracker) int {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	return len(tr.activeFights)
}

// activeFightFor returns the active Fight for npcName (or nil), used by
// tests to inspect per-fight state without exporting internals package-wide.
func activeFightFor(tr *Tracker, npcName string) *Fight {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	return tr.activeFights[npcName]
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
	// Duration should be ~3s (start to killTS), well under fightExpiryWithDamage (30s).
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

// TestYouRelabeledToCharacterName verifies that when a PlayerNameProvider is
// wired, the "You" combatant row is renamed to the active character's name on
// output. This is what lets the frontend rollup merge the player's row with
// pet rows whose OwnerName already carries the character's canonical name —
// without it the UI shows "You" and "Osui (+pet)" as two separate rows.
func TestYouRelabeledToCharacterName(t *testing.T) {
	hub := ws.NewHub()
	go hub.Run()
	tr := NewTracker(hub, func() string { return "Osui" })
	now := time.Now()

	// Charm bind the pet to Osui — so the pet damage rolls under "Osui",
	// matching the renamed player row.
	tr.Handle(petOwnerEvent("a shissar revenant", "Osui", now))

	// You deal a tiny amount (Asphyxiate tick) and the charmed pet does most
	// of the damage. Without the relabel, "You" and "a shissar revenant"
	// (OwnerName=Osui) end up in two separate frontend rows.
	tr.Handle(hitEvent("You", "Pli Thall Xakra", 50, now.Add(time.Second)))
	tr.Handle(hitEvent("a shissar revenant", "Pli Thall Xakra", 800, now.Add(2*time.Second)))

	st := tr.GetState()
	if st.CurrentFight == nil {
		t.Fatal("expected an active fight")
	}
	if youRow := findCombatant(st.CurrentFight.Combatants, "You"); youRow != nil {
		t.Errorf("expected no 'You' row when player name is wired; got one with %d damage", youRow.TotalDamage)
	}
	osuiRow := findCombatant(st.CurrentFight.Combatants, "Osui")
	if osuiRow == nil {
		t.Fatal("expected 'Osui' row (renamed from 'You') in combatants")
	}
	if osuiRow.TotalDamage != 50 {
		t.Errorf("expected Osui personal damage 50, got %d", osuiRow.TotalDamage)
	}
	pet := findCombatant(st.CurrentFight.Combatants, "a shissar revenant")
	if pet == nil {
		t.Fatal("expected charmed pet row")
	}
	if pet.OwnerName != "Osui" {
		t.Errorf("expected pet OwnerName=Osui, got %q", pet.OwnerName)
	}
	// YouDamage internal aggregate should still pivot on the "You" key,
	// independent of the relabel — the player's own damage is 50.
	if st.CurrentFight.YouDamage != 50 {
		t.Errorf("expected YouDamage=50, got %d", st.CurrentFight.YouDamage)
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

// TestCharmedPetSurvivesPriorPlayerDamage verifies that charming a mob you
// previously damaged still attributes its post-charm damage to the player.
// Regression: confirmedNPCs's youTargets loop was missing the petOwners skip
// the other three loops have, so any mob you tagged before charming ended up
// in the NPC set and excludeNPCs stripped its row — the charmed pet's damage
// vanished from the meter regardless of the rollup toggle.
func TestCharmedPetSurvivesPriorPlayerDamage(t *testing.T) {
	tr := newTestTracker(t)
	now := time.Now()

	// You tag the mob with a DD before charming (Asphyxiate, melee, etc.) —
	// this puts it in youTargets.
	tr.Handle(hitEvent("You", "a temple guard", 30, now))

	// Charm lands.
	tr.Handle(petOwnerEvent("a temple guard", "Kildrey", now.Add(time.Second)))

	// Charmed pet attacks a different mob in the room.
	tr.Handle(hitEvent("a temple guard", "another temple guard", 400, now.Add(2*time.Second)))
	tr.Handle(hitEvent("You", "another temple guard", 50, now.Add(3*time.Second)))

	st := tr.GetState()
	if st.CurrentFight == nil {
		t.Fatal("expected an active fight")
	}
	pet := findCombatant(st.CurrentFight.Combatants, "a temple guard")
	if pet == nil {
		t.Fatal("expected charmed pet 'a temple guard' in combatants — youTargets gap dropped it")
	}
	if pet.OwnerName != "Kildrey" {
		t.Errorf("expected pet OwnerName=Kildrey, got %q", pet.OwnerName)
	}
	if pet.TotalDamage != 400 {
		t.Errorf("expected pet damage=400, got %d", pet.TotalDamage)
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

// A pure healer's first action is often an outgoing heal — no incoming
// damage on themselves yet, no outgoing damage ever. The tracker must seed
// a fight from that heal (when "You" is involved) so subsequent NPC hits
// Under the per-NPC fight model heals no longer seed a fight on their own —
// a damage line touching an NPC has to start the fight first. This test
// captures the new behavior: a heal that arrives before any damage event is
// dropped, and a heal that arrives after a damage line attributes to the
// most-recently-touched fight.
func TestHealAttributesToMostRecentActiveFight(t *testing.T) {
	tr := newTestTracker(t)
	now := time.Now()

	// Heal-before-engage is dropped (no NPC fight to attach to).
	tr.Handle(healEvent("You", "Tank", 200, now))
	if got := activeFightCount(tr); got != 0 {
		t.Fatalf("heal before any damage should not seed a fight, got %d active", got)
	}

	// A damage line seeds the fight; the next heal lands inside it.
	tr.Handle(hitEvent("You", "Aten Ha Ra", 100, now.Add(time.Second)))
	tr.Handle(healEvent("You", "Tank", 1000, now.Add(2*time.Second)))

	st := tr.GetState()
	if st.CurrentFight == nil {
		t.Fatal("expected a current fight after the seeding hit")
	}
	if st.CurrentFight.YouHeal != 1000 {
		t.Errorf("YouHeal = %d, want 1000", st.CurrentFight.YouHeal)
	}
}

// Heals between two third-party players (no involvement of "You") still
// must NOT seed a fight — under the per-NPC model nothing can seed from a
// heal at all, so this is the trivial case.
func TestHealBetweenThirdPartiesDoesNotSeedFight(t *testing.T) {
	tr := newTestTracker(t)
	now := time.Now()

	tr.Handle(healEvent("Cleric1", "Tank", 500, now))

	if got := activeFightCount(tr); got != 0 {
		t.Fatalf("heal between unrelated parties should not seed a fight, got %d active", got)
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

	f := activeFightFor(tr, "Aten Ha Ra")
	if f == nil {
		t.Fatal("expected active fight on Aten Ha Ra after heal")
	}
	if got := f.lastTouched; !got.Equal(now.Add(10 * time.Second)) {
		t.Errorf("lastTouched = %v, want %v", got, now.Add(10*time.Second))
	}
}

// TestMissExtendsActivity verifies that EventCombatMiss pushes the inactivity
// window even though no damage lands — important for tank-only fights where
// avoidance dominates.
func TestMissExtendsActivity(t *testing.T) {
	tr := newTestTracker(t)
	now := time.Now()

	tr.Handle(hitEvent("You", "a gnoll", 100, now))
	tr.Handle(missEvent("a gnoll", "You", now.Add(8*time.Second)))

	f := activeFightFor(tr, "a gnoll")
	if f == nil {
		t.Fatal("expected active fight on a gnoll")
	}
	if got := f.lastTouched; !got.Equal(now.Add(8 * time.Second)) {
		t.Errorf("lastTouched = %v, want %v", got, now.Add(8*time.Second))
	}
}

// TestMultiMobPullProducesPerNPCFights verifies the core Phase 2 behavior:
// hitting two different NPCs within the same time window produces two
// independent fights, each archivable on its own kill. Under the previous
// pooled-encounter model both mobs would have collapsed into one summary.
func TestMultiMobPullProducesPerNPCFights(t *testing.T) {
	tr := newTestTracker(t)
	now := time.Now()

	tr.Handle(hitEvent("You", "a gnoll", 100, now))
	tr.Handle(hitEvent("You", "a wolf", 80, now.Add(time.Second)))

	if got := activeFightCount(tr); got != 2 {
		t.Fatalf("expected 2 active fights for 2 distinct NPCs, got %d", got)
	}

	tr.Handle(killEvent("You", "a gnoll", now.Add(2*time.Second)))
	if got := activeFightCount(tr); got != 1 {
		t.Fatalf("after killing gnoll, expected 1 active fight, got %d", got)
	}
	tr.Handle(killEvent("You", "a wolf", now.Add(3*time.Second)))

	st := tr.GetState()
	if len(st.RecentFights) != 2 {
		t.Errorf("expected 2 archived fights, got %d", len(st.RecentFights))
	}
	// Each fight's PrimaryTarget should be its own NPC (no heuristic guess).
	targets := map[string]bool{}
	for _, f := range st.RecentFights {
		targets[f.PrimaryTarget] = true
	}
	if !targets["a gnoll"] || !targets["a wolf"] {
		t.Errorf("expected both NPCs as PrimaryTarget across summaries, got %v", targets)
	}
}

// TestSameNameRepullProducesSeparateFights captures that two distinct pulls
// of an identically-named mob (engaged after the previous fight is closed)
// archive as separate fights. The previous mergeWindow could collapse these;
// per-NPC fights cannot, because the second engage hits a fresh map slot
// after the first's kill removes it.
func TestSameNameRepullProducesSeparateFights(t *testing.T) {
	tr := newTestTracker(t)
	now := time.Now()

	tr.Handle(hitEvent("You", "a temple skirmisher", 100, now))
	tr.Handle(killEvent("You", "a temple skirmisher", now.Add(time.Second)))
	tr.Handle(hitEvent("You", "a temple skirmisher", 80, now.Add(10*time.Second)))
	tr.Handle(killEvent("You", "a temple skirmisher", now.Add(11*time.Second)))

	st := tr.GetState()
	if len(st.RecentFights) != 2 {
		t.Errorf("expected 2 archived fights for two distinct pulls, got %d", len(st.RecentFights))
	}
}

// TestKillEndsOnlyTheNamedFight ensures killing one NPC in a multi-mob pull
// doesn't tear down the other in-flight fight. The previous global-active
// model would archive the entire encounter; per-NPC fights only close the
// named one.
func TestKillEndsOnlyTheNamedFight(t *testing.T) {
	tr := newTestTracker(t)
	now := time.Now()

	tr.Handle(hitEvent("You", "a gnoll", 100, now))
	tr.Handle(hitEvent("You", "Aten Ha Ra", 500, now.Add(time.Second)))
	tr.Handle(killEvent("You", "a gnoll", now.Add(2*time.Second)))

	if activeFightFor(tr, "Aten Ha Ra") == nil {
		t.Fatal("boss fight should still be active after killing trash")
	}
	if activeFightFor(tr, "a gnoll") != nil {
		t.Fatal("gnoll fight should be archived after kill")
	}
}

// TestZoneEndsAllActiveFights confirms that a zone change archives every
// in-flight fight, not just the most-recent one.
func TestZoneEndsAllActiveFights(t *testing.T) {
	tr := newTestTracker(t)
	now := time.Now()

	tr.Handle(hitEvent("You", "a gnoll", 100, now))
	tr.Handle(hitEvent("You", "a wolf", 80, now.Add(time.Second)))
	tr.Handle(zoneEvent(now.Add(2 * time.Second)))

	if got := activeFightCount(tr); got != 0 {
		t.Fatalf("expected 0 active fights after zone, got %d", got)
	}
	if got := len(tr.GetState().RecentFights); got != 2 {
		t.Errorf("expected 2 archived fights after zone, got %d", got)
	}
}

// TestCurrentFightTracksMostRecentlyTouched verifies the snapshot picks the
// fight whose lastTouched is most recent — it should switch as the player
// (or a hostile mob) targets a new NPC mid-pull.
func TestCurrentFightTracksMostRecentlyTouched(t *testing.T) {
	tr := newTestTracker(t)
	now := time.Now()

	tr.Handle(hitEvent("You", "a gnoll", 100, now))
	tr.Handle(hitEvent("You", "a wolf", 80, now.Add(2*time.Second)))

	st := tr.GetState()
	if st.CurrentFight == nil || st.CurrentFight.PrimaryTarget != "a wolf" {
		t.Fatalf("expected CurrentFight to track 'a wolf', got %+v", st.CurrentFight)
	}

	// Swap back to the gnoll — CurrentFight should follow.
	tr.Handle(hitEvent("You", "a gnoll", 50, now.Add(3*time.Second)))
	st = tr.GetState()
	if st.CurrentFight == nil || st.CurrentFight.PrimaryTarget != "a gnoll" {
		t.Fatalf("expected CurrentFight to track 'a gnoll' after re-engage, got %+v", st.CurrentFight)
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

// findCombatantByName returns the combatant entry with the given name from
// the most recently archived fight, or nil if not found.
func findArchivedCombatant(st CombatState, name string) *EntityStats {
	if len(st.RecentFights) == 0 {
		return nil
	}
	for i := range st.RecentFights[0].Combatants {
		if st.RecentFights[0].Combatants[i].Name == name {
			return &st.RecentFights[0].Combatants[i]
		}
	}
	return nil
}

// A constantly-engaged attacker (hits every 2-3 seconds — well within the
// activeGapWindow) should have ActiveDPS within ~10% of the fight-duration
// DPS, since their active window roughly equals the fight duration.
func TestActiveDPS_MatchesDPSForConstantAttacker(t *testing.T) {
	tr := newTestTracker(t)
	now := time.Now()

	for i := 0; i < 10; i++ {
		tr.Handle(hitEvent("You", "a gnoll", 100, now.Add(time.Duration(i*3)*time.Second)))
	}
	tr.Handle(killEvent("You", "a gnoll", now.Add(28*time.Second)))

	st := tr.GetState()
	c := findArchivedCombatant(st, "You")
	if c == nil {
		t.Fatalf("missing 'You' combatant; combatants=%+v", st.RecentFights)
	}
	if c.DPS == 0 || c.ActiveDPS == 0 {
		t.Fatalf("dps=%v active=%v expected both nonzero", c.DPS, c.ActiveDPS)
	}
	ratio := c.ActiveDPS / c.DPS
	if ratio < 0.9 || ratio > 1.15 {
		t.Errorf("constant attacker ActiveDPS/DPS ratio %.2f out of expected range; dps=%v active=%v active_secs=%.2f",
			ratio, c.DPS, c.ActiveDPS, c.ActiveSeconds)
	}
}

// TestPersonalDPS_LateJoinerIsHigherThanEncounter captures the EQLP
// "Personal DPS" intuition: a player who engaged late should be judged on
// the time they were actually swinging, not on the full fight wall-clock.
// Their ActiveDPS (Personal) > Encounter DPS, while a constant attacker's
// numbers are roughly equal.
func TestPersonalDPS_LateJoinerIsHigherThanEncounter(t *testing.T) {
	tr := newTestTracker(t)
	now := time.Now()

	// Tank engages at t=0 and swings to t=60. Wizard arrives at t=30 and
	// nukes through t=60. Both deal 6000 damage total; Wizard's personal
	// span is half the fight wall-clock so ActiveDPS should be ~2× DPS.
	for i := 0; i < 6; i++ {
		tr.Handle(hitEvent("Tank", "a gnoll", 1000, now.Add(time.Duration(i*10)*time.Second)))
	}
	for i := 0; i < 6; i++ {
		tr.Handle(hitEvent("Wizard", "a gnoll", 1000, now.Add(time.Duration(30+i*5)*time.Second)))
	}
	tr.Handle(killEvent("You", "a gnoll", now.Add(60*time.Second)))

	st := tr.GetState()
	wiz := findArchivedCombatant(st, "Wizard")
	if wiz == nil {
		t.Fatalf("missing Wizard combatant")
	}
	if wiz.ActiveDPS < wiz.DPS*1.5 {
		t.Errorf("late-joiner ActiveDPS (%.0f) should be ~2× DPS (%.0f)", wiz.ActiveDPS, wiz.DPS)
	}
	// Personal span is exactly 30s (5 nukes 5s apart starting at t=30, ending
	// at t=55) — give or take a beat for floor handling.
	if wiz.ActiveSeconds < 24 || wiz.ActiveSeconds > 32 {
		t.Errorf("Wizard ActiveSeconds = %.1f, want ~25–30 (their first-to-last span)", wiz.ActiveSeconds)
	}
}

// TestRaidDPS_UsesRaidWideSpanAcrossPlayers verifies the new raid-relative
// metric: every combatant's raid_seconds is the same value (raid first
// activity → raid last activity), so a late-joining player's RaidDPS is
// strictly less than their ActiveDPS (personal). This is the metric that
// makes cross-player rankings fair within one fight.
func TestRaidDPS_UsesRaidWideSpanAcrossPlayers(t *testing.T) {
	tr := newTestTracker(t)
	now := time.Now()

	// Tank engages at t=0; Wizard at t=30. Both kill the mob at t=60.
	tr.Handle(hitEvent("Tank", "a gnoll", 1000, now))
	tr.Handle(hitEvent("Tank", "a gnoll", 1000, now.Add(60*time.Second)))
	tr.Handle(hitEvent("Wizard", "a gnoll", 5000, now.Add(30*time.Second)))
	tr.Handle(hitEvent("Wizard", "a gnoll", 5000, now.Add(60*time.Second)))
	tr.Handle(killEvent("You", "a gnoll", now.Add(60*time.Second)))

	st := tr.GetState()
	tank := findArchivedCombatant(st, "Tank")
	wiz := findArchivedCombatant(st, "Wizard")
	if tank == nil || wiz == nil {
		t.Fatalf("missing Tank or Wizard combatant")
	}
	// raid_seconds should be ~60 (raid first hit at t=0 to last hit at t=60)
	// and identical for both players.
	if tank.RaidSeconds != wiz.RaidSeconds {
		t.Errorf("RaidSeconds mismatch: tank=%v wiz=%v (must be the same)", tank.RaidSeconds, wiz.RaidSeconds)
	}
	if tank.RaidSeconds < 55 || tank.RaidSeconds > 65 {
		t.Errorf("RaidSeconds = %.1f, want ~60", tank.RaidSeconds)
	}
	// Wizard's RaidDPS uses the full raid span; their ActiveDPS uses just
	// their 30s personal span. So ActiveDPS > RaidDPS.
	if wiz.ActiveDPS <= wiz.RaidDPS {
		t.Errorf("late-joiner ActiveDPS (%.0f) should exceed RaidDPS (%.0f)", wiz.ActiveDPS, wiz.RaidDPS)
	}
}

// A combatant with a single hit must not divide by zero — ActiveDPS should
// reflect the activeMinSegment floor rather than NaN/Inf.
func TestActiveDPS_SingleHitUsesMinSegment(t *testing.T) {
	tr := newTestTracker(t)
	now := time.Now()

	tr.Handle(hitEvent("You", "a gnoll", 1000, now))
	tr.Handle(killEvent("You", "a gnoll", now.Add(2*time.Second)))

	st := tr.GetState()
	c := findArchivedCombatant(st, "You")
	if c == nil {
		t.Fatalf("missing 'You' combatant")
	}
	if c.ActiveSeconds != 1.0 {
		t.Errorf("single-hit ActiveSeconds: got %.2f, want %.2f (min segment)", c.ActiveSeconds, 1.0)
	}
	if c.ActiveDPS != 1000 {
		t.Errorf("single-hit ActiveDPS: got %v, want 1000 (1000 dmg / 1s)", c.ActiveDPS)
	}
}

// TestCritCorrelationMatchesNextDamage exercises the PQ-format crit flow:
// a "Scores a critical hit!(N)" event is emitted, followed by the matching
// damage event from the same actor. The crit count and crit damage on the
// matching combatant should be incremented by exactly one matching hit.
func TestCritCorrelationMatchesNextDamage(t *testing.T) {
	tr := newTestTracker(t)
	now := time.Now()

	// Crit announcement, then matching damage line — same actor, same amount.
	tr.Handle(critEvent("You", 201, now))
	tr.Handle(hitEvent("You", "a gnoll", 201, now))
	// A second non-crit hit shouldn't bump crit counts.
	tr.Handle(hitEvent("You", "a gnoll", 50, now.Add(time.Second)))

	st := tr.GetState()
	c := requireCombatant(t, st, "You")
	if c.HitCount != 2 {
		t.Fatalf("HitCount = %d, want 2", c.HitCount)
	}
	if c.TotalDamage != 251 {
		t.Fatalf("TotalDamage = %d, want 251", c.TotalDamage)
	}
	if c.CritCount != 1 {
		t.Fatalf("CritCount = %d, want 1", c.CritCount)
	}
	if c.CritDamage != 201 {
		t.Fatalf("CritDamage = %d, want 201", c.CritDamage)
	}
}

// TestCritWithoutMatchingHitNotCounted ensures a crit announcement that
// never finds a matching damage line doesn't bleed into the next hit and
// doesn't crash. (PQ doesn't normally produce orphan crit lines, but log
// truncation or out-of-order events shouldn't corrupt accounting.)
func TestCritWithoutMatchingHitNotCounted(t *testing.T) {
	tr := newTestTracker(t)
	now := time.Now()

	// Crit announces 100, but the next hit is for a different amount —
	// the crit stays queued and is never matched.
	tr.Handle(critEvent("You", 100, now))
	tr.Handle(hitEvent("You", "a gnoll", 75, now))

	st := tr.GetState()
	c := requireCombatant(t, st, "You")
	if c.CritCount != 0 {
		t.Fatalf("CritCount = %d, want 0 (amount mismatch must not match)", c.CritCount)
	}
}

// TestCritMatchesPerActor verifies pending crits don't leak across actors —
// a crit announcement for one player must not flag the next damage event from
// a different player.
func TestCritMatchesPerActor(t *testing.T) {
	tr := newTestTracker(t)
	now := time.Now()

	// Seed a fight so both actors have entities.
	tr.Handle(hitEvent("Sandrian", "a gnoll", 10, now))
	tr.Handle(hitEvent("You", "a gnoll", 10, now))

	// Crit announcement for Sandrian, then a same-amount hit by "You" —
	// should NOT be marked as a crit on "You".
	tr.Handle(critEvent("Sandrian", 50, now.Add(time.Second)))
	tr.Handle(hitEvent("You", "a gnoll", 50, now.Add(time.Second)))

	st := tr.GetState()
	you := requireCombatant(t, st, "You")
	if you.CritCount != 0 {
		t.Fatalf("You.CritCount = %d, want 0 (crit was queued for Sandrian)", you.CritCount)
	}

	// Sandrian's matching hit lands later — that one should be a crit.
	tr.Handle(hitEvent("Sandrian", "a gnoll", 50, now.Add(2*time.Second)))
	st = tr.GetState()
	sandrian := requireCombatant(t, st, "Sandrian")
	if sandrian.CritCount != 1 {
		t.Fatalf("Sandrian.CritCount = %d, want 1", sandrian.CritCount)
	}
	if sandrian.CritDamage != 50 {
		t.Fatalf("Sandrian.CritDamage = %d, want 50", sandrian.CritDamage)
	}
}

// TestPendingCritsBounded confirms the per-actor pending-crit queue is capped
// so a stream of unmatched crits can't grow without bound.
func TestPendingCritsBounded(t *testing.T) {
	tr := newTestTracker(t)
	now := time.Now()

	// Push more than the cap; ensure the queue length stays bounded.
	for i := 0; i < maxPendingCritsPerActor*3; i++ {
		tr.Handle(critEvent("Spammer", i+1, now))
	}
	tr.mu.Lock()
	got := len(tr.pendingCrits["Spammer"])
	tr.mu.Unlock()
	if got != maxPendingCritsPerActor {
		t.Fatalf("pendingCrits[Spammer] length = %d, want %d", got, maxPendingCritsPerActor)
	}
}

// TestArchivedFightsPersistedWhenStoreWired verifies the SetHistoryStore
// integration: every archived fight ends up in user.db, with the same
// PrimaryTarget and totals as the in-memory FightSummary.
func TestArchivedFightsPersistedWhenStoreWired(t *testing.T) {
	tr := newTestTracker(t)
	store := newTestStore(t)
	tr.SetHistoryStore(store)

	now := time.Now()
	tr.Handle(hitEvent("You", "a gnoll", 100, now))
	tr.Handle(hitEvent("You", "a gnoll", 50, now.Add(time.Second)))
	tr.Handle(killEvent("You", "a gnoll", now.Add(2*time.Second)))

	saved, err := store.ListFights(FightFilter{})
	if err != nil {
		t.Fatalf("ListFights: %v", err)
	}
	if len(saved) != 1 {
		t.Fatalf("expected 1 persisted fight, got %d", len(saved))
	}
	if saved[0].NPCName != "a gnoll" {
		t.Errorf("persisted NPCName = %q, want %q", saved[0].NPCName, "a gnoll")
	}
	if saved[0].YouDamage != 150 {
		t.Errorf("persisted YouDamage = %d, want 150", saved[0].YouDamage)
	}
}

// verifiedPlayerEvent / charmedPetEvent / charmBrokenEvent — test helpers
// for the new disambiguation events. Mirror the existing helper style.

func verifiedPlayerEvent(name string, ts time.Time) logparser.LogEvent {
	return logparser.LogEvent{
		Type:      logparser.EventVerifiedPlayer,
		Timestamp: ts,
		Data:      logparser.VerifiedPlayerData{Name: name},
	}
}

func charmedPetEvent(pet string, ts time.Time) logparser.LogEvent {
	return logparser.LogEvent{
		Type:      logparser.EventCharmedPet,
		Timestamp: ts,
		Data:      logparser.CharmedPetData{Pet: pet},
	}
}

func charmBrokenEvent(ts time.Time) logparser.LogEvent {
	return logparser.LogEvent{
		Type:      logparser.EventCharmBroken,
		Timestamp: ts,
		Data:      nil,
	}
}

// TestVerifiedPlayerRoutesSingleWordBossAsNPC is the Zlandicar regression
// case: a single-word capitalised boss name (fails looksLikeNPC) and a
// single-word player attacker should still get routed correctly as long
// as the player has been verified via a prior chat-channel line.
func TestVerifiedPlayerRoutesSingleWordBossAsNPC(t *testing.T) {
	tr := newTestTracker(t)
	now := time.Now()

	// Sandrian chats earlier; that's all it takes to verify her as a player.
	tr.Handle(verifiedPlayerEvent("Sandrian", now))

	// First hit on the boss: third-party form, both single-capitalised.
	// Under the old routing both failed looksLikeNPC and the hit was
	// dropped. The verifiedPlayers asymmetry now identifies Zlandicar.
	tr.Handle(hitEvent("Sandrian", "Zlandicar", 1000, now.Add(time.Second)))

	if got := activeFightCount(tr); got != 1 {
		t.Fatalf("expected 1 active fight on Zlandicar, got %d", got)
	}
	if activeFightFor(tr, "Zlandicar") == nil {
		t.Fatal("expected fight keyed on 'Zlandicar'")
	}

	// Subsequent third-party hits — including hits FROM Zlandicar back
	// onto players — should route to the same fight thanks to the
	// activeFights lookup, not require re-verification.
	tr.Handle(hitEvent("Takkisina", "Zlandicar", 500, now.Add(2*time.Second)))
	tr.Handle(hitEvent("Zlandicar", "Sandrian", 200, now.Add(3*time.Second)))

	st := tr.GetState()
	if st.CurrentFight == nil || st.CurrentFight.PrimaryTarget != "Zlandicar" {
		t.Fatalf("expected current fight on Zlandicar, got %+v", st.CurrentFight)
	}
	// Sandrian and Takkisina appear; Zlandicar itself is filtered (it's
	// the fight's own NPC). Total damage = 1000 + 500 from the players.
	if st.CurrentFight.TotalDamage != 1500 {
		t.Errorf("TotalDamage = %d, want 1500", st.CurrentFight.TotalDamage)
	}
}

// TestUnverifiedPlayerVsPlayerNoFight ensures we don't over-correct: when
// neither side is a verified player and neither structurally looks like
// an NPC, the hit is still dropped (player-vs-player or unknown).
func TestUnverifiedPlayerVsPlayerNoFight(t *testing.T) {
	tr := newTestTracker(t)
	now := time.Now()

	tr.Handle(hitEvent("Stranger", "Friend", 100, now))

	if got := activeFightCount(tr); got != 0 {
		t.Fatalf("expected no fight from unverified-vs-unverified hit, got %d active", got)
	}
}

// TestCharmedPetTellBindsOwner verifies the charmed-pet "tells you" path.
// The pet's subsequent damage to a third-party mob should appear in that
// mob's fight with the active character as OwnerName.
func TestCharmedPetTellBindsOwner(t *testing.T) {
	hub := ws.NewHub()
	go hub.Run()
	tr := NewTracker(hub, func() string { return "Osui" })
	now := time.Now()

	// Charm-tell binds "a fetid fiend" to Osui without any "My leader is" line.
	tr.Handle(charmedPetEvent("a fetid fiend", now))
	tr.Handle(hitEvent("a fetid fiend", "a spinechiller spider", 400, now.Add(time.Second)))
	tr.Handle(hitEvent("You", "a spinechiller spider", 100, now.Add(2*time.Second)))

	st := tr.GetState()
	pet := findCombatant(st.CurrentFight.Combatants, "a fetid fiend")
	if pet == nil {
		t.Fatal("expected 'a fetid fiend' in combatants")
	}
	if pet.OwnerName != "Osui" {
		t.Errorf("OwnerName = %q, want %q", pet.OwnerName, "Osui")
	}
	if pet.TotalDamage != 400 {
		t.Errorf("pet TotalDamage = %d, want 400", pet.TotalDamage)
	}
}

// TestCharmBrokenReleasesCharmedPet exercises the cleanup half of charm
// binding: after "Your charm spell has worn off", the petOwners entry the
// charm tell installed is gone, so subsequent damage by that name is not
// attributed to the player. (How prior damage is displayed is a separate
// concern — the binding is what matters here.)
func TestCharmBrokenReleasesCharmedPet(t *testing.T) {
	hub := ws.NewHub()
	go hub.Run()
	tr := NewTracker(hub, func() string { return "Osui" })
	now := time.Now()

	tr.Handle(charmedPetEvent("a fetid fiend", now))

	// Binding present.
	tr.mu.Lock()
	_, beforeBound := tr.petOwners["a fetid fiend"]
	_, beforeCharm := tr.charmedPets["a fetid fiend"]
	tr.mu.Unlock()
	if !beforeBound || !beforeCharm {
		t.Fatalf("pre-break: petOwners=%v charmedPets=%v, want both bound", beforeBound, beforeCharm)
	}

	tr.Handle(charmBrokenEvent(now.Add(3 * time.Second)))

	// Binding gone.
	tr.mu.Lock()
	_, afterBound := tr.petOwners["a fetid fiend"]
	_, afterCharm := tr.charmedPets["a fetid fiend"]
	tr.mu.Unlock()
	if afterBound || afterCharm {
		t.Errorf("post-break: petOwners=%v charmedPets=%v, want both cleared", afterBound, afterCharm)
	}
}

// TestCharmBrokenDoesNotClearSummonedPet protects the summon binding from
// charm-break cleanup. Magician/necro summon pets ("X says 'My leader is
// Y'") must persist across charm-break events from a different pet.
func TestCharmBrokenDoesNotClearSummonedPet(t *testing.T) {
	hub := ws.NewHub()
	go hub.Run()
	tr := NewTracker(hub, func() string { return "Osui" })
	now := time.Now()

	// Summoned pet bound via "My leader is" — must not be flagged charmed.
	tr.Handle(petOwnerEvent("Vebekab", "Kildrey", now))
	// Charm a different mob.
	tr.Handle(charmedPetEvent("a fetid fiend", now.Add(time.Second)))
	// Charm wears off.
	tr.Handle(charmBrokenEvent(now.Add(2 * time.Second)))

	tr.mu.Lock()
	_, sumBound := tr.petOwners["Vebekab"]
	_, charmBound := tr.petOwners["a fetid fiend"]
	tr.mu.Unlock()
	if !sumBound {
		t.Error("summoned pet Vebekab should remain bound after charm break")
	}
	if charmBound {
		t.Error("charmed pet 'a fetid fiend' should be released")
	}
}

// TestEyeOfPlayerSkippedAsFight verifies that the magician scout-pet
// dismiss pattern doesn't pollute the meter with 0-second 60K-DPS rows.
func TestEyeOfPlayerSkippedAsFight(t *testing.T) {
	tr := newTestTracker(t)
	now := time.Now()

	// Mage attacks her own Eye to dismiss it — same pattern that produced
	// "Eye of Rora · 0s · 60000 DPS" rows in the user's history.
	tr.Handle(hitEvent("Rora", "Eye of Rora", 60, now))

	if got := activeFightCount(tr); got != 0 {
		t.Fatalf("expected no fight for 'Eye of Rora' dismiss, got %d active", got)
	}
}

// TestZeroOutgoingFightDiscarded confirms a fight that only logged
// incoming player damage (got-hit-running-past) doesn't archive as a
// 0-DPS row. Required against a backdrop where the inactivity timer
// eventually expires.
func TestZeroOutgoingFightDiscarded(t *testing.T) {
	tr := newTestTracker(t)
	now := time.Now()

	// NPC hits You; You never hit back; fight ends via zone change.
	tr.Handle(hitEvent("a scareling", "You", 100, now))
	tr.Handle(zoneEvent(now.Add(time.Second)))

	st := tr.GetState()
	if got := len(st.RecentFights); got != 0 {
		t.Errorf("expected 0-outgoing fight to be discarded, got %d archived", got)
	}
}

// TestDoTTickAttributedToYou verifies the new DoT tick event flows through
// the tracker as outgoing damage credited to "You", same as other player
// damage. The spell name on the event is informational and shouldn't affect
// accounting.
func TestDoTTickAttributedToYou(t *testing.T) {
	tr := newTestTracker(t)
	now := time.Now()

	// Seed the fight with a direct hit so a fight exists, then add DoT ticks.
	tr.Handle(hitEvent("You", "a goblin", 50, now))
	tr.Handle(dotTickEvent("a goblin", 48, "Asphyxiate", now.Add(6*time.Second)))
	tr.Handle(dotTickEvent("a goblin", 48, "Asphyxiate", now.Add(12*time.Second)))

	st := tr.GetState()
	you := requireCombatant(t, st, "You")
	if you.TotalDamage != 50+48+48 {
		t.Fatalf("TotalDamage = %d, want %d", you.TotalDamage, 50+48+48)
	}
	if you.HitCount != 3 {
		t.Fatalf("HitCount = %d, want 3 (1 melee + 2 dot ticks)", you.HitCount)
	}
}

// TestRealOsuiLogTrackerIntegration streams the real Osui (Enchanter) log
// file through the parser AND the tracker end-to-end. Verifies the
// regressions reported from the Sun May 10 raid session are fixed:
//
//   - The Zlandicar fight (raid boss with a single-word capitalised
//     name, where the player only debuffed and never directly damaged
//     the mob) is now archived instead of being silently dropped.
//   - No "Eye of <PlayerName>" fights appear in archived history — the
//     magician scout-pet dismiss flood is filtered out.
//   - The 0-outgoing-damage filter is doing its job — archived fights
//     all have outgoing total > 0.
//
// Uses a HistoryStore (unbounded) rather than the in-memory ring buffer
// because the fixture covers a full raid night and the 20-fight ring
// cycles past the Zlandicar entry well before the stream ends.
//
// Skipped when the gitignored testdata fixture is not present (CI).
func TestRealOsuiLogTrackerIntegration(t *testing.T) {
	path := filepath.Join("..", "..", "..", "testdata", "eqlog_Osui_pq.proj.txt")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("testdata fixture %s not present", path)
	}

	hub := ws.NewHub()
	go hub.Run()
	tr := NewTracker(hub, func() string { return "Osui" })

	storePath := filepath.Join(t.TempDir(), "user.db")
	store, err := OpenHistoryStore(storePath)
	if err != nil {
		t.Fatalf("OpenHistoryStore: %v", err)
	}
	defer store.Close()
	tr.SetHistoryStore(store)

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open log: %v", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		ev, ok := logparser.ParseLine(scanner.Text())
		if !ok {
			continue
		}
		tr.Handle(ev)
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan: %v", err)
	}

	fights, err := store.ListFights(FightFilter{Limit: 1000})
	if err != nil {
		t.Fatalf("ListFights: %v", err)
	}

	zlandicarSeen := false
	eyeSeen := 0
	zeroSeen := 0
	for _, sf := range fights {
		if sf.NPCName == "Zlandicar" {
			zlandicarSeen = true
			if sf.TotalDamage == 0 {
				t.Errorf("Zlandicar fight archived with zero damage — routing did half the job")
			}
		}
		if strings.HasPrefix(sf.NPCName, "Eye of ") {
			eyeSeen++
		}
		if sf.TotalDamage == 0 {
			zeroSeen++
		}
	}

	if !zlandicarSeen {
		t.Error("Zlandicar fight missing from persisted history — verifiedPlayers routing fix didn't take")
	}
	if eyeSeen > 0 {
		t.Errorf("expected 0 'Eye of …' archived fights, got %d", eyeSeen)
	}
	if zeroSeen > 0 {
		t.Errorf("expected 0 zero-damage archived fights, got %d", zeroSeen)
	}
}
