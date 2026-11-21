package spelltimer

import (
	"testing"
	"time"
)

// seedSelfBuff adds a buff timer on the test engine's active character
// ("Osui") that has remaining left at now.
func seedSelfBuff(e *Engine, spell string, now time.Time, elapsed, remaining time.Duration) *ActiveTimer {
	tm := &ActiveTimer{
		ID:         timerKey(spell, "Osui"),
		SpellName:  spell,
		TargetName: "Osui",
		Category:   CategoryBuff,
		StartsAt:   now.Add(-elapsed),
		ExpiresAt:  now.Add(remaining),
	}
	tm.DurationSeconds = tm.ExpiresAt.Sub(tm.StartsAt).Seconds()
	e.timers[tm.ID] = tm
	return tm
}

func TestPipeBuffTicksCorrectSelfBuff(t *testing.T) {
	now := time.Now()
	tick := eqTickDuration

	tests := []struct {
		name      string
		remaining time.Duration
		ticks     int
		want      time.Duration // expected remaining after the snapshot
	}{
		// Formula used the reader's own AAs; the caster's extended it.
		{"pipe says longer", 30 * time.Minute, 400, 400*tick - tick/2},
		{"pipe says shorter", 30 * time.Minute, 100, 100*tick - tick/2},
		// Already consistent with the tick count: keep sub-tick precision.
		{"inside window", 100*tick - 2*time.Second, 100, 100*tick - 2*time.Second},
		{"within slack above", 100*tick + time.Second, 100, 100*tick + time.Second},
		// Not a countdown.
		{"permanent", 30 * time.Minute, pipePermanentTicks, 30 * time.Minute},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := newTestEngine()
			tm := seedSelfBuff(e, "Aegolism", now, time.Minute, tc.remaining)
			e.setPipeBuffSlotsAt("Osui", []PipeBuff{{Name: "Aegolism", Ticks: tc.ticks}}, now)
			if got := tm.ExpiresAt.Sub(now); got != tc.want {
				t.Fatalf("remaining = %v, want %v", got, tc.want)
			}
			if d := tm.ExpiresAt.Sub(tm.StartsAt).Seconds(); tm.DurationSeconds != d {
				t.Fatalf("DurationSeconds = %v, want %v", tm.DurationSeconds, d)
			}
		})
	}
}

func TestPipeBuffTicksIgnoreOtherCharacterAndOtherTargets(t *testing.T) {
	now := time.Now()
	e := newTestEngine()
	self := seedSelfBuff(e, "Aegolism", now, time.Minute, 30*time.Minute)
	other := &ActiveTimer{
		ID: timerKey("Aegolism", "Groupmate"), SpellName: "Aegolism", TargetName: "Groupmate",
		Category: CategoryBuff, StartsAt: now, ExpiresAt: now.Add(30 * time.Minute),
	}
	e.timers[other.ID] = other
	before := self.ExpiresAt

	// The pipe belongs to a boxed character, not the one whose log is read.
	e.setPipeBuffSlotsAt("Boxedalt", []PipeBuff{{Name: "Aegolism", Ticks: 50}}, now)
	if !self.ExpiresAt.Equal(before) {
		t.Fatal("snapshot from another character changed the active player's timer")
	}

	e.setPipeBuffSlotsAt("osui", []PipeBuff{{Name: "Aegolism", Ticks: 50}}, now)
	if self.ExpiresAt.Equal(before) {
		t.Fatal("active character's snapshot (case-insensitive) should apply")
	}
	if !other.ExpiresAt.Equal(now.Add(30 * time.Minute)) {
		t.Fatal("a buff on someone else must not follow the player's buff window")
	}
}

func TestPipeBuffFadeEndsConfirmedTimer(t *testing.T) {
	now := time.Now()
	e := newTestEngine()
	clarity := seedSelfBuff(e, "Clarity", now, time.Minute, 20*time.Minute)
	// Never seen in a ticked slot (e.g. the client names it differently):
	// the pipe can't vouch for it, so it is never ended by absence.
	unconfirmed := seedSelfBuff(e, "Illusion: Wood Elf", now, time.Minute, 20*time.Minute)
	other := []PipeBuff{{Name: "Spirit of Wolf", Ticks: 300}}

	e.setPipeBuffSlotsAt("Osui", append([]PipeBuff{{Name: "Clarity", Ticks: 200}}, other...), now)

	// Dropped, then an empty frame (zone/camp) — neither may end it.
	e.setPipeBuffSlotsAt("Osui", other, now.Add(time.Second))
	e.setPipeBuffSlotsAt("Osui", nil, now.Add(2*time.Second))
	if !clarity.ExpiresAt.After(now.Add(2 * time.Second)) {
		t.Fatal("ended before the fade was confirmed")
	}
	// Still missing well past the confirmation window.
	at := now.Add(1*time.Second + pipeFadeConfirm)
	e.setPipeBuffSlotsAt("Osui", other, at)
	if !clarity.ExpiresAt.Equal(at) {
		t.Fatalf("Clarity should expire at %v, got %v", at, clarity.ExpiresAt)
	}
	if !unconfirmed.ExpiresAt.After(at) {
		t.Fatal("an unconfirmed timer must not be ended by the pipe")
	}
}

