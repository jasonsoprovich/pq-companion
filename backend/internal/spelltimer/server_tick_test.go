package spelltimer

import (
	"strconv"
	"testing"
	"time"
)

// near reports whether a and b are within the gauge's resolution (1/1000 of a
// tick, plus integer truncation).
func near(a, b time.Time) bool {
	d := a.Sub(b)
	return d < 20*time.Millisecond && d > -20*time.Millisecond
}

// gaugeFor builds the gauge 24 reading Zeal would send with until left before
// the next tick (Zeal tick.cpp GetTickGauge).
func gaugeFor(until time.Duration, reverse bool) (float64, string) {
	ms := until.Milliseconds()
	secs := ms/1000 + 1
	if secs > 6 {
		secs = 6
	}
	v := float64(ms * 1000 / serverTickPeriod.Milliseconds())
	if reverse {
		v = 1000 - v
	}
	return v, strconv.FormatInt(secs, 10)
}

// feedTicks sends readings every 200 ms for 2 s from start, with the server
// tick boundary at tickAt (plus whole periods).
func feedTicks(e *Engine, start, tickAt time.Time) time.Time {
	now := start
	for i := 0; i <= 10; i++ {
		d := tickAt.Sub(now) % serverTickPeriod
		if d <= 0 {
			d += serverTickPeriod
		}
		v, txt := gaugeFor(d, false)
		e.setPipeServerTickAt("Osui", v, txt, now)
		now = now.Add(200 * time.Millisecond)
	}
	return now
}

func TestMsUntilTick(t *testing.T) {
	for _, until := range []time.Duration{100 * time.Millisecond, time.Second, 3005 * time.Millisecond, 5900 * time.Millisecond} {
		for _, rev := range []bool{false, true} {
			v, txt := gaugeFor(until, rev)
			got, ok := msUntilTick(v, txt)
			if !ok {
				t.Fatalf("%v rev=%v: not ok", until, rev)
			}
			if d := got - until; d > 10*time.Millisecond || d < -10*time.Millisecond {
				t.Errorf("%v rev=%v: got %v", until, rev, got)
			}
		}
	}
	if _, ok := msUntilTick(500, "x"); ok {
		t.Error("unreadable text should not decode")
	}
}

func TestTickClockTrustsOnlyConsistentReadings(t *testing.T) {
	start := time.Now()
	e := newTestEngine()
	feedTicks(e, start, start.Add(2500*time.Millisecond))
	if _, ok := e.tick.nextTick(start.Add(2 * time.Second)); !ok {
		t.Fatal("consistent readings should give a trusted phase")
	}

	// Before the first tick after zoning Zeal reads 0 ("due now") every
	// frame. That never agrees with itself across a second.
	e2 := newTestEngine()
	now := start
	for i := 0; i <= 10; i++ {
		e2.setPipeServerTickAt("Osui", 0, "1", now)
		now = now.Add(200 * time.Millisecond)
	}
	if _, ok := e2.tick.nextTick(now); ok {
		t.Fatal("a gauge stuck at zero must not be trusted")
	}

	// Another character's pipe says nothing about the active player's tick.
	e3 := newTestEngine()
	now = start
	for i := 0; i <= 10; i++ {
		v, txt := gaugeFor(2*time.Second, false)
		e3.setPipeServerTickAt("Boxedalt", v, txt, now)
		now = now.Add(200 * time.Millisecond)
	}
	if _, ok := e3.tick.nextTick(now); ok {
		t.Fatal("another character's gauge must be ignored")
	}

	// Stale after the pipe goes quiet.
	if _, ok := e.tick.nextTick(start.Add(2*time.Second + tickAnchorTTL + time.Second)); ok {
		t.Fatal("phase should expire without fresh readings")
	}
}

func TestFormulaBuffSnapsToLastTickBeforeExpiry(t *testing.T) {
	start := time.Now().Truncate(time.Second)
	tickAt := start.Add(2500 * time.Millisecond)
	e := newTestEngine()
	// Formula expiry 10 minutes out, 4.2 s after a tick boundary.
	base := tickAt.Add(100*serverTickPeriod + 4200*time.Millisecond)
	tm := seedSelfBuff(e, "Spirit of Wolf", start, time.Minute, base.Sub(start))

	now := feedTicks(e, start, tickAt)
	want := tickAt.Add(100 * serverTickPeriod)
	if !near(tm.ExpiresAt, want) {
		t.Fatalf("ExpiresAt = %v after %v, want the tick at %v", tm.ExpiresAt, base, want)
	}
	snapped := tm.ExpiresAt

	// Further readings leave it where it is (snaps from the remembered
	// formula value, not from the snapped one).
	feedTicks(e, now, tickAt)
	if !tm.ExpiresAt.Equal(snapped) {
		t.Fatalf("snap drifted from %v to %v", snapped, tm.ExpiresAt)
	}

	// A recast rewrites ExpiresAt: the new value becomes the base.
	recast := tickAt.Add(200*serverTickPeriod + 300*time.Millisecond)
	tm.ExpiresAt = recast
	feedTicks(e, now.Add(3*time.Second), tickAt)
	if want := tickAt.Add(200 * serverTickPeriod); !near(tm.ExpiresAt, want) {
		t.Fatalf("after recast ExpiresAt = %v, want %v", tm.ExpiresAt, want)
	}
}

func TestPipeTicksUseServerTickPhase(t *testing.T) {
	start := time.Now()
	tickAt := start.Add(2500 * time.Millisecond)
	e := newTestEngine()
	tm := seedSelfBuff(e, "Aegolism", start, time.Minute, 30*time.Minute)
	now := feedTicks(e, start, tickAt)

	e.setPipeBuffSlotsAt("Osui", []PipeBuff{{Name: "Aegolism", Ticks: 50}}, now)
	next, _ := e.tick.nextTick(now)
	if want := next.Add(49 * serverTickPeriod); !tm.ExpiresAt.Equal(want) {
		t.Fatalf("ExpiresAt = %v, want the 50th tick %v", tm.ExpiresAt, want)
	}
	// Placed by tick count, so tick snapping must not move it again.
	feedTicks(e, now, tickAt)
	if want := next.Add(49 * serverTickPeriod); !tm.ExpiresAt.Equal(want) {
		t.Fatalf("tick snap moved a pipe-placed timer to %v", tm.ExpiresAt)
	}
}
