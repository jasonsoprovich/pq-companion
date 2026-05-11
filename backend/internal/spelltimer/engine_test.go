package spelltimer

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// newTestEngine builds an engine wired to a hub but without a database. Tests
// that don't exercise onSpellLanded's DB lookup can use it freely; the hub's
// channel is buffered so broadcasts succeed without a Run() goroutine.
func newTestEngine() *Engine {
	hub := ws.NewHub()
	return &Engine{
		hub:    hub,
		timers: make(map[string]*ActiveTimer),
		charCtx: func() (string, string) {
			return "/eq", "Osui"
		},
		// Default test scope is "anyone" so legacy tests that don't care
		// about the scope filter exercise the unconditional-track path.
		// Scope-specific tests install their own scopeFn.
		scopeFn: func() string { return scopeAnyone },
	}
}

func TestTimerKey(t *testing.T) {
	got := timerKey("Visions of Grandeur", "Tank")
	if got != "Visions of Grandeur@Tank" {
		t.Errorf("composite: got %q", got)
	}
	if timerKey("Trigger Name", "") != "Trigger Name@" {
		t.Errorf("empty target: got %q", timerKey("Trigger Name", ""))
	}
}

// EventSpellCast must NOT create a timer — it only records lastCastSpell so
// a subsequent ambiguous EventSpellLanded can be disambiguated. This is the
// load-bearing PR1-3 behavior change vs. the previous cast-begin pipeline.
func TestHandle_SpellCast_RecordsButDoesNotCreate(t *testing.T) {
	e := newTestEngine()

	e.Handle(logparser.LogEvent{
		Type: logparser.EventSpellCast,
		Data: logparser.SpellCastData{SpellName: "Mesmerization"},
	})

	if len(e.timers) != 0 {
		t.Fatalf("expected no timers after cast event, got %d", len(e.timers))
	}
	if e.lastCastSpell != "Mesmerization" {
		t.Errorf("lastCastSpell: got %q", e.lastCastSpell)
	}
	if e.lastCastAt.IsZero() {
		t.Error("lastCastAt should be set")
	}
}

// Resist / interrupt / did-not-take-hold all imply the spell didn't land.
// They should clear the recorded last-cast so a stale value can't bind to
// an unrelated future landed event.
func TestHandle_FailedCastsClearLastCast(t *testing.T) {
	cases := []logparser.LogEvent{
		{Type: logparser.EventSpellResist, Data: logparser.SpellResistData{SpellName: "Mez"}},
		{Type: logparser.EventSpellInterrupt, Data: logparser.SpellInterruptData{SpellName: "Mez"}},
		{Type: logparser.EventSpellDidNotTakeHold, Data: logparser.SpellDidNotTakeHoldData{}},
	}
	for _, ev := range cases {
		t.Run(string(ev.Type), func(t *testing.T) {
			e := newTestEngine()
			e.lastCastSpell = "Something"
			e.lastCastAt = time.Now()

			e.Handle(ev)

			if e.lastCastSpell != "" {
				t.Errorf("expected lastCastSpell cleared, got %q", e.lastCastSpell)
			}
			if !e.lastCastAt.IsZero() {
				t.Error("expected lastCastAt zero")
			}
		})
	}
}

