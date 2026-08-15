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
		hub:            hub,
		timers:         make(map[string]*ActiveTimer),
		pendingArms:    make(map[string]*pendingArm),
		nextStackIndex: make(map[string]uint64),
		charCtx: func() (string, string, int) {
			return "/eq", "Osui", -1
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
		name  string
		setup func(*Engine)
		data  logparser.SpellLandedData
		want  string
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
		{
			// Shield of the Eighth (Coldain ring clicky, id 1963) shares its
			// cast-on-you text with the item-less Shield of the Ring (1796).
			// Both are instant clickies (no "begin casting" line), so there's
			// no recent cast to disambiguate against — but only one is produced
			// by a real item, so it resolves uniquely.
			name: "ambiguous instant clicky resolves to sole item-produced candidate",
			setup: func(e *Engine) {
				e.clickableLoaded = true
				e.clickableSpellIDs = map[int]bool{1963: true}
			},
			data: logparser.SpellLandedData{
				Candidates: []logparser.SpellLandedCandidate{
					{SpellID: 1796, SpellName: "Shield of the Ring"},
					{SpellID: 1963, SpellName: "Shield of the Eighth"},
				},
			},
			want: "Shield of the Eighth",
		},
		{
			// When more than one candidate is item-produced we can't safely
			// pick, so we stay ambiguous (no recent cast → skip).
			name: "ambiguous with multiple item-produced candidates stays empty",
			setup: func(e *Engine) {
				e.clickableLoaded = true
				e.clickableSpellIDs = map[int]bool{1796: true, 1963: true}
			},
			data: logparser.SpellLandedData{
				Candidates: []logparser.SpellLandedCandidate{
					{SpellID: 1796, SpellName: "Shield of the Ring"},
					{SpellID: 1963, SpellName: "Shield of the Eighth"},
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
	e.StartExternal("Visions of Grandeur", "buff", 1620, 0, now.Add(time.Second), nil, 0, "", "", false, "", false)

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
	e.StartExternal("AE Incoming", "debuff", 30, 0, time.Now(), nil, 0, "", "", false, "", false)

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
	e.StartExternal("Long Buff", "buff", 7200, 600, time.Now(), nil, 0, "", "", false, "", false)

	got, ok := e.timers[timerKey("Long Buff", "")]
	if !ok {
		t.Fatalf("timer not found")
	}
	if got.DisplayThresholdSecs != 600 {
		t.Errorf("threshold: got %v, want 600", got.DisplayThresholdSecs)
	}
}

// Pinned timers must float to the top of the broadcast snapshot regardless
// of remaining time, so a raid leader's signature-spell timers don't shift
// down the list as unrelated (unpinned) timers count lower. Within each
// group (pinned, then unpinned) the normal ascending-remaining-time order
// still applies.
// TestConfirmCast_TargetsTheCallersOwnTimer is the regression guard for the
// false-possible-miss storm: every cleric in a CH chain heals the SAME tank,
// so confirmation must be addressed by the callout's full label (which embeds
// the caster) and not by target. The previous per-target FIFO confirmed
// whichever row on that tank was oldest, i.e. almost always a different
// cleric's, which flagged healers who cast perfectly.
func TestConfirmCast_TargetsTheCallersOwnTimer(t *testing.T) {
	e := newTestEngine()
	now := time.Now()
	e.StartExternal("#1  Tank  <- Alice", "ch_chain", 10, 0, now, nil, 0, "Tank", "", false, "", false)
	e.StartExternal("#2  Tank  <- Bob", "ch_chain", 10, 0, now.Add(3*time.Second), nil, 0, "Tank", "", false, "", false)

	// Bob casts. Alice's row is older and shares the target.
	e.ConfirmCast("#2  Tank  <- Bob", "Tank")

	alice := e.timers[timerKey("#1  Tank  <- Alice", "Tank")]
	bob := e.timers[timerKey("#2  Tank  <- Bob", "Tank")]
	if alice == nil || bob == nil {
		t.Fatalf("expected both timers to still exist, got alice=%v bob=%v", alice, bob)
	}
	if !bob.castConfirmed {
		t.Error("Bob's own callout should be the one confirmed")
	}
	if alice.castConfirmed {
		t.Error("Alice's callout must not be confirmed by Bob's cast")
	}
}

// TestConfirmCast_IgnoresUnknownAndOtherCategories keeps confirmation from
// leaking outside the addressed CH-chain row: an unknown label, a mismatched
// target, and a non-CH-chain category must all be no-ops.
func TestConfirmCast_IgnoresUnknownAndOtherCategories(t *testing.T) {
	e := newTestEngine()
	now := time.Now()
	e.StartExternal("#1  MainTank  <- Alice", "ch_chain", 10, 0, now, nil, 0, "MainTank", "", false, "", false)
	e.StartExternal("Some Buff", "buff", 10, 0, now, nil, 0, "MainTank", "", false, "", false)

	e.ConfirmCast("#9  MainTank  <- Nobody", "MainTank") // no such callout
	e.ConfirmCast("#1  MainTank  <- Alice", "OtherTank") // right label, wrong target
	e.ConfirmCast("", "MainTank")                        // empty label is a no-op
	e.ConfirmCast("Some Buff", "MainTank")               // not a CH-chain category

	if e.timers[timerKey("#1  MainTank  <- Alice", "MainTank")].castConfirmed {
		t.Error("ch_chain timer confirmed by a non-matching ConfirmCast")
	}
	if e.timers[timerKey("Some Buff", "MainTank")].castConfirmed {
		t.Error("non-CH-chain category timer must never be touched by ConfirmCast")
	}
}

// TestConfirmCast_ClearsAnAlreadySetMissFlag covers a late confirmation (a
// resisted-and-recast heal): the row must go back to normal rather than stay
// stuck red for the rest of its grace window.
func TestConfirmCast_ClearsAnAlreadySetMissFlag(t *testing.T) {
	e := newTestEngine()
	e.StartExternal("#1  Tank  <- Alice", "ch_chain", 10, 0,
		time.Now().Add(-5*time.Second), nil, 0, "Tank", "", false, "", false)

	e.pruneExpired() // 5s in with no confirmation → flagged
	tm := e.timers[timerKey("#1  Tank  <- Alice", "Tank")]
	if !tm.PossibleMiss {
		t.Fatal("expected an unconfirmed timer past the check delay to be flagged")
	}

	e.ConfirmCast("#1  Tank  <- Alice", "Tank")
	if tm.PossibleMiss || !tm.castConfirmed {
		t.Errorf("late confirmation should clear the flag: miss=%v confirmed=%v",
			tm.PossibleMiss, tm.castConfirmed)
	}
}

// TestUnconfirmCast_FlagsAnInterruptedConfirmedTimer is the core of the
// interrupt-detection refinement: a timer already confirmed by a cast-begin
// line must be un-confirmed and flagged PossibleMiss when that same caster's
// cast is then observed being interrupted, instead of quietly expiring as if
// the heal landed.
func TestUnconfirmCast_FlagsAnInterruptedConfirmedTimer(t *testing.T) {
	e := newTestEngine()
	started := time.Now().Add(-3 * time.Second)
	e.StartExternal("#1  Tank  ← Alice", "ch_chain", 10, 0, started, nil, 0, "Tank", "", false, "", false)
	e.ConfirmCast("#1  Tank  ← Alice", "Tank")

	e.UnconfirmCast("Alice", time.Now())

	tm := e.timers[timerKey("#1  Tank  ← Alice", "Tank")]
	if tm.castConfirmed {
		t.Error("interrupted cast should be un-confirmed")
	}
	if !tm.PossibleMiss {
		t.Error("interrupted cast should be flagged PossibleMiss immediately")
	}
}

// TestUnconfirmCast_IgnoresUnrelatedOrUnconfirmedTimers guards the same
// no-leak properties as TestConfirmCast_IgnoresUnknownAndOtherCategories: an
// interrupt for a caster with no matching confirmed timer, a caster whose row
// was never confirmed in the first place, and a non-CH-chain category must
// all be no-ops.
func TestUnconfirmCast_IgnoresUnrelatedOrUnconfirmedTimers(t *testing.T) {
	e := newTestEngine()
	now := time.Now()
	e.StartExternal("#1  Tank  ← Alice", "ch_chain", 10, 0, now, nil, 0, "Tank", "", false, "", false)
	e.StartExternal("Some Buff", "buff", 10, 0, now, nil, 0, "Tank", "", false, "", false)
	// Bob's timer is deliberately left unconfirmed.
	e.StartExternal("#2  Tank  ← Bob", "ch_chain", 10, 0, now, nil, 0, "Tank", "", false, "", false)

	e.UnconfirmCast("Nobody", now) // no matching caster
	e.UnconfirmCast("Bob", now)    // matching caster, but never confirmed
	e.UnconfirmCast("", now)       // empty caster is a no-op

	if e.timers[timerKey("#1  Tank  ← Alice", "Tank")].PossibleMiss {
		t.Error("Alice's timer was never confirmed and should be untouched by an unrelated interrupt")
	}
	if e.timers[timerKey("#2  Tank  ← Bob", "Tank")].PossibleMiss {
		t.Error("un-confirming a never-confirmed timer must not flag it")
	}
}

// TestUnconfirmCast_IgnoresInterruptOutsideCastWindow guards against a stale
// or out-of-order interrupt line reversing a timer that has already fully
// resolved — the interrupt timestamp must fall within the timer's own cast
// window (StartsAt..ExpiresAt).
func TestUnconfirmCast_IgnoresInterruptOutsideCastWindow(t *testing.T) {
	e := newTestEngine()
	started := time.Now().Add(-20 * time.Second)
	e.StartExternal("#1  Tank  ← Alice", "ch_chain", 10, 0, started, nil, 0, "Tank", "", false, "", false)
	e.ConfirmCast("#1  Tank  ← Alice", "Tank")

	e.UnconfirmCast("Alice", time.Now()) // 20s after start, well past the 10s window

	tm := e.timers[timerKey("#1  Tank  ← Alice", "Tank")]
	if !tm.castConfirmed || tm.PossibleMiss {
		t.Error("an interrupt timestamp outside the cast window must not reverse confirmation")
	}
}

// TestPruneExpired_FlagsUnconfirmedCHChainTimerEarly is the core of the
// possible-miss feature: a CH-chain callout whose caster was never seen
// starting a cast is flagged chChainMissCheckDelay in — well before the 10s
// cast window ends, since a miss reported at the end is far too late to act
// on. A confirmed callout is never flagged.
func TestPruneExpired_FlagsUnconfirmedCHChainTimerEarly(t *testing.T) {
	e := newTestEngine()
	started := time.Now().Add(-chChainMissCheckDelay - 100*time.Millisecond)
	e.StartExternal("#1  Tank  <- Alice", "ch_chain", 10, 0, started, nil, 0, "Tank", "", false, "", false)
	e.StartExternal("#2  Tank  <- Bob", "ch_chain", 10, 0, started, nil, 0, "Tank", "", false, "", false)
	e.ConfirmCast("#1  Tank  <- Alice", "Tank")

	e.pruneExpired()

	if got := e.timers[timerKey("#1  Tank  <- Alice", "Tank")]; got.PossibleMiss {
		t.Error("a confirmed callout must never be flagged PossibleMiss")
	}
	missed := e.timers[timerKey("#2  Tank  <- Bob", "Tank")]
	if !missed.PossibleMiss {
		t.Error("unconfirmed CH-chain timer should be flagged PossibleMiss")
	}
	// The flag is display-only: it must not disturb the countdown, which the
	// CH Chain bar renders and the metronome derives its anchor from.
	if !missed.ExpiresAt.Equal(started.Add(10 * time.Second)) {
		t.Errorf("PossibleMiss must not move ExpiresAt: got %v, want %v",
			missed.ExpiresAt, started.Add(10*time.Second))
	}
}

// TestPruneExpired_MissGraceKeepsRowWithoutMovingExpiry covers the linger
// window that lets the overlay finish showing a red row. It must be applied
// via missGraceUntil, never by extending ExpiresAt: pushing ExpiresAt forward
// made RemainingSeconds climb back up, which re-inflated the overlay bar and
// desynced the metronome anchor by the grace amount.
func TestPruneExpired_MissGraceKeepsRowWithoutMovingExpiry(t *testing.T) {
	e := newTestEngine()
	started := time.Now().Add(-10*time.Second - 200*time.Millisecond)
	e.StartExternal("#2  Tank  <- Bob", "ch_chain", 10, 0, started, nil, 0, "Tank", "", false, "", false)
	wantExpiry := started.Add(10 * time.Second)

	e.pruneExpired()

	missed := e.timers[timerKey("#2  Tank  <- Bob", "Tank")]
	if missed == nil {
		t.Fatal("flagged timer should linger through its grace window, not be dropped")
	}
	if !missed.ExpiresAt.Equal(wantExpiry) {
		t.Errorf("grace must not move ExpiresAt: got %v, want %v", missed.ExpiresAt, wantExpiry)
	}
	if missed.missGraceUntil.IsZero() {
		t.Error("expected a missGraceUntil deadline to be set")
	}

	// Once the grace deadline passes, the row is dropped.
	missed.missGraceUntil = time.Now().Add(-time.Second)
	e.pruneExpired()
	if e.timers[timerKey("#2  Tank  <- Bob", "Tank")] != nil {
		t.Error("PossibleMiss timer should be dropped once its grace window elapses")
	}
}

// TestPruneExpired_ConfirmedCHChainTimerDropsOnTime guards that the grace
// path is scoped to flagged rows only — a confirmed callout expires like any
// other timer.
func TestPruneExpired_ConfirmedCHChainTimerDropsOnTime(t *testing.T) {
	e := newTestEngine()
	e.StartExternal("#1  Tank  <- Alice", "ch_chain", 10, 0,
		time.Now().Add(-10*time.Second-200*time.Millisecond), nil, 0, "Tank", "", false, "", false)
	e.ConfirmCast("#1  Tank  <- Alice", "Tank")

	e.pruneExpired()

	if e.timers[timerKey("#1  Tank  <- Alice", "Tank")] != nil {
		t.Error("a confirmed CH-chain timer should be dropped at expiry with no grace")
	}
}

// TestPruneExpired_NonCHChainCategoriesNeverFlagged guards the scope of the
// feature: ordinary buff/debuff timers must never get a PossibleMiss flag or
// grace extension, confirmed timers unaffected.
func TestPruneExpired_NonCHChainCategoriesNeverFlagged(t *testing.T) {
	e := newTestEngine()
	past := time.Now().Add(-1 * time.Hour)
	e.StartExternal("Mesmerization", "debuff", 10, 0, past, nil, 0, "Bob", "", false, "", false)

	e.pruneExpired()

	if e.timers[timerKey("Mesmerization", "Bob")] != nil {
		t.Error("an expired non-CH-chain timer should be dropped immediately, not flagged/extended")
	}
}

func TestSnapshot_PinnedTimersSortFirst(t *testing.T) {
	e := newTestEngine()
	now := time.Now()
	e.StartExternal("Short Unpinned", "debuff", 5, 0, now, nil, 0, "", "", false, "", false)
	e.StartExternal("Long Pinned", "custom", 300, 0, now, nil, 0, "", "", true, "", false)
	e.StartExternal("Short Pinned", "custom", 10, 0, now, nil, 0, "", "", true, "", false)
	e.StartExternal("Long Unpinned", "buff", 600, 0, now, nil, 0, "", "", false, "", false)

	state := e.GetState()
	if len(state.Timers) != 4 {
		t.Fatalf("expected 4 timers, got %d", len(state.Timers))
	}
	names := make([]string, len(state.Timers))
	for i, tm := range state.Timers {
		names[i] = tm.SpellName
	}
	want := []string{"Short Pinned", "Long Pinned", "Short Unpinned", "Long Unpinned"}
	for i, name := range want {
		if names[i] != name {
			t.Errorf("position %d: got %q, want %q (full order: %v)", i, names[i], name, names)
			break
		}
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
	e.StartExternal("Manual VoG", "buff", 1620, 0, time.Now(), nil, 0, "", "", false, "", false)
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
	e.StartExternal("Koadic's Endless Intellect", "buff", 4500, 300, now.Add(50*time.Millisecond), alerts, 0, "", "", false, "", false)

	if len(e.timers) != 1 {
		t.Fatalf("expected 1 timer (merge, not duplicate), got %d", len(e.timers))
	}
	got := e.timers[key]
	if got.DisplayThresholdSecs != 300 {
		t.Errorf("threshold: got %v, want 300 (merged from trigger)", got.DisplayThresholdSecs)
	}
	if string(got.TimerAlerts) != string(alerts) {
		t.Errorf("alerts: got %s, want %s", got.TimerAlerts, alerts)
	}
}

// Mez spells (Mesmerize/Mesmerization/Dazzle) share the "X has been
// mesmerized." land text but have divergent durations, so the trigger
// pack fires them on "You begin casting <Name>." Without deferred
// rendering, this creates a visible 2-3s "ghost" timer that later gets
// merged when the spell lands — and on fizzle/interrupt/resist leaves a
// stale timer running for its full nominal duration. StartExternal must
// stash these as pendingArms instead of creating immediate timers.
func TestStartExternal_DefersMezTimerToSpellLanded(t *testing.T) {
	e := newTestEngine()
	now := time.Now()
	alerts := json.RawMessage(`[{"id":"fade","seconds":5,"type":"tts"}]`)

	e.StartExternal("Mesmerize", "debuff", 24, 8, now, alerts, 0, "", "", false, "", false)

	if len(e.timers) != 0 {
		t.Fatalf("mez cast-begin must not create a visible timer, got %d", len(e.timers))
	}
	arm, ok := e.pendingArms["Mesmerize"]
	if !ok {
		t.Fatal("pendingArms missing Mesmerize entry")
	}
	if arm.DisplayThresholdSecs != 8 {
		t.Errorf("pending arm threshold: got %v, want 8", arm.DisplayThresholdSecs)
	}
	if string(arm.TimerAlerts) != string(alerts) {
		t.Errorf("pending arm alerts: got %s, want %s", arm.TimerAlerts, alerts)
	}
}

// StopExternal must also clear pendingArms so a resist or worn-off line
// for a deferred-render spell doesn't leave a stale arm that gets
// falsely promoted onto a later genuine cast.
func TestStopExternal_ClearsPendingArm(t *testing.T) {
	e := newTestEngine()
	now := time.Now()
	e.StartExternal("Dazzle", "debuff", 96, 0, now, nil, 0, "", "", false, "", false)
	if _, ok := e.pendingArms["Dazzle"]; !ok {
		t.Fatal("setup: Dazzle arm not stored")
	}

	e.StopExternal("Dazzle", 0)

	if _, ok := e.pendingArms["Dazzle"]; ok {
		t.Error("pending arm should have been cleared on StopExternal")
	}
}

// Pending arms older than pendingArmTTL must be GC'd lazily so a fizzled
// mez doesn't get promoted onto a recast minutes later.
func TestStartExternal_ExpiresStalePendingArms(t *testing.T) {
	e := newTestEngine()
	stale := time.Now().Add(-2 * pendingArmTTL)
	e.pendingArms["Mesmerization"] = &pendingArm{ArmedAt: stale}

	// Any StartExternal call triggers gcPendingArmsLocked; use an unrelated
	// non-deferred trigger so we don't reseed the slot.
	e.StartExternal("AE Incoming", "debuff", 30, 0, time.Now(), nil, 0, "", "", false, "", false)

	if _, ok := e.pendingArms["Mesmerization"]; ok {
		t.Error("stale pending arm should have been GC'd")
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

	e.StopExternal("Visions of Grandeur", 0)

	if len(e.timers) != 1 {
		t.Errorf("expected 1 surviving timer, got %d", len(e.timers))
	}
	if _, ok := e.timers[timerKey("Tashanian", "Mob")]; !ok {
		t.Error("Tashanian timer should have been preserved")
	}
}

// A worn-off line names no target, but several mobs can carry the same
// detrimental at once (AoE mez). Each break is its own worn-off line, so
// StopExternal must peel off ONE timer per call — the nearest expiry — rather
// than wiping every same-named row at once (the reported AoE-mez bug).
func TestStopExternal_DetrimentalPeelsOneTimer(t *testing.T) {
	e := newTestEngine()
	now := time.Now()
	mobs := []struct {
		target string
		expiry time.Duration
	}{
		{"a gnoll", 10 * time.Second},  // earliest — peeled first
		{"a kobold", 20 * time.Second}, // peeled second
		{"a bat", 30 * time.Second},    // survives two breaks
	}
	for _, m := range mobs {
		key := timerKey("Mesmerization", m.target)
		e.timers[key] = &ActiveTimer{
			ID: key, SpellName: "Mesmerization", TargetName: m.target,
			Category: CategoryMez, CastAt: now, StartsAt: now,
			ExpiresAt: now.Add(m.expiry),
		}
	}

	e.StopExternal("Mesmerization", 0)
	if len(e.timers) != 2 {
		t.Fatalf("first break should leave 2 timers, got %d", len(e.timers))
	}
	if _, ok := e.timers[timerKey("Mesmerization", "a gnoll")]; ok {
		t.Error("first break should peel the nearest-expiry timer (a gnoll)")
	}

	e.StopExternal("Mesmerization", 0)
	if len(e.timers) != 1 {
		t.Fatalf("second break should leave 1 timer, got %d", len(e.timers))
	}
	if _, ok := e.timers[timerKey("Mesmerization", "a bat")]; !ok {
		t.Error("the longest-running mez (a bat) should survive two breaks")
	}
}

// sameSpellForDedup matches on name, on a shared non-zero SpellID, and on
// neither when both differ. The SpellID arm is what lets a combined pack
// name ("Speed of the Shissar/Brood") dedup against the DB name the
// spell-landed pipeline resolves ("Speed of the Shissar").
func TestSameSpellForDedup(t *testing.T) {
	cases := []struct {
		name     string
		existing *ActiveTimer
		inName   string
		inID     int
		want     bool
	}{
		{"name match", &ActiveTimer{SpellName: "Clarity"}, "Clarity", 0, true},
		{"id match, names differ", &ActiveTimer{SpellName: "Speed of the Shissar", SpellID: 1939}, "Speed of the Shissar/Brood", 1939, true},
		{"id zero, names differ", &ActiveTimer{SpellName: "Speed of the Shissar", SpellID: 1939}, "Speed of the Shissar/Brood", 0, false},
		{"existing id zero", &ActiveTimer{SpellName: "Foo", SpellID: 0}, "Bar", 1939, false},
		{"different id and name", &ActiveTimer{SpellName: "Foo", SpellID: 10}, "Bar", 20, false},
	}
	for _, tc := range cases {
		if got := sameSpellForDedup(tc.existing, tc.inName, tc.inID); got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, got, tc.want)
		}
	}
}

// A trigger using a combined pack name but linked to a DB SpellID must dedup
// against an already-present spell-landed timer carrying that same SpellID
// under the canonical DB name — otherwise the user sees two rows for one
// cast (the reported "Speed of the Shissar" + "Speed of the Shissar/Brood"
// double-timer). The trigger's per-spell metadata grafts onto the surviving
// timer; the surviving timer keeps its target-bearing identity.
func TestStartExternal_DedupsBySpellIDAcrossNames(t *testing.T) {
	e := newTestEngine()
	now := time.Now()

	// Spell-landed-style timer already present (target-keyed, DB name, no
	// per-trigger metadata).
	key := timerKey("Speed of the Shissar", "Tank")
	e.timers[key] = &ActiveTimer{
		ID: key, SpellName: "Speed of the Shissar", SpellID: 1939,
		TargetName: "Tank", Category: CategoryBuff,
		CastAt: now, StartsAt: now, ExpiresAt: now.Add(30 * time.Minute),
	}

	// Enchanter pack trigger fires with the combined name + linked SpellID and
	// a display-threshold override.
	e.StartExternal("Speed of the Shissar/Brood", "buff", 1800, 120, now, nil, 1939, "", "", false, "", false)

	if len(e.timers) != 1 {
		t.Fatalf("expected the trigger to merge into the existing timer, got %d rows", len(e.timers))
	}
	surviving, ok := e.timers[key]
	if !ok {
		t.Fatalf("the target-keyed spell-landed timer should survive; keys: %v", keysOf(e.timers))
	}
	if surviving.TargetName != "Tank" {
		t.Errorf("surviving timer lost its target: %q", surviving.TargetName)
	}
	if surviving.DisplayThresholdSecs != 120 {
		t.Errorf("trigger threshold should graft onto surviving timer, got %v", surviving.DisplayThresholdSecs)
	}
}

// The trigger's worn-off pattern fires StopExternal with the combined pack
// name + SpellID. After a merge the surviving timer carries the DB name, so a
// name-only removal would miss it; the SpellID arm clears it. This is the
// fade path for haste buffs, which log "Your body slows." (the worn-off
// pattern) with no generic "...spell has worn off." line.
func TestStopExternal_RemovesMergedTimerBySpellID(t *testing.T) {
	e := newTestEngine()
	now := time.Now()
	key := timerKey("Speed of the Shissar", "Osui")
	e.timers[key] = &ActiveTimer{
		ID: key, SpellName: "Speed of the Shissar", SpellID: 1939,
		TargetName: "Osui", CastAt: now, StartsAt: now, ExpiresAt: now.Add(30 * time.Minute),
	}

	e.StopExternal("Speed of the Shissar/Brood", 1939)

	if len(e.timers) != 0 {
		t.Errorf("merged timer should be cleared by SpellID, got %d rows", len(e.timers))
	}
}

// An ambiguous self-land for the Speed of the Shissar/Brood pair (identical
// "...body pulses with the spirit of the Shissar." text, two DB rows) with no
// disambiguating recent cast resolves to the combined group name rather than
// "" — so the pipeline creates a target-keyed timer instead of dropping the
// land and leaving only the pack trigger's target-less row.
func TestResolveLandedSpellName_AmbiguousGroupFallback(t *testing.T) {
	e := newTestEngine()
	data := logparser.SpellLandedData{
		Kind: logparser.SpellLandedKindYou,
		Candidates: []logparser.SpellLandedCandidate{
			{SpellID: 1939, SpellName: "Speed of the Shissar"},
			{SpellID: 2895, SpellName: "Speed of the Brood"},
		},
	}

	if got := e.resolveLandedSpellName(data); got != "Speed of the Shissar/Brood" {
		t.Errorf("ambiguous land: want combined group name, got %q", got)
	}

	// A recent matching cast still wins — it names the specific member, which is
	// more precise than the combined fallback.
	e.lastCastSpell = "Speed of the Brood"
	e.lastCastAt = time.Now()
	if got := e.resolveLandedSpellName(data); got != "Speed of the Brood" {
		t.Errorf("recent cast should resolve to the specific member, got %q", got)
	}
}

func TestMatchAmbiguousLandGroup(t *testing.T) {
	cand := func(ids ...int) []logparser.SpellLandedCandidate {
		out := make([]logparser.SpellLandedCandidate, len(ids))
		for i, id := range ids {
			out[i] = logparser.SpellLandedCandidate{SpellID: id}
		}
		return out
	}
	if g := matchAmbiguousLandGroup(cand(1939, 2895)); g == nil || g.displayName != "Speed of the Shissar/Brood" {
		t.Errorf("exact member set should match the group, got %v", g)
	}
	if g := matchAmbiguousLandGroup(cand(2895, 1939)); g == nil {
		t.Error("member order should not matter")
	}
	if g := matchAmbiguousLandGroup(cand(1939)); g != nil {
		t.Error("a subset of members must not match")
	}
	if g := matchAmbiguousLandGroup(cand(1939, 2895, 9999)); g != nil {
		t.Error("a superset with an outsider must not match")
	}
	if g := matchAmbiguousLandGroup(cand(1939, 9999)); g != nil {
		t.Error("a set with a non-member must not match")
	}
}

func TestAmbiguousGroupRepID(t *testing.T) {
	if got := ambiguousGroupRepID("Speed of the Shissar/Brood"); got != 1939 {
		t.Errorf("combined name should map to rep spell 1939, got %d", got)
	}
	if got := ambiguousGroupRepID("Clarity"); got != 0 {
		t.Errorf("ordinary spell should map to 0, got %d", got)
	}
}

func keysOf(m map[string]*ActiveTimer) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
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

	e2 := &Engine{charCtx: func() (string, string, int) { return "", "", -1 }}
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
	// A self-buff created before the character context was available carries
	// the literal "You" placeholder rather than the resolved name. It must
	// still be cleared on death (the original bug: these lingered).
	e.timers[timerKey("Clarity", "You")] = &ActiveTimer{
		ID: timerKey("Clarity", "You"), SpellName: "Clarity",
		TargetName: "You", CastAt: now, StartsAt: now, ExpiresAt: now.Add(30 * time.Minute),
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
	if _, ok := e.timers[timerKey("Clarity", "You")]; ok {
		t.Error(`self-target buff with "You" placeholder should have been removed on player death`)
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

// sameLandOrphan gates the cross-name dedup that collapses a broad trigger's
// phantom timer (e.g. the Enchanter pack's "Tashanian" firing on the shared
// "glances nervously about" land text) into the pipeline's target-bound row.
// It must fire for a non-charm timer whose land timestamp matches the
// resolving spell, whose category family matches the pipeline's resolved
// category, and whose target is either empty or the same target the
// pipeline resolved — never for charm, a mismatched category family, a
// different target, or an orphan from a different log line.
func TestSameLandOrphan(t *testing.T) {
	land := time.Date(2026, 5, 30, 12, 13, 47, 0, time.UTC)
	orphanDebuff := &ActiveTimer{TargetName: "", Category: CategoryDebuff, StartsAt: land}
	if !sameLandOrphan(orphanDebuff, "a gnoll", land, CategoryDebuff) {
		t.Error("orphan detrimental on the same land line should merge")
	}
	// A trigger's generic "debuff" category still merges into the pipeline's
	// more specific detrimental subtype (dot/mez/stun) — TimerType only ever
	// produces the flat "debuff" category, never the finer-grained one.
	if !sameLandOrphan(orphanDebuff, "a gnoll", land, CategoryDot) {
		t.Error("trigger's generic debuff orphan should merge into a more specific detrimental category")
	}
	// Different land line (timestamp) — two distinct debuffs, keep both.
	if sameLandOrphan(orphanDebuff, "a gnoll", land.Add(time.Second), CategoryDebuff) {
		t.Error("orphan from a different land timestamp must not merge")
	}
	// Charm orphan is intentionally kept separate.
	charm := &ActiveTimer{TargetName: "", Category: CategoryDebuff, IsCharm: true, StartsAt: land}
	if sameLandOrphan(charm, "a gnoll", land, CategoryDebuff) {
		t.Error("charm orphan must not be absorbed")
	}
	// Target-bound timer merges when it names the SAME target the pipeline
	// resolved — the case introduced when capture-less triggers started
	// falling back to the current combat target instead of going orphan
	// (e.g. the broad "Rapture"/"Tashanian" triggers landing on a raid boss
	// alongside "Ancient: Eternal Rapture"/"Wind of Tashanian").
	sameTargetBound := &ActiveTimer{TargetName: "a raid boss", Category: CategoryDebuff, StartsAt: land}
	if !sameLandOrphan(sameTargetBound, "a raid boss", land, CategoryDebuff) {
		t.Error("trigger twin bound to the same target as the pipeline should still merge")
	}
	// Target-bound timer on a DIFFERENT target is a separate recipient (AoE
	// mez cast on several mobs) and must not merge.
	bound := &ActiveTimer{TargetName: "a gnoll", Category: CategoryDebuff, StartsAt: land}
	if sameLandOrphan(bound, "an orc", land, CategoryDebuff) {
		t.Error("timer bound to a different target must not merge as an orphan")
	}
	// A buff orphan must not be absorbed as a detrimental land twin (category
	// family mismatch), even on the same land line.
	buff := &ActiveTimer{TargetName: "", Category: CategoryBuff, StartsAt: land}
	if sameLandOrphan(buff, "a gnoll", land, CategoryDebuff) {
		t.Error("buff orphan must not be treated as a detrimental land twin")
	}
	// Reproduces the live report: Savage Spirit shares its land text
	// ("'s eyes gleam with madness.") with six other spells; a pack trigger
	// for one of those siblings fires on the same self-cast line and must
	// collapse into the pipeline's correctly-resolved Savage Spirit row
	// instead of surviving as a duplicate buff.
	orphanBuff := &ActiveTimer{TargetName: "", Category: CategoryBuff, StartsAt: land}
	if !sameLandOrphan(orphanBuff, "Kilowatt", land, CategoryBuff) {
		t.Error("orphan buff on the same land line should merge into the pipeline's resolved buff")
	}
	// Detrimental orphan must not be absorbed into a resolved buff — category
	// families must match in both directions.
	if sameLandOrphan(orphanDebuff, "Kilowatt", land, CategoryBuff) {
		t.Error("detrimental orphan must not be treated as a buff land twin")
	}
}

// A charm timer (Cajoling Whispers etc.) tracks a living, still-charmed pet.
// Killing an UNRELATED mob — e.g. the enemy the charmed pet is tanking — must
// not sweep the orphan charm timer away with the other detrimentals.
// Reproduces the live report: charming a Netherbian Warrior, then killing the
// Netherbian Drone it was fighting, dropped the charm timer.
func TestHandle_Kill_KeepsOrphanCharmTimer(t *testing.T) {
	e := newTestEngine()
	now := time.Now()
	// Orphan charm (trigger-driven "You begin casting Cajoling Whispers." —
	// no target on the line). Flagged IsCharm by StartExternal's spell lookup.
	e.timers[timerKey("Cajoling Whispers", "")] = &ActiveTimer{
		ID: timerKey("Cajoling Whispers", ""), SpellName: "Cajoling Whispers",
		Category: CategoryDebuff, IsCharm: true, TargetName: "",
		CastAt: now, StartsAt: now, ExpiresAt: now.Add(3 * time.Minute),
	}
	// A plain orphan debuff in the same fight — should still clear on kill.
	e.timers[timerKey("Tashanian", "")] = &ActiveTimer{
		ID: timerKey("Tashanian", ""), SpellName: "Tashanian", Category: CategoryDebuff,
		TargetName: "", CastAt: now, StartsAt: now, ExpiresAt: now.Add(13 * time.Minute),
	}

	e.Handle(logparser.LogEvent{
		Type: logparser.EventKill,
		Data: logparser.KillData{Killer: "Osui", Target: "A Netherbian Drone"},
	})

	if _, ok := e.timers[timerKey("Cajoling Whispers", "")]; !ok {
		t.Error("orphan charm timer should survive an unrelated kill")
	}
	if _, ok := e.timers[timerKey("Tashanian", "")]; ok {
		t.Error("orphan non-charm debuff should still clear on kill")
	}
}

// "Your charm spell has worn off." (EventCharmBroken) must clear the charm
// timer. EQ emits this generic line for every charm spell, so the charm
// trigger's per-name worn-off pattern never matches it — the engine has to
// catch the break itself. Non-charm detrimentals are untouched.
func TestHandle_CharmBroken_ClearsCharmTimers(t *testing.T) {
	e := newTestEngine()
	now := time.Now()
	e.timers[timerKey("Cajoling Whispers", "")] = &ActiveTimer{
		ID: timerKey("Cajoling Whispers", ""), SpellName: "Cajoling Whispers",
		Category: CategoryDebuff, IsCharm: true, TargetName: "",
		CastAt: now, StartsAt: now, ExpiresAt: now.Add(3 * time.Minute),
	}
	e.timers[timerKey("Tashanian", "a gnoll")] = &ActiveTimer{
		ID: timerKey("Tashanian", "a gnoll"), SpellName: "Tashanian", Category: CategoryDebuff,
		TargetName: "a gnoll", CastAt: now, StartsAt: now, ExpiresAt: now.Add(13 * time.Minute),
	}

	e.Handle(logparser.LogEvent{Type: logparser.EventCharmBroken})

	if _, ok := e.timers[timerKey("Cajoling Whispers", "")]; ok {
		t.Error("charm timer should clear on EventCharmBroken")
	}
	if _, ok := e.timers[timerKey("Tashanian", "a gnoll")]; !ok {
		t.Error("non-charm debuff should survive a charm break")
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

func TestParseCorpseTarget(t *testing.T) {
	cases := []struct {
		in       string
		wantName string
		wantOK   bool
	}{
		{"Daibo Xi Xin's corpse", "Daibo Xi Xin", true},
		{"a netherbian drone's corpse", "a netherbian drone", true},
		{"A Gnoll's Corpse", "A Gnoll", true},
		{"a_gnoll's_corpse", "a_gnoll", true},
		{"Daibo Xi Xin", "Daibo Xi Xin", false},
		{"corpse", "corpse", false},
	}
	for _, tc := range cases {
		gotName, gotOK := parseCorpseTarget(tc.in)
		if gotName != tc.wantName || gotOK != tc.wantOK {
			t.Errorf("parseCorpseTarget(%q) = (%q, %t), want (%q, %t)",
				tc.in, gotName, gotOK, tc.wantName, tc.wantOK)
		}
	}
}

// A Zeal corpse target is the raid-range death signal: when a boss dies while
// still selected, the pipe reports "<Name>'s corpse" even though the slain
// line never reaches a far-off caster's log. The transition must drop the
// boss's detrimental timers just like a log-driven kill would.
func TestHandlePipeTarget_CorpseDropsDetrimental(t *testing.T) {
	e := newTestEngine()
	now := time.Now()
	e.timers[timerKey("Tashanian", "Daibo Xi Xin")] = &ActiveTimer{
		ID: timerKey("Tashanian", "Daibo Xi Xin"), SpellName: "Tashanian", Category: CategoryDebuff,
		TargetName: "Daibo Xi Xin", CastAt: now, StartsAt: now, ExpiresAt: now.Add(13 * time.Minute),
	}

	// Live target first, then the corpse form once it dies.
	e.HandlePipeTarget("Daibo Xi Xin")
	if _, ok := e.timers[timerKey("Tashanian", "Daibo Xi Xin")]; !ok {
		t.Fatal("timer should still exist while the boss is alive")
	}
	e.HandlePipeTarget("Daibo Xi Xin's corpse")
	if _, ok := e.timers[timerKey("Tashanian", "Daibo Xi Xin")]; ok {
		t.Error("detrimental timer should be removed once the target is a corpse")
	}
}

// A non-corpse target must never drop timers, and the ~10 Hz repeat of the
// same corpse name must not keep re-firing removeOnKill against later timers.
func TestHandlePipeTarget_NonCorpseAndRepeatAreNoops(t *testing.T) {
	e := newTestEngine()
	now := time.Now()
	add := func() {
		e.timers[timerKey("Cripple", "a netherbian drone")] = &ActiveTimer{
			ID: timerKey("Cripple", "a netherbian drone"), SpellName: "Cripple", Category: CategoryDebuff,
			TargetName: "a netherbian drone", CastAt: now, StartsAt: now, ExpiresAt: now.Add(3 * time.Minute),
		}
	}

	// Live target: no removal.
	add()
	e.HandlePipeTarget("a netherbian drone")
	if _, ok := e.timers[timerKey("Cripple", "a netherbian drone")]; !ok {
		t.Error("live (non-corpse) target must not drop timers")
	}

	// First corpse pulse removes it.
	e.HandlePipeTarget("a netherbian drone's corpse")
	if _, ok := e.timers[timerKey("Cripple", "a netherbian drone")]; ok {
		t.Fatal("corpse target should have removed the timer")
	}

	// A re-cast on a freshly-pulled same-name mob, then a repeat of the
	// identical corpse string: the de-dupe must suppress the repeat so the
	// new timer survives until the target actually changes.
	add()
	e.HandlePipeTarget("a netherbian drone's corpse")
	if _, ok := e.timers[timerKey("Cripple", "a netherbian drone")]; !ok {
		t.Error("repeated identical corpse target must be de-duped, not re-fire removeOnKill")
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

// StartExternal's stack argument (Trigger.TimerStack) is the fix for the
// reported "respawn timers overwrite each other" bug: two firings of the
// same trigger name must produce two independent rows sharing one label,
// not one row that keeps getting restarted.
func TestStartExternal_Stacking_CreatesIndependentRows(t *testing.T) {
	e := newTestEngine()
	now := time.Now()

	e.StartExternal("Zun Thall Xakra Spawn", "custom", 60, 0, now, nil, 0, "", "", false, "", false, true)
	e.StartExternal("Zun Thall Xakra Spawn", "custom", 60, 0, now.Add(5*time.Second), nil, 0, "", "", false, "", false, true)

	if len(e.timers) != 2 {
		t.Fatalf("expected 2 stacked rows, got %d", len(e.timers))
	}
	var ids []string
	for id, timer := range e.timers {
		ids = append(ids, id)
		if timer.SpellName != "Zun Thall Xakra Spawn" {
			t.Errorf("SpellName should stay the trigger name (no suffix leak), got %q", timer.SpellName)
		}
		if !timer.Stacked {
			t.Error("stacked timer should have Stacked=true")
		}
	}
	if ids[0] == ids[1] {
		t.Errorf("expected distinct map keys, both were %q", ids[0])
	}
}

// Two mobs of the same name can die within the same log second (raid pulls,
// PBAE). The stacking uniquifier must be a monotonic counter, not the log
// timestamp, or these would collide exactly like the pre-fix bug.
func TestStartExternal_Stacking_SameTimestampStillCreatesTwoRows(t *testing.T) {
	e := newTestEngine()
	now := time.Now()

	e.StartExternal("Griffon", "custom", 30, 0, now, nil, 0, "", "", false, "", false, true)
	e.StartExternal("Griffon", "custom", 30, 0, now, nil, 0, "", "", false, "", false, true)

	if len(e.timers) != 2 {
		t.Fatalf("same-second stacked firings should still produce 2 rows, got %d", len(e.timers))
	}
}

// Without stacking, two same-name firings inside dedupGraceWindow merge into
// one row (existing cross-pipeline dedup behaviour). With stacking, that
// merge loop must be bypassed entirely — sameSpellForDedup matches on name,
// not on the map key, so a unique key alone wouldn't be enough.
func TestStartExternal_Stacking_BypassesDedupGraceWindow(t *testing.T) {
	e := newTestEngine()
	now := time.Now()

	e.StartExternal("Griffon", "custom", 30, 0, now, nil, 0, "", "", false, "", false)
	e.StartExternal("Griffon", "custom", 30, 0, now.Add(time.Second), nil, 0, "", "", false, "", false)
	if len(e.timers) != 1 {
		t.Fatalf("non-stacked firings inside the grace window should merge to 1 row, got %d", len(e.timers))
	}

	e2 := newTestEngine()
	e2.StartExternal("Griffon", "custom", 30, 0, now, nil, 0, "", "", false, "", false, true)
	e2.StartExternal("Griffon", "custom", 30, 0, now.Add(time.Second), nil, 0, "", "", false, "", false, true)
	if len(e2.timers) != 2 {
		t.Fatalf("stacked firings inside the grace window should still produce 2 rows, got %d", len(e2.timers))
	}
}

// RemoveByID (used by the Custom Timers panel's per-row dismiss button)
// must drop exactly the row whose key was clicked, leaving sibling stacked
// rows for the same trigger untouched.
func TestRemoveByID_DropsOnlyOneStackedRow(t *testing.T) {
	e := newTestEngine()
	now := time.Now()
	e.StartExternal("Griffon", "custom", 30, 0, now, nil, 0, "", "", false, "", false, true)
	e.StartExternal("Griffon", "custom", 30, 0, now.Add(time.Second), nil, 0, "", "", false, "", false, true)
	if len(e.timers) != 2 {
		t.Fatalf("setup: expected 2 stacked rows, got %d", len(e.timers))
	}
	var target string
	for id := range e.timers {
		target = id
		break
	}

	if !e.RemoveByID(target) {
		t.Fatal("RemoveByID should report the row was found and removed")
	}
	if len(e.timers) != 1 {
		t.Fatalf("expected 1 remaining row, got %d", len(e.timers))
	}
	if _, ok := e.timers[target]; ok {
		t.Error("the removed row's key should no longer be present")
	}
}

// A worn-off pattern match names no specific instance. For stacked timers —
// like the peel-one-per-signal rule already used for AoE detrimentals —
// clearing should drop just the oldest still-running row per signal, not
// every stacked row for the trigger at once.
func TestRemoveBySpellNameOrID_PeelsOneStackedRow(t *testing.T) {
	e := newTestEngine()
	now := time.Now()
	e.StartExternal("Zun Thall Xakra Spawn", "custom", 300, 0, now, nil, 0, "", "", false, "", false, true)
	e.StartExternal("Zun Thall Xakra Spawn", "custom", 900, 0, now.Add(time.Second), nil, 0, "", "", false, "", false, true)
	if len(e.timers) != 2 {
		t.Fatalf("setup: expected 2 stacked rows, got %d", len(e.timers))
	}
	var earliest, latest string
	for id, timer := range e.timers {
		if timer.DurationSeconds == 300 {
			earliest = id
		} else {
			latest = id
		}
	}

	e.removeBySpellNameOrID("Zun Thall Xakra Spawn", 0)

	if len(e.timers) != 1 {
		t.Fatalf("expected 1 surviving row, got %d", len(e.timers))
	}
	if _, ok := e.timers[earliest]; ok {
		t.Error("the nearer-expiry stacked row should have been peeled")
	}
	if _, ok := e.timers[latest]; !ok {
		t.Error("the later-expiry stacked row should have survived")
	}
}

// maxStackedPerName caps runaway stacking (a misconfigured trigger on a
// frequent log line): once the cap is hit, the next firing evicts the
// oldest-expiring stacked row for that name rather than growing unbounded.
func TestStartExternal_Stacking_CapEvictsOldest(t *testing.T) {
	e := newTestEngine()
	now := time.Now()
	for i := 0; i < maxStackedPerName; i++ {
		e.StartExternal("Griffon", "custom", float64(10+i), 0, now, nil, 0, "", "", false, "", false, true)
	}
	if len(e.timers) != maxStackedPerName {
		t.Fatalf("setup: expected %d rows, got %d", maxStackedPerName, len(e.timers))
	}

	e.StartExternal("Griffon", "custom", 1000, 0, now, nil, 0, "", "", false, "", false, true)

	if len(e.timers) != maxStackedPerName {
		t.Fatalf("expected cap to hold at %d rows, got %d", maxStackedPerName, len(e.timers))
	}
	for _, timer := range e.timers {
		if timer.DurationSeconds == 10 {
			t.Error("the oldest-expiring row (duration 10) should have been evicted")
		}
	}
}

// removeOnKill must not sweep a stacked timer that carries the just-killed
// mob's name as its TargetName. The log tailer delivers raw lines (which
// fire triggers) before parsed events, so a respawn trigger firing on
// "You have slain Foo!" creates its stacked timer a moment before this same
// kill's EventKill reaches removeOnKill("Foo") — without the exemption that
// would delete the row the instant it was created.
func TestHandle_Kill_LeavesStackedTimersAlone(t *testing.T) {
	e := newTestEngine()
	now := time.Now()
	e.StartExternal("a gnoll respawned", "custom", 1200, 0, now, nil, 0, "a gnoll", "", false, "", false, true)

	e.Handle(logparser.LogEvent{
		Type: logparser.EventKill,
		Data: logparser.KillData{Killer: "Tank", Target: "a gnoll"},
	})

	if len(e.timers) != 1 {
		t.Errorf("stacked timer should survive a kill of its same-named target, got %d timers", len(e.timers))
	}
}
