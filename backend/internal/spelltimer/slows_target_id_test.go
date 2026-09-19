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
	shasAdvantageName = "Sha's Advantage" // BST slow (50-65%)
	shasAdvantageID   = 2942
)

// keyTargetTokenLocked is the core of the fix for the issue Grimrose (SoS)
// reported against the built-in Slows pack: slowing two identically-named
// mobs only showed one countdown, because the trigger-driven timer key
// namespaces purely by captured mob name (see timerKey), and the combat log
// line that name comes from carries no spawn id at all. Exercised directly
// here (rather than only through StartExternal) so each condition of the
// "confidently my own recent cast" test is isolated.
func TestKeyTargetTokenLocked(t *testing.T) {
	spell := &db.Spell{Name: shasAdvantageName}
	now := time.Now()

	cases := []struct {
		name       string
		target     string
		spell      *db.Spell
		castSpell  string
		castAt     time.Time
		castTarget int
		at         time.Time
		want       string
	}{
		{
			name:       "matching recent self-cast appends spawn id",
			target:     "a gnoll",
			spell:      spell,
			castSpell:  shasAdvantageName,
			castAt:     now,
			castTarget: 101,
			at:         now.Add(time.Second),
			want:       "a gnoll#id:101",
		},
		{
			name:   "no captured target passes through untouched",
			target: "",
			spell:  spell,
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
			spell:      spell,
			castSpell:  shasAdvantageName,
			castAt:     now,
			castTarget: 0,
			at:         now.Add(time.Second),
			want:       "a gnoll",
		},
		{
			name:       "different spell name (someone else's cast, or unrelated) falls back",
			target:     "a gnoll",
			spell:      spell,
			castSpell:  "Some Other Spell",
			castAt:     now,
			castTarget: 101,
			at:         now.Add(time.Second),
			want:       "a gnoll",
		},
		{
			name:       "outside the correlation window falls back",
			target:     "a gnoll",
			spell:      spell,
			castSpell:  shasAdvantageName,
			castAt:     now,
			castTarget: 101,
			at:         now.Add(castSelfSlowWindow + time.Second),
			want:       "a gnoll",
		},
		{
			name:       "landed line before the cast (stale/negative delta) falls back",
			target:     "a gnoll",
			spell:      spell,
			castSpell:  shasAdvantageName,
			castAt:     now,
			castTarget: 101,
			at:         now.Add(-time.Second),
			want:       "a gnoll",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := &Engine{
				lastCastSpell:    tc.castSpell,
				lastCastAt:       tc.castAt,
				lastCastTargetID: tc.castTarget,
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

// slowKeyOn returns the composite key keyTargetTokenLocked would produce for
// a self-cast slow on "a bloodguard" landing on the given spawn id, and
// inserts a matching ActiveTimer directly into e.timers. Bypasses
// StartExternal's cross-pipeline dedup (sameSpellForDedup, which is keyed on
// spell identity alone and would otherwise collapse two same-spell timers
// started within dedupGraceWindow of each other regardless of target — a
// real but separate latent gap, not what this test exercises) so the test
// stays focused on removeOnKill's own id-matching logic.
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
// reported against v0.22.0: two identically-named mobs are both slowed by
// the player (each timer's key gets a distinct spawn-id suffix, per
// keyTargetTokenLocked), then one of them dies. Before this fix, removeOnKill
// matched purely on TargetName and deleted BOTH timers; it must now spare the
// still-living instance whenever the known-dead spawn id disagrees with a
// timer's own key.
func TestRemoveOnKill_SpawnIDSparesOtherInstance(t *testing.T) {
	t.Run("self-kill log line", func(t *testing.T) {
		e := spawnIDEngine(t)
		slowKeyOn(e, 101)
		slowKeyOn(e, 102)

		// Mob #101 dies; the pipe's last-known target is still #101 (it was
		// the player's live target when it died) at the moment the log's
		// self-kill line reaches EventKill.
		e.SetPipeTargetID(intPtr(101))
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

	t.Run("corpse-target signal", func(t *testing.T) {
		e := spawnIDEngine(t)
		slowKeyOn(e, 201)
		slowKeyOn(e, 202)

		e.SetPipeTargetID(intPtr(201))
		e.HandlePipeTarget("a bloodguard's corpse")

		wantSurvivor := timerKey("BST slow (50-65%)", "a bloodguard"+targetIDKeySep+"202")
		if _, ok := e.timers[wantSurvivor]; !ok {
			t.Errorf("expected surviving instance's timer at key %q, got keys: %v", wantSurvivor, keysOf(e.timers))
		}
		if len(e.timers) != 1 {
			t.Errorf("expected exactly 1 surviving timer, got %d: %v", len(e.timers), keysOf(e.timers))
		}
	})

	t.Run("kill credited to someone else keeps the pre-existing name collision", func(t *testing.T) {
		// lastPipeTargetID reflects only the active character's own target,
		// so it must not be trusted to disambiguate a kill some other raid
		// member landed — this remains the documented LIMITATIONS.md §1.3
		// remainder, unchanged by this fix.
		e := spawnIDEngine(t)
		slowKeyOn(e, 301)
		slowKeyOn(e, 302)

		e.SetPipeTargetID(intPtr(301))
		e.Handle(logparser.LogEvent{
			Type: logparser.EventKill,
			Data: logparser.KillData{Killer: "Groupmate", Target: "a bloodguard"},
		})

		if len(e.timers) != 0 {
			t.Errorf("expected both same-named timers cleared without trustworthy spawn-id evidence, got %d: %v",
				len(e.timers), keysOf(e.timers))
		}
	})
}