func TestPipeBuffFadeResetWhenBuffReturns(t *testing.T) {
	now := time.Now()
	e := newTestEngine()
	tm := seedSelfBuff(e, "Clarity", now, time.Minute, 20*time.Minute)
	keep := []PipeBuff{{Name: "Spirit of Wolf", Ticks: 300}}

	e.setPipeBuffSlotsAt("Osui", []PipeBuff{{Name: "Clarity", Ticks: 200}}, now)
	e.setPipeBuffSlotsAt("Osui", keep, now.Add(time.Second))
	// Back in the next frame: the missing streak must reset.
	e.setPipeBuffSlotsAt("Osui", []PipeBuff{{Name: "Clarity", Ticks: 200}}, now.Add(1500*time.Millisecond))
	e.setPipeBuffSlotsAt("Osui", keep, now.Add(2*time.Second))
	e.setPipeBuffSlotsAt("Osui", keep, now.Add(3*time.Second))
	if !tm.ExpiresAt.After(now.Add(3 * time.Second)) {
		t.Fatal("a brief absence after the buff came back ended the timer")
	}
}

// The pipe's buff window reaches TimerState as spell ids for the active
// character, including buffs no timer covers (another player's Hand of
// Virtue, whose land text Virtue shares). Another character's snapshot leaves
// it empty, so consumers fall back to the timers. A disconnect (camping out)
// keeps the last window, marked stale, until the next pipe update.
func TestPipeBuffWindowInTimerState(t *testing.T) {
	e, _ := levelEngine(t, 13, 65)
	now := time.Now()
	window := []PipeBuff{
		{Name: "Hand of Virtue", Ticks: 1400},
		{Name: "Focus of the Seventh", Ticks: 700},
		{Name: "Brell's Steadfast Aegis", Ticks: 1400},
		{Name: "Hand of Virtue", Ticks: 1400}, // duplicate slot: listed once
		{Name: "Not A Spell", Ticks: 10},
	}
	e.setPipeBuffSlotsAt("Tester", window, now)
	got := e.snapshot(now).SelfBuffSpellIDs
	want := []int{3479, 3397, 3578}
	if !sameInts(got, want) {
		t.Fatalf("SelfBuffSpellIDs = %v, want %v", got, want)
	}

	e.setPipeBuffSlotsAt("SomeoneElse", window, now)
	if got := e.snapshot(now).SelfBuffSpellIDs; len(got) != 0 {
		t.Fatalf("other character's window leaked: %v", got)
	}
	if e.snapshot(now).SelfBuffsStale {
		t.Fatal("empty window marked stale")
	}
	e.setPipeBuffSlotsAt("Tester", window, now)
	e.setPipeBuffSlotsAt("", nil, now)
	st := e.snapshot(now)
	if !sameInts(st.SelfBuffSpellIDs, want) || !st.SelfBuffsStale {
		t.Fatalf("after disconnect: ids %v stale %v, want %v kept and stale", st.SelfBuffSpellIDs, st.SelfBuffsStale, want)
	}
	// The next pipe update replaces it and clears the stale mark.
	e.setPipeBuffSlotsAt("Tester", window[:1], now)
	st = e.snapshot(now)
	if !sameInts(st.SelfBuffSpellIDs, []int{3479}) || st.SelfBuffsStale {
		t.Fatalf("after reconnect: ids %v stale %v, want [3479] fresh", st.SelfBuffSpellIDs, st.SelfBuffsStale)
	}
}

// A combined timer ("Illusion", "Speed of the Shissar/Brood") never matches
// a slot name, so the pipe matches it by family: confirmed while any member
// is in the window (an illusion swap keeps it), ended once none is.
func TestPipeBuffCombinedTimersMatchByFamily(t *testing.T) {
	cases := []struct {
		timer   string
		member  string
		swapped string // another member replacing it; "" to skip
	}{
		{illusionCombinedName, "Illusion: Dark Elf", "Illusion: Gnome"},
		{"Speed of the Shissar/Brood", "Speed of the Brood", ""},
	}
	for _, c := range cases {
		t.Run(c.timer, func(t *testing.T) {
			e, _ := levelEngine(t, 13, 65)
			now := time.Now()
			tm := &ActiveTimer{
				ID: timerKey(c.timer, "Tester"), SpellName: c.timer, TargetName: "Tester",
				Category: CategoryBuff, StartsAt: now.Add(-time.Minute), ExpiresAt: now.Add(20 * time.Minute),
			}
			e.timers[tm.ID] = tm
			wolf := PipeBuff{Name: "Spirit of Wolf", Ticks: 300}

			e.setPipeBuffSlotsAt("Tester", []PipeBuff{{Name: c.member, Ticks: 150}, wolf}, now)
			if !e.pipeConfirmed[tm.ID] {
				t.Fatal("combined timer not confirmed by its family member")
			}
			if got := tm.ExpiresAt.Sub(now); got != 150*eqTickDuration-eqTickDuration/2 {
				t.Fatalf("not synced to the member's ticks: %v left", got)
			}
			at := now
			if c.swapped != "" {
				for i := 1; i <= 3; i++ {
					at = now.Add(time.Duration(i) * time.Second)
					e.setPipeBuffSlotsAt("Tester", []PipeBuff{{Name: c.swapped, Ticks: 140}, wolf}, at)
				}
				if !tm.ExpiresAt.After(at) {
					t.Fatal("swapping to another member ended the timer")
				}
			}
			// Clicked off: gone from the window past the confirm window.
			off := at.Add(time.Second)
			e.setPipeBuffSlotsAt("Tester", []PipeBuff{wolf}, off)
			end := off.Add(pipeFadeConfirm)
			e.setPipeBuffSlotsAt("Tester", []PipeBuff{wolf}, end)
			if !tm.ExpiresAt.Equal(end) {
				t.Fatalf("combined timer should end at %v, got %v", end, tm.ExpiresAt)
			}
		})
	}
}
