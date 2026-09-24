package spelltimer

import (
	"strconv"
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// spawnIDEngine builds a DB-backed engine (needed so StartExternal can
// resolve spellID to a DB spell name for keyTargetTokenLocked's comparison
// against lastCastSpell). Mirrors ownershipEngine in clicky_ownership_test.go.
func spawnIDEngine(t *testing.T) *Engine {
	t.Helper()
	database, err := db.Open("../../data/quarm.db")
	if err != nil {
		t.Skipf("quarm.db not available: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	charCtx := func() (string, string, int) { return "/eq", "Osui", -1 }
	return NewEngine(ws.NewHub(), database, charCtx,
		func() string { return scopeAnyone }, nil, nil, nil, nil, nil)
}

func intPtr(i int) *int { return &i }

const (
	shasAdvantageName = "Sha's Advantage" // BST slow (50-65%), single-target
	shasAdvantageID   = 2942
)

// settlePipeTarget drives HandlePipeTarget/SetPipeTargetID as production
// code would, backdates pipeTargetNameChangedAt/pipeTargetIDChangedAt well
// past pipeTargetSettle — simulating "the pipe has reported this exact
// (name, id) pair consistently for a while" without a real sleep — then
// re-delivers the same id so SetPipeTargetID's own settle check (and its
// observeAliveLocked side effect) sees the backdated timestamps, exactly as
// a later ~100ms pipe tick would once the pair had genuinely held that
// long. Tests that want to exercise the UNSETTLED case drive
// HandlePipeTarget/SetPipeTargetID directly instead of using this helper.
func settlePipeTarget(e *Engine, name string, id int) {
	e.HandlePipeTarget(name)
	e.SetPipeTargetID(intPtr(id))
	e.mu.Lock()
	past := time.Now().Add(-pipeTargetSettle - time.Second)
	e.pipeTargetNameChangedAt = past
	e.pipeTargetIDChangedAt = past
	e.mu.Unlock()
	e.SetPipeTargetID(intPtr(id))
}

// keyTargetTokenLocked is the core of the fix for the issue Grimrose (SoS)
// reported against the built-in Slows pack: slowing two identically-named
// mobs only showed one countdown, because the trigger-driven timer key
// namespaces purely by captured mob name (see timerKey), and the combat log
// line that name comes from carries no spawn id at all. Exercised directly
// here (rather than only through StartExternal) so each condition of the
// "confidently my own recent cast" test is isolated.
func TestKeyTargetTokenLocked(t *testing.T) {
	// ST_Target (5): a single-target spell type, matching Sha's Advantage's
	// real spells_new.targettype in quarm.db.
	singleTarget := &db.Spell{Name: shasAdvantageName, TargetType: 5}
	// ST_AECaster or similar group/AE type: one cast legitimately lands on
	// several recipients, so the caster's single live-selected target can't
	// identify any one of them.
	aeSpell := &db.Spell{Name: shasAdvantageName, TargetType: 2}
	now := time.Now()

	cases := []struct {
		name           string
		target         string
		spell          *db.Spell
		castSpell      string
		castAt         time.Time
		castTarget     int
		castTargetName string
		at             time.Time
		want           string
	}{
		{
			name:       "matching recent self-cast appends spawn id",
			target:     "a gnoll",
			spell:      singleTarget,
			castSpell:  shasAdvantageName,
			castAt:     now,
			castTarget: 101,
			at:         now.Add(time.Second),
			want:       "a gnoll#id:101",
		},
		{
			name:   "no captured target passes through untouched",
			target: "",
			spell:  singleTarget,
			want:   "",
		},
		{
			name:   "no spell (spellID <= 0 or unknown) passes through untouched",
			target: "a gnoll",
			spell:  nil,
			want:   "a gnoll",
		},
		{
			name:       "no live target id at cast time falls back to name",
			target:     "a gnoll",
			spell:      singleTarget,
			castSpell:  shasAdvantageName,
			castAt:     now,
			castTarget: 0,
			at:         now.Add(time.Second),
			want:       "a gnoll",
		},
		{
			name:       "different spell name (someone else's cast, or unrelated) falls back",
			target:     "a gnoll",
			spell:      singleTarget,
			castSpell:  "Some Other Spell",
			castAt:     now,
			castTarget: 101,
			at:         now.Add(time.Second),
			want:       "a gnoll",
		},
		{
			name:       "outside the correlation window falls back",
			target:     "a gnoll",
			spell:      singleTarget,
			castSpell:  shasAdvantageName,
			castAt:     now,
			castTarget: 101,
			at:         now.Add(castSelfSlowWindow + time.Second),
			want:       "a gnoll",
		},
		{
			name:       "landed line before the cast (stale/negative delta) falls back",
			target:     "a gnoll",
			spell:      singleTarget,
			castSpell:  shasAdvantageName,
			castAt:     now,
			castTarget: 101,
			at:         now.Add(-time.Second),
			want:       "a gnoll",
		},
		{
			name:       "group/AE spell type never gets tagged, even with a live cast id",
			target:     "a gnoll",
			spell:      aeSpell,
			castSpell:  shasAdvantageName,
			castAt:     now,
			castTarget: 101,
			at:         now.Add(time.Second),
			want:       "a gnoll",
		},
		{
			name:           "pipe-recorded cast target disagrees with the landed target falls back",
			target:         "a gnoll",
			spell:          singleTarget,
			castSpell:      shasAdvantageName,
			castAt:         now,
			castTarget:     101,
			castTargetName: "a different gnoll",
			at:             now.Add(time.Second),
			want:           "a gnoll",
		},
		{
			name:           "pipe-recorded cast target agreeing with the landed target still tags",
			target:         "a gnoll",
			spell:          singleTarget,
			castSpell:      shasAdvantageName,
			castAt:         now,
			castTarget:     101,
			castTargetName: "A Gnoll", // case/article differences normalize equal
			at:             now.Add(time.Second),
			want:           "a gnoll#id:101",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := &Engine{
				lastCastSpell:    tc.castSpell,
				lastCastAt:       tc.castAt,
				lastCastTargetID: tc.castTarget,
				lastCastTarget:   tc.castTargetName,
			}
			got := e.keyTargetTokenLocked(tc.spell, tc.target, tc.at)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// End-to-end wiring check: a real StartExternal call, fed by the same
// SetPipeTargetID + EventSpellCast path production code uses, must produce
// a timer keyed with the spawn-id suffix while leaving the displayed
// TargetName as the plain mob name.
func TestStartExternal_UsesSpawnIDInKeyNotDisplay(t *testing.T) {
	e := spawnIDEngine(t)
	now := time.Now()

	e.SetPipeTargetID(intPtr(101))
	e.Handle(logparser.LogEvent{
		Type: logparser.EventSpellCast,
		Data: logparser.SpellCastData{SpellName: shasAdvantageName},
	})
	e.StartExternal("BST slow (50-65%)", "debuff", 192, 0, now.Add(time.Second),
		nil, shasAdvantageID, "a gnoll", "#ff0000", false, "", false)

	wantKey := timerKey("BST slow (50-65%)", "a gnoll"+targetIDKeySep+"101")
	timer, ok := e.timers[wantKey]
	if !ok {
		t.Fatalf("expected timer at key %q, got keys: %v", wantKey, keysOf(e.timers))
	}
	if timer.TargetName != "a gnoll" {
		t.Errorf("displayed TargetName should stay the plain mob name, got %q", timer.TargetName)
	}
}

// Without a live target id at cast time (Zeal disconnected, an NPC-cast
// slow with no "You begin casting" line on this client, or a slow cast by
// someone else in the raid), the key falls back to the plain target name
// exactly as before this feature existed — two same-named mobs still
// collide into one row. This is the documented, unavoidable remainder of
// LIMITATIONS.md §1.3: those cases have nothing on this client to
// correlate a spawn id against.
func TestStartExternal_NoSpawnIDFallsBackToNameCollision(t *testing.T) {
	e := spawnIDEngine(t)
	now := time.Now()

	e.StartExternal("BST slow (50-65%)", "debuff", 192, 0, now,
		nil, shasAdvantageID, "a gnoll", "#ff0000", false, "", false)
	e.StartExternal("BST slow (50-65%)", "debuff", 192, 0, now.Add(time.Second),
		nil, shasAdvantageID, "a gnoll", "#ff0000", false, "", false)

	if len(e.timers) != 1 {
		t.Fatalf("expected the pre-existing name-only collision without a spawn id, got %d rows: %v",
			len(e.timers), keysOf(e.timers))
	}
}

// The default "auto" tracking pipeline (no trigger involved) now benefits
// from the same disambiguation: onSpellLanded tags its own key via
// keyTargetTokenLocked, so two identically-named mobs the player personally
// slowed in quick succession — each preceded by its own "You begin casting"
// line and live target id — get independent rows instead of one collapsing
// into the other. Before this fix, auto mode never tagged at all, which was
// the actual reason the bug was still reproducible after 789d1b33 (that fix
// only ever touched the trigger-driven StartExternal path).
func TestOnSpellLanded_AutoModeTagsBySpawnID(t *testing.T) {
	e := spawnIDEngine(t)

	cast := func(targetID int) {
		e.SetPipeTargetID(intPtr(targetID))
		e.Handle(logparser.LogEvent{
			Type: logparser.EventSpellCast,
			Data: logparser.SpellCastData{SpellName: shasAdvantageName},
		})
	}

	// Each land's timestamp must be AFTER its own cast's lastCastAt — a
	// negative delta is exactly what keyTargetTokenLocked treats as a
	// stale/impossible correlation and refuses to tag (see
	// TestKeyTargetTokenLocked's "landed line before the cast" case) — so
	// this reads time.Now() fresh after each cast rather than a single
	// pre-captured timestamp.
	cast(101)
	e.onSpellLanded(time.Now(), logparser.SpellLandedData{
		Kind:       logparser.SpellLandedKindOther,
		SpellName:  shasAdvantageName,
		TargetName: "a bloodguard",
	})
	cast(102)
	e.onSpellLanded(time.Now(), logparser.SpellLandedData{
		Kind:       logparser.SpellLandedKindOther,
		SpellName:  shasAdvantageName,
		TargetName: "a bloodguard",
	})

	if len(e.timers) != 2 {
		t.Fatalf("expected two independent auto-mode timers, got %d: %v", len(e.timers), keysOf(e.timers))
	}
	for _, id := range []string{"101", "102"} {
		key := timerKey(shasAdvantageName, "a bloodguard"+targetIDKeySep+id)
		timer, ok := e.timers[key]
		if !ok {
			t.Errorf("expected timer at key %q, got keys: %v", key, keysOf(e.timers))
			continue
		}
		if timer.TargetName != "a bloodguard" {
			t.Errorf("displayed TargetName should stay the plain mob name, got %q", timer.TargetName)
		}
	}
}

// slowKeyOn returns the composite key keyTargetTokenLocked would produce for
// a self-cast slow on "a bloodguard" landing on the given spawn id, and
// inserts a matching ActiveTimer directly into e.timers. Bypasses
// StartExternal's cross-pipeline dedup so the test stays focused on
// removeOnKill's own id-matching logic.
func slowKeyOn(e *Engine, spawnID int) string {
	key := timerKey("BST slow (50-65%)", "a bloodguard"+targetIDKeySep+strconv.Itoa(spawnID))
	e.timers[key] = &ActiveTimer{
		ID:         key,
		SpellName:  "BST slow (50-65%)",
		SpellID:    shasAdvantageID,
		TargetName: "a bloodguard",
		Category:   CategoryDebuff,
		CastAt:     time.Now(),
		StartsAt:   time.Now(),
		ExpiresAt:  time.Now().Add(time.Minute),
	}
	return key
}

// TestRemoveOnKill_SpawnIDSparesOtherInstance covers the bug Grimrose (SoS)
// reported against v0.22.0 (and again against v0.23.0, after 789d1b33 only
// partly fixed it): two identically-named mobs are both slowed by the
// player (each timer's key gets a distinct spawn-id suffix, per
// keyTargetTokenLocked), then one of them dies. removeOnKill must spare the
// still-living instance whenever it can't prove which exact spawn id died.
func TestRemoveOnKill_SpawnIDSparesOtherInstance(t *testing.T) {
	t.Run("self-kill log line, settled", func(t *testing.T) {
		e := spawnIDEngine(t)
		slowKeyOn(e, 101)
		slowKeyOn(e, 102)

		// Mob #101 dies; the pipe's last-known target is still #101 (it was
		// the player's live target when it died), and the name/id pair has
		// held long enough to be trusted.
		settlePipeTarget(e, "a bloodguard", 101)
		e.Handle(logparser.LogEvent{
			Type: logparser.EventKill,
			Data: logparser.KillData{Killer: "You", Target: "a bloodguard"},
		})

		wantSurvivor := timerKey("BST slow (50-65%)", "a bloodguard"+targetIDKeySep+"102")
		if _, ok := e.timers[wantSurvivor]; !ok {
			t.Errorf("expected surviving instance's timer at key %q, got keys: %v", wantSurvivor, keysOf(e.timers))
		}
		wantGone := timerKey("BST slow (50-65%)", "a bloodguard"+targetIDKeySep+"101")
		if _, ok := e.timers[wantGone]; ok {
			t.Errorf("expected killed instance's timer at key %q to be removed", wantGone)
		}
		if len(e.timers) != 1 {
			t.Errorf("expected exactly 1 surviving timer, got %d: %v", len(e.timers), keysOf(e.timers))
		}
	})

	t.Run("corpse-target signal, settled", func(t *testing.T) {
		e := spawnIDEngine(t)
		slowKeyOn(e, 201)
		slowKeyOn(e, 202)

		// The corpse pulse arrives before name/id have settled together, so
		// it only queues an ambiguous pendingKill rather than guessing
		// (HandlePipeTarget only re-fires removeOnKill on a NAME
		// transition, so it can't itself retry once the pair settles).
		e.HandlePipeTarget("a bloodguard's corpse")
		e.SetPipeTargetID(intPtr(201))

		// A poll cycle later the log's own "You have slain X!" line for the
		// same death arrives — by then the pair has held long enough to
		// settle, so this resolves it exactly.
		e.mu.Lock()
		past := time.Now().Add(-pipeTargetSettle - time.Second)
		e.pipeTargetNameChangedAt = past
		e.pipeTargetIDChangedAt = past
		e.mu.Unlock()
		e.Handle(logparser.LogEvent{
			Type: logparser.EventKill,
			Data: logparser.KillData{Killer: "You", Target: "a bloodguard"},
		})

		wantSurvivor := timerKey("BST slow (50-65%)", "a bloodguard"+targetIDKeySep+"202")
		if _, ok := e.timers[wantSurvivor]; !ok {
			t.Errorf("expected surviving instance's timer at key %q, got keys: %v", wantSurvivor, keysOf(e.timers))
		}
		if len(e.timers) != 1 {
			t.Errorf("expected exactly 1 surviving timer, got %d: %v", len(e.timers), keysOf(e.timers))
		}
	})

	t.Run("kill credited to someone else leaves both instances untouched immediately", func(t *testing.T) {
		// lastPipeTargetID reflects only the active character's own target,
		// so it must not be trusted to disambiguate a kill some other raid
		// member landed. Earlier versions of this fix still wiped both
		// same-named timers here (the actual regression this test guards
		// against) — now neither is touched until resolvePendingKillsLocked
		// gets a chance to weigh in (see the pendingKills tests below).
		e := spawnIDEngine(t)
		slowKeyOn(e, 301)
		slowKeyOn(e, 302)

		settlePipeTarget(e, "a bloodguard", 999) // player's OWN target, unrelated to either instance
		e.Handle(logparser.LogEvent{
			Type: logparser.EventKill,
			Data: logparser.KillData{Killer: "Groupmate", Target: "a bloodguard"},
		})

		if len(e.timers) != 2 {
			t.Errorf("expected both same-named timers to survive an untrusted kill, got %d: %v",
				len(e.timers), keysOf(e.timers))
		}
	})

	t.Run("unsettled name/id pair is not trusted even for a self-kill", func(t *testing.T) {
		// Simulates the stale-pairing race: the pipe's target name and id
		// changed only moments ago (e.g. the corpse name arrived a tick
		// ahead of the id update), so they haven't settled together —
		// trusting lastPipeTargetID here risks crediting the wrong
		// still-living instance as dead.
		e := spawnIDEngine(t)
		slowKeyOn(e, 401)
		slowKeyOn(e, 402)

		e.HandlePipeTarget("a bloodguard")
		e.SetPipeTargetID(intPtr(401)) // both changed "just now" — unsettled
		e.Handle(logparser.LogEvent{
			Type: logparser.EventKill,
			Data: logparser.KillData{Killer: "You", Target: "a bloodguard"},
		})

		if len(e.timers) != 2 {
			t.Errorf("expected both instances to survive while the pipe pair is unsettled, got %d: %v",
				len(e.timers), keysOf(e.timers))
		}
	})
}

// TestResolvePendingKillsLocked_FlagsMaybeDeadAfterGrace covers what
// happens to the ambiguous survivors from the "kill credited to someone
// else" case above once the grace period elapses with no corroborating
// exact-id evidence: the engine marks them MaybeDead ("a same-named mob
// died; this might be the one") rather than either deleting them or leaving
// them silently unmarked forever.
func TestResolvePendingKillsLocked_FlagsMaybeDeadAfterGrace(t *testing.T) {
	e := spawnIDEngine(t)
	slowKeyOn(e, 501)
	slowKeyOn(e, 502)

	settlePipeTarget(e, "a bloodguard", 999) // some unrelated live target
	e.Handle(logparser.LogEvent{
		Type: logparser.EventKill,
		Data: logparser.KillData{Killer: "Groupmate", Target: "a bloodguard"},
	})

	e.mu.Lock()
	if len(e.pendingKills) != 1 {
		t.Fatalf("expected one pendingKill queued, got %d", len(e.pendingKills))
	}
	// Force the grace period to have elapsed without touching real time.
	e.resolvePendingKillsLocked(time.Now().Add(killEvidenceGrace + time.Second))
	e.mu.Unlock()

	key501 := timerKey("BST slow (50-65%)", "a bloodguard"+targetIDKeySep+"501")
	key502 := timerKey("BST slow (50-65%)", "a bloodguard"+targetIDKeySep+"502")
	if !e.timers[key501].MaybeDead {
		t.Error("expected #501 flagged MaybeDead")
	}
	if !e.timers[key502].MaybeDead {
		t.Error("expected #502 flagged MaybeDead")
	}
	if len(e.timers) != 2 {
		t.Errorf("resolving must never delete an ambiguous instance, got %d timers: %v", len(e.timers), keysOf(e.timers))
	}
}

// An instance the player is actively watching alive around the time of the
// kill must not be flagged, even though it shares the dead mob's name.
func TestResolvePendingKillsLocked_SparesAnInstanceSeenAliveAroundTheKill(t *testing.T) {
	e := spawnIDEngine(t)
	slowKeyOn(e, 601)
	slowKeyOn(e, 602)

	// Player is watching #602 alive right as the kill line for the OTHER
	// instance (credited to a groupmate) comes in.
	settlePipeTarget(e, "a bloodguard", 602)
	e.Handle(logparser.LogEvent{
		Type: logparser.EventKill,
		Data: logparser.KillData{Killer: "Groupmate", Target: "a bloodguard"},
	})

	e.mu.Lock()
	e.resolvePendingKillsLocked(time.Now().Add(killEvidenceGrace + time.Second))
	e.mu.Unlock()

	key601 := timerKey("BST slow (50-65%)", "a bloodguard"+targetIDKeySep+"601")
	key602 := timerKey("BST slow (50-65%)", "a bloodguard"+targetIDKeySep+"602")
	if !e.timers[key601].MaybeDead {
		t.Error("expected #601 (never seen alive) flagged MaybeDead")
	}
	if e.timers[key602].MaybeDead {
		t.Error("expected #602 (seen alive around the kill) NOT flagged")
	}
}

// Retargeting an already-flagged instance and seeing it's still alive
// clears the flag — the "?" marker is meant to resolve itself the moment
// better evidence arrives, not linger until the timer's natural expiry.
func TestObserveAliveLocked_ClearsAnExistingMaybeDeadFlag(t *testing.T) {
	e := spawnIDEngine(t)
	key := slowKeyOn(e, 701)
	e.timers[key].MaybeDead = true

	settlePipeTarget(e, "a bloodguard", 701)

	if e.timers[key].MaybeDead {
		t.Error("expected MaybeDead cleared once the pipe confirmed this exact instance alive")
	}
}
