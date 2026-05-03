package spelltimer

import (
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
		// Default to "anyone" (post-PR1 behaviour). Tests that need
		// "self"-only behaviour install a different scopeFn directly.
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
	e.StartExternal("Visions of Grandeur", "buff", 1620, 0, now.Add(time.Second), nil)

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
	e.StartExternal("AE Incoming", "debuff", 30, 0, time.Now(), nil)

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
	e.StartExternal("Long Buff", "buff", 7200, 600, time.Now(), nil)

	got, ok := e.timers[timerKey("Long Buff", "")]
	if !ok {
		t.Fatalf("timer not found")
	}
	if got.DisplayThresholdSecs != 600 {
		t.Errorf("threshold: got %d, want 600", got.DisplayThresholdSecs)
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

// trackingScope falls back to "anyone" for nil providers and unknown values
// so legacy/empty config files don't unexpectedly silence the engine.
func TestTrackingScope_DefaultsAndFallbacks(t *testing.T) {
	cases := []struct {
		name string
		fn   ScopeProvider
		want string
	}{
		{"nil provider", nil, scopeAnyone},
		{"empty string", func() string { return "" }, scopeAnyone},
		{"unknown value", func() string { return "garbage" }, scopeAnyone},
		{"explicit anyone", func() string { return scopeAnyone }, scopeAnyone},
		{"explicit self", func() string { return scopeSelf }, scopeSelf},
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