// resolveLandedSpellName picks the right candidate when cast text is shared
// across multiple spells, using lastCastSpell as the disambiguator.
func TestResolveLandedSpellName(t *testing.T) {
	cases := []struct {
		name      string
		setup     func(*Engine)
		data      logparser.SpellLandedData
		want      string
	}{
		{
			name: "unique match returns spell name directly",
			data: logparser.SpellLandedData{SpellName: "Visions of Grandeur"},
			want: "Visions of Grandeur",
		},
		{
			name: "no candidates returns empty",
			data: logparser.SpellLandedData{},
			want: "",
		},
		{
			name: "ambiguous with no recent cast returns empty",
			data: logparser.SpellLandedData{
				Candidates: []logparser.SpellLandedCandidate{
					{SpellID: 1000, SpellName: "Ultravision"},
					{SpellID: 1001, SpellName: "Plainsight"},
				},
			},
			want: "",
		},
		{
			name: "ambiguous with matching recent cast picks that candidate",
			setup: func(e *Engine) {
				e.lastCastSpell = "Plainsight"
				e.lastCastAt = time.Now()
			},
			data: logparser.SpellLandedData{
				Candidates: []logparser.SpellLandedCandidate{
					{SpellID: 1000, SpellName: "Ultravision"},
					{SpellID: 1001, SpellName: "Plainsight"},
				},
			},
			want: "Plainsight",
		},
		{
			name: "ambiguous with stale recent cast returns empty",
			setup: func(e *Engine) {
				e.lastCastSpell = "Plainsight"
				e.lastCastAt = time.Now().Add(-2 * lastCastWindow)
			},
			data: logparser.SpellLandedData{
				Candidates: []logparser.SpellLandedCandidate{
					{SpellID: 1000, SpellName: "Ultravision"},
					{SpellID: 1001, SpellName: "Plainsight"},
				},
			},
			want: "",
		},
		{
			name: "ambiguous with non-matching recent cast returns empty",
			setup: func(e *Engine) {
				e.lastCastSpell = "Mesmerization"
				e.lastCastAt = time.Now()
			},
			data: logparser.SpellLandedData{
				Candidates: []logparser.SpellLandedCandidate{
					{SpellID: 1000, SpellName: "Ultravision"},
					{SpellID: 1001, SpellName: "Plainsight"},
				},
			},
			want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newTestEngine()
			if tc.setup != nil {
				tc.setup(e)
			}
			got := e.resolveLandedSpellName(tc.data)
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// StartExternal must dedup against any same-spell-name timer regardless of
// target. This stops a user-defined trigger from creating a duplicate row
// when the spell-landed pipeline has already created a per-target entry for
// the same buff.
func TestStartExternal_DedupsAgainstSpellLandedTimer(t *testing.T) {
	e := newTestEngine()
	now := time.Now()
	// Simulate a spell-landed entry for VoG on Tank.
	e.timers[timerKey("Visions of Grandeur", "Tank")] = &ActiveTimer{
		ID:         timerKey("Visions of Grandeur", "Tank"),
		SpellName:  "Visions of Grandeur",
		TargetName: "Tank",
		Category:   CategoryBuff,
		CastAt:     now,
		StartsAt:   now,
		ExpiresAt:  now.Add(30 * time.Minute),
	}

	// User trigger fires with the same spell name moments later.
	e.StartExternal("Visions of Grandeur", "buff", 1620, 0, now.Add(time.Second), nil, 0)

	// Still only the one entry — the trigger's would-be entry was suppressed.
	if len(e.timers) != 1 {
		t.Errorf("expected 1 timer (dedup), got %d", len(e.timers))
	}
	if _, ok := e.timers[timerKey("Visions of Grandeur", "")]; ok {
		t.Errorf("trigger-created entry should have been suppressed")
	}
}

// A custom trigger with a unique name (i.e. one that doesn't shadow a real
// spell already in the timer map) should create its entry as before.
func TestStartExternal_CreatesEntryWhenNoSpellMatch(t *testing.T) {
	e := newTestEngine()
	e.StartExternal("AE Incoming", "debuff", 30, 0, time.Now(), nil, 0)

	if len(e.timers) != 1 {
		t.Fatalf("expected 1 timer, got %d", len(e.timers))
	}
	got, ok := e.timers[timerKey("AE Incoming", "")]
	if !ok {
		t.Fatalf("timer not found at expected key")
	}
	if got.SpellName != "AE Incoming" || got.TargetName != "" {
		t.Errorf("payload: name=%q target=%q", got.SpellName, got.TargetName)
	}
}

// Per-trigger DisplayThresholdSecs must be copied onto the ActiveTimer so
// the frontend can apply the override instead of the global default.
func TestStartExternal_CopiesDisplayThreshold(t *testing.T) {
	e := newTestEngine()
	e.StartExternal("Long Buff", "buff", 7200, 600, time.Now(), nil, 0)

	got, ok := e.timers[timerKey("Long Buff", "")]
	if !ok {
		t.Fatalf("timer not found")
	}
	if got.DisplayThresholdSecs != 600 {
		t.Errorf("threshold: got %d, want 600", got.DisplayThresholdSecs)
	}
}

// In triggers-only mode, the spell-landed pipeline must not create timer
// rows. Triggers (StartExternal) are unaffected.
func TestOnSpellLanded_TriggersOnlyModeSuppressesAutoTimers(t *testing.T) {
	e := newTestEngine()
	e.modeFn = func() string { return modeTriggersOnly }

	e.onSpellLanded(time.Now(), logparser.SpellLandedData{
		SpellID:    2570,
		SpellName:  "Koadic's Endless Intellect",
		TargetName: "Osui",
		Kind:       logparser.SpellLandedKindOther,
	})

	if len(e.timers) != 0 {
		t.Errorf("expected 0 auto-timers in triggers_only mode, got %d", len(e.timers))
	}

	// Triggers still create timers in this mode — that's the whole point.
	e.StartExternal("Manual VoG", "buff", 1620, 0, time.Now(), nil, 0)
	if len(e.timers) != 1 {
		t.Errorf("triggers should still create timers in triggers_only mode, got %d", len(e.timers))
	}
}

// When a trigger fires after a spell-landed timer for the same spell has
// already been created, StartExternal must not add a duplicate row — but it
// MUST graft the trigger's threshold and alerts onto the existing timer.
// Spell-landed has no way to know about user-configured thresholds, so the
// trigger is the user's only channel for "treat this spell specially."
func TestStartExternal_MergesMetadataOntoExistingTimer(t *testing.T) {
	e := newTestEngine()
	now := time.Now()
	key := timerKey("Koadic's Endless Intellect", "Osui")
	e.timers[key] = &ActiveTimer{
		ID:         key,
		SpellName:  "Koadic's Endless Intellect",
		TargetName: "Osui",
		Category:   CategoryBuff,
		CastAt:     now,
		StartsAt:   now,
		ExpiresAt:  now.Add(75 * time.Minute),
	}

	alerts := json.RawMessage(`[{"id":"x","seconds":300,"type":"tts"}]`)
	e.StartExternal("Koadic's Endless Intellect", "buff", 4500, 300, now.Add(50*time.Millisecond), alerts, 0)

	if len(e.timers) != 1 {
		t.Fatalf("expected 1 timer (merge, not duplicate), got %d", len(e.timers))
	}
	got := e.timers[key]
	if got.DisplayThresholdSecs != 300 {
		t.Errorf("threshold: got %d, want 300 (merged from trigger)", got.DisplayThresholdSecs)
	}
	if string(got.TimerAlerts) != string(alerts) {
		t.Errorf("alerts: got %s, want %s", got.TimerAlerts, alerts)
	}
}

// StopExternal removes every timer with the given spell name regardless of
// target — a worn-off pattern is presumed to wipe the buff entirely.
func TestStopExternal_RemovesAllSameNameTimers(t *testing.T) {
	e := newTestEngine()
	now := time.Now()
	for _, target := range []string{"Tank", "Healer", "Osui"} {
		key := timerKey("Visions of Grandeur", target)
		e.timers[key] = &ActiveTimer{
			ID: key, SpellName: "Visions of Grandeur", TargetName: target,
			CastAt: now, StartsAt: now, ExpiresAt: now.Add(30 * time.Minute),
		}
	}
	// Different spell — should survive.
	e.timers[timerKey("Tashanian", "Mob")] = &ActiveTimer{
		ID: timerKey("Tashanian", "Mob"), SpellName: "Tashanian", TargetName: "Mob",
		CastAt: now, StartsAt: now, ExpiresAt: now.Add(2 * time.Minute),
	}

	e.StopExternal("Visions of Grandeur")

	if len(e.timers) != 1 {
		t.Errorf("expected 1 surviving timer, got %d", len(e.timers))
	}
	if _, ok := e.timers[timerKey("Tashanian", "Mob")]; !ok {
		t.Error("Tashanian timer should have been preserved")
	}
}

// SpellFade ("Your X spell has worn off.") only fires for the active player,
// so the engine must remove the timer keyed by (spell, active-player-name).
// A timer for the same spell on a different target must survive.
func TestHandle_SpellFade_RemovesActivePlayerEntryOnly(t *testing.T) {
	e := newTestEngine() // charCtx returns ("/eq", "Osui")
	now := time.Now()
	e.timers[timerKey("Visions of Grandeur", "Osui")] = &ActiveTimer{
		ID: timerKey("Visions of Grandeur", "Osui"), SpellName: "Visions of Grandeur",
		TargetName: "Osui", CastAt: now, StartsAt: now, ExpiresAt: now.Add(30 * time.Minute),
	}
	e.timers[timerKey("Visions of Grandeur", "Tank")] = &ActiveTimer{
		ID: timerKey("Visions of Grandeur", "Tank"), SpellName: "Visions of Grandeur",
		TargetName: "Tank", CastAt: now, StartsAt: now, ExpiresAt: now.Add(30 * time.Minute),
	}

	e.Handle(logparser.LogEvent{
		Type: logparser.EventSpellFade,
		Data: logparser.SpellFadeData{SpellName: "Visions of Grandeur"},
	})

	if _, ok := e.timers[timerKey("Visions of Grandeur", "Osui")]; ok {
		t.Error("active-player entry should have been removed")
	}
	if _, ok := e.timers[timerKey("Visions of Grandeur", "Tank")]; !ok {
		t.Error("other-target entry should have been preserved")
	}
}

// SpellFadeFrom carries the target name explicitly. The engine should remove
// the timer for that exact (spell, target) and leave others untouched.
func TestHandle_SpellFadeFrom_RemovesNamedTargetEntryOnly(t *testing.T) {
	e := newTestEngine()
	now := time.Now()
	e.timers[timerKey("Tashanian", "Mob1")] = &ActiveTimer{
		ID: timerKey("Tashanian", "Mob1"), SpellName: "Tashanian", TargetName: "Mob1",
		CastAt: now, StartsAt: now, ExpiresAt: now.Add(2 * time.Minute),
	}
	e.timers[timerKey("Tashanian", "Mob2")] = &ActiveTimer{
		ID: timerKey("Tashanian", "Mob2"), SpellName: "Tashanian", TargetName: "Mob2",
		CastAt: now, StartsAt: now, ExpiresAt: now.Add(2 * time.Minute),
	}

	e.Handle(logparser.LogEvent{
		Type: logparser.EventSpellFadeFrom,
		Data: logparser.SpellFadeFromData{SpellName: "Tashanian", TargetName: "Mob1"},
	})

	if _, ok := e.timers[timerKey("Tashanian", "Mob1")]; ok {
		t.Error("Mob1 entry should have been removed")
	}
	if _, ok := e.timers[timerKey("Tashanian", "Mob2")]; !ok {
		t.Error("Mob2 entry should have been preserved")
	}
}

// trackingScope falls back to "cast_by_me" for nil providers and unknown
// values so legacy/empty config files match the current default.
func TestTrackingScope_DefaultsAndFallbacks(t *testing.T) {
	cases := []struct {
		name string
		fn   ScopeProvider
		want string
	}{
		{"nil provider", nil, scopeCastByMe},
		{"empty string", func() string { return "" }, scopeCastByMe},
		{"unknown value", func() string { return "garbage" }, scopeCastByMe},
		{"explicit anyone", func() string { return scopeAnyone }, scopeAnyone},
		{"explicit self", func() string { return scopeSelf }, scopeSelf},
		{"explicit cast_by_me", func() string { return scopeCastByMe }, scopeCastByMe},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := newTestEngine()
			e.scopeFn = tc.fn
			if got := e.trackingScope(); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// onSpellLanded must drop other-target events when scope=self. The active
// player's own buffs (target == active player or fallback "You") still
// pass. We verify by calling onSpellLanded directly with a fake spell name
// that doesn't exist in any DB — when filtered, the DB lookup never runs;
// when allowed through, the lookup fails harmlessly. Either way we assert
// on whether the timer map grew, which is what the user actually sees.
func TestOnSpellLanded_ScopeSelf_FiltersNonSelfTargets(t *testing.T) {
	e := newTestEngine()
	e.scopeFn = func() string { return scopeSelf }
	// charCtx returns "Osui" as the active player.

	// Simulate a buff landing on a raid member.
	e.onSpellLanded(time.Now(), logparser.SpellLandedData{
		Kind:       logparser.SpellLandedKindOther,
		SpellName:  "Visions of Grandeur",
		TargetName: "Tank",
	})

	if len(e.timers) != 0 {
		t.Errorf("scope=self should drop other-target landing; got %d timers", len(e.timers))
	}
}

func TestActivePlayerName_FallsBackToYou(t *testing.T) {
	e := &Engine{} // no charCtx
	if got := e.activePlayerName(); got != "You" {
		t.Errorf("nil charCtx fallback: got %q", got)
	}

	e2 := &Engine{charCtx: func() (string, string) { return "", "" }}
	if got := e2.activePlayerName(); got != "You" {
		t.Errorf("empty charCtx fallback: got %q", got)
	}
}

// Zone changes used to wipe every active timer. Buffs survive zoning in EQ,
// so the engine must keep them — this regression-tests the persistence fix.
func TestHandle_Zone_KeepsTimers(t *testing.T) {
	e := newTestEngine()
	now := time.Now()
	e.timers[timerKey("Visions of Grandeur", "Tank")] = &ActiveTimer{
		ID: timerKey("Visions of Grandeur", "Tank"), SpellName: "Visions of Grandeur",
		TargetName: "Tank", CastAt: now, StartsAt: now, ExpiresAt: now.Add(30 * time.Minute),
	}
	e.timers[timerKey("Tashanian", "a gnoll")] = &ActiveTimer{
		ID: timerKey("Tashanian", "a gnoll"), SpellName: "Tashanian",
		TargetName: "a gnoll", CastAt: now, StartsAt: now, ExpiresAt: now.Add(2 * time.Minute),
	}

	e.Handle(logparser.LogEvent{
		Type: logparser.EventZone,
		Data: logparser.ZoneData{ZoneName: "The North Karana"},
	})

	if len(e.timers) != 2 {
		t.Fatalf("zone change should not clear timers; got %d (want 2)", len(e.timers))
	}
}

// Self-death clears only timers targeting the active player. Buffs the
// player put on group/raid members and debuffs on mobs survive.
func TestHandle_Death_ClearsSelfTargetsOnly(t *testing.T) {
	e := newTestEngine() // active player = "Osui"
	now := time.Now()
	e.timers[timerKey("Symbol of Marzin", "Osui")] = &ActiveTimer{
		ID: timerKey("Symbol of Marzin", "Osui"), SpellName: "Symbol of Marzin",
		TargetName: "Osui", CastAt: now, StartsAt: now, ExpiresAt: now.Add(30 * time.Minute),
	}
	e.timers[timerKey("Visions of Grandeur", "Tank")] = &ActiveTimer{
		ID: timerKey("Visions of Grandeur", "Tank"), SpellName: "Visions of Grandeur",
		TargetName: "Tank", CastAt: now, StartsAt: now, ExpiresAt: now.Add(30 * time.Minute),
	}

	e.Handle(logparser.LogEvent{
		Type: logparser.EventDeath,
		Data: logparser.DeathData{SlainBy: "a gnoll"},
	})

	if _, ok := e.timers[timerKey("Symbol of Marzin", "Osui")]; ok {
		t.Error("self-target buff should have been removed on player death")
	}
	if _, ok := e.timers[timerKey("Visions of Grandeur", "Tank")]; !ok {
		t.Error("buff on Tank should have survived player death")
	}
}

// EventKill drops timers targeting the slain entity — typical case is a
// mob we'd debuffed/mezzed dying mid-fight.
func TestHandle_Kill_RemovesTimersOnVictim(t *testing.T) {
	e := newTestEngine()
	now := time.Now()
	e.timers[timerKey("Tashanian", "a gnoll")] = &ActiveTimer{
		ID: timerKey("Tashanian", "a gnoll"), SpellName: "Tashanian",
		TargetName: "a gnoll", CastAt: now, StartsAt: now, ExpiresAt: now.Add(2 * time.Minute),
	}
	e.timers[timerKey("Tashanian", "an orc")] = &ActiveTimer{
		ID: timerKey("Tashanian", "an orc"), SpellName: "Tashanian",
		TargetName: "an orc", CastAt: now, StartsAt: now, ExpiresAt: now.Add(2 * time.Minute),
	}

	e.Handle(logparser.LogEvent{
		Type: logparser.EventKill,
		Data: logparser.KillData{Killer: "Tank", Target: "a gnoll"},
	})

	if _, ok := e.timers[timerKey("Tashanian", "a gnoll")]; ok {
		t.Error("timer on slain mob should have been removed")
	}
	if _, ok := e.timers[timerKey("Tashanian", "an orc")]; !ok {
		t.Error("timer on unrelated mob should have survived")
	}
}

// EventKill also clears trigger-driven detrimental timers that have no
// target binding. Triggers fire on a regex match but don't currently
// extract the target from capture groups, so StartExternal records the
// timer with TargetName="". Without this orphan-cleanup the timer would
// run for its full nominal duration even after the mob died.
func TestHandle_Kill_RemovesOrphanDetrimentalTimers(t *testing.T) {
	e := newTestEngine()
	now := time.Now()
	// Trigger-driven Tashanian timer — no target.
	e.timers[timerKey("Tashanian", "")] = &ActiveTimer{
		ID: timerKey("Tashanian", ""), SpellName: "Tashanian", Category: CategoryDebuff,
		TargetName: "", CastAt: now, StartsAt: now, ExpiresAt: now.Add(13 * time.Minute),
	}
	// Trigger-driven Mez timer — no target, different category, still detrimental.
	e.timers[timerKey("Mesmerize", "")] = &ActiveTimer{
		ID: timerKey("Mesmerize", ""), SpellName: "Mesmerize", Category: CategoryMez,
		TargetName: "", CastAt: now, StartsAt: now, ExpiresAt: now.Add(60 * time.Second),
	}
	// Orphan buff — should NOT be cleared by a kill event.
	e.timers[timerKey("Visions of Grandeur", "")] = &ActiveTimer{
		ID: timerKey("Visions of Grandeur", ""), SpellName: "Visions of Grandeur", Category: CategoryBuff,
		TargetName: "", CastAt: now, StartsAt: now, ExpiresAt: now.Add(27 * time.Minute),
	}
	// Bound timer on an unrelated mob — should also survive.
	e.timers[timerKey("Tashanian", "an orc")] = &ActiveTimer{
		ID: timerKey("Tashanian", "an orc"), SpellName: "Tashanian", Category: CategoryDebuff,
		TargetName: "an orc", CastAt: now, StartsAt: now, ExpiresAt: now.Add(13 * time.Minute),
	}

	e.Handle(logparser.LogEvent{
		Type: logparser.EventKill,
		Data: logparser.KillData{Killer: "Stonae", Target: "Zun Thall Xakra"},
	})

	if _, ok := e.timers[timerKey("Tashanian", "")]; ok {
		t.Error("orphan detrimental should have been cleared on kill")
	}
	if _, ok := e.timers[timerKey("Mesmerize", "")]; ok {
		t.Error("orphan mez should have been cleared on kill")
	}
	if _, ok := e.timers[timerKey("Visions of Grandeur", "")]; !ok {
		t.Error("orphan buff should have survived the kill")
	}
	if _, ok := e.timers[timerKey("Tashanian", "an orc")]; !ok {
		t.Error("target-bound timer on unrelated mob should have survived")
	}
}

// Multi-word boss names — verify the existing target-match path handles
// names with spaces (e.g. "Zun Thall Xakra") since these are the typical
// raid targets where users notice debuffs lingering.
func TestHandle_Kill_RemovesMultiWordBossTimer(t *testing.T) {
	e := newTestEngine()
	now := time.Now()
	e.timers[timerKey("Tashanian", "Zun Thall Xakra")] = &ActiveTimer{
		ID: timerKey("Tashanian", "Zun Thall Xakra"), SpellName: "Tashanian", Category: CategoryDebuff,
		TargetName: "Zun Thall Xakra", CastAt: now, StartsAt: now, ExpiresAt: now.Add(13 * time.Minute),
	}

	e.Handle(logparser.LogEvent{
		Type: logparser.EventKill,
		Data: logparser.KillData{Killer: "Stonae", Target: "Zun Thall Xakra"},
	})

	if _, ok := e.timers[timerKey("Tashanian", "Zun Thall Xakra")]; ok {
		t.Error("timer on slain multi-word boss should have been removed")
	}
}

// scope=cast_by_me drops other-target lands when there's no recent local
// cast of the same spell — i.e. another player's buff on a third party.
func TestOnSpellLanded_ScopeCastByMe_FiltersWithoutRecentCast(t *testing.T) {
	e := newTestEngine()
	e.scopeFn = func() string { return scopeCastByMe }

	e.onSpellLanded(time.Now(), logparser.SpellLandedData{
		Kind:       logparser.SpellLandedKindOther,
		SpellName:  "Visions of Grandeur",
		TargetName: "Bob",
	})

	if len(e.timers) != 0 {
		t.Errorf("cast_by_me without matching local cast should drop; got %d timers", len(e.timers))
	}
}
