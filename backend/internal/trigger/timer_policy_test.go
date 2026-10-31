package trigger

import (
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/spelltimer"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// policyEngine wires a trigger engine to a real spelltimer engine so the
// encounter timer options are tested against the actual timer rows.
// *char is the active character the engine sees on each line.
func policyEngine(t *testing.T, char *string, triggers ...*Trigger) (*Engine, *spelltimer.Engine) {
	t.Helper()
	store := openTestStore(t)
	for _, tr := range triggers {
		if err := store.Insert(tr); err != nil {
			t.Fatalf("Insert %s: %v", tr.Name, err)
		}
	}
	timers := spelltimer.NewEngine(ws.NewHub(), nil, nil, nil, nil, nil, nil, nil, nil)
	e := NewEngine(store, ws.NewHub(), timers, func() string { return *char })
	e.Reload()
	return e, timers
}

func policyTrigger(name, pattern string, ifIdle, clearOnContext bool) *Trigger {
	return &Trigger{
		ID:                        "policy-" + name,
		Name:                      name,
		Enabled:                   true,
		Pattern:                   pattern,
		WornOffPattern:            "^" + name + " ends\\.$",
		TimerType:                 TimerTypeCustom,
		TimerDurationSecs:         600,
		TimerIfIdle:               ifIdle,
		TimerClearOnContextChange: clearOnContext,
	}
}

// timerExpiry returns the expiry of the named timer, or the zero time when
// it isn't running.
func timerExpiry(timers *spelltimer.Engine, name string) time.Time {
	for _, row := range timers.GetState().Timers {
		if row.SpellName == name {
			return row.ExpiresAt
		}
	}
	return time.Time{}
}

func firedCount(e *Engine, name string) int {
	n := 0
	for _, f := range e.GetHistory() {
		if f.TriggerName == name {
			n++
		}
	}
	return n
}

func TestTimerIfIdle(t *testing.T) {
	tests := []struct {
		name        string
		ifIdle      bool
		wantRestart bool // the second opening line moves the deadline
		wantFired   int  // trigger firings after both opening lines
	}{
		{"if idle keeps the first countdown", true, false, 1},
		{"default restarts the countdown", false, true, 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			char := "Osui"
			e, timers := policyEngine(t, &char, policyTrigger("Event", `^The event begins\.$`, tc.ifIdle, false))
			// Backdated so the second line falls outside the timer engine's
			// same-name dedup window, which would otherwise merge it.
			start := time.Now().Add(-2 * time.Minute)
			e.Handle(start, "The event begins.")
			first := timerExpiry(timers, "Event")
			if first.IsZero() {
				t.Fatal("first opening line didn't start the timer")
			}
			e.Handle(start.Add(time.Minute), "The event begins.")
			second := timerExpiry(timers, "Event")
			if restarted := !second.Equal(first); restarted != tc.wantRestart {
				t.Errorf("restarted = %v, want %v (first %v, second %v)", restarted, tc.wantRestart, first, second)
			}
			if got := firedCount(e, "Event"); got != tc.wantFired {
				t.Errorf("fired %d times, want %d", got, tc.wantFired)
			}
		})
	}
}

func TestTimerIfIdleAllowsStartAfterStop(t *testing.T) {
	char := "Osui"
	e, timers := policyEngine(t, &char, policyTrigger("Event", `^The event begins\.$`, true, false))
	start := time.Now()
	e.Handle(start, "The event begins.")
	e.Handle(start.Add(time.Second), "Event ends.")
	if !timerExpiry(timers, "Event").IsZero() {
		t.Fatal("worn-off line didn't stop the timer")
	}
	e.Handle(start.Add(2*time.Second), "The event begins.")
	if timerExpiry(timers, "Event").IsZero() {
		t.Fatal("a new attempt after the stop didn't start the timer")
	}
	if got := firedCount(e, "Event"); got != 2 {
		t.Errorf("fired %d times, want 2", got)
	}
}

func TestTimerClearOnContextChange(t *testing.T) {
	tests := []struct {
		name   string
		change func(e *Engine, char *string, at time.Time)
	}{
		{"zone change", func(e *Engine, _ *string, at time.Time) {
			e.Handle(at, "You have entered The Plane of Water.")
		}},
		{"character change", func(e *Engine, char *string, at time.Time) {
			*char = "Nariana"
			e.Handle(at, "You say, 'hello'")
		}},
		{"replay start", func(e *Engine, _ *string, _ time.Time) {
			e.SetReplaying(true)
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			char := "Osui"
			e, timers := policyEngine(t, &char,
				policyTrigger("Event", `^The event begins\.$`, false, true),
				policyTrigger("Other", `^Something else\.$`, false, false),
			)
			start := time.Now()
			e.Handle(start, "The event begins.")
			e.Handle(start, "Something else.")
			tc.change(e, &char, start.Add(time.Second))
			if !timerExpiry(timers, "Event").IsZero() {
				t.Error("opted-in timer survived the context change")
			}
			if timerExpiry(timers, "Other").IsZero() {
				t.Error("timer without the option was cleared")
			}
		})
	}
}

func TestTimerClearOnContextNotOnNonZoneEnteredLine(t *testing.T) {
	char := "Osui"
	e, timers := policyEngine(t, &char, policyTrigger("Event", `^The event begins\.$`, false, true))
	start := time.Now()
	e.Handle(start, "The event begins.")
	e.Handle(start.Add(time.Second), "You have entered an area where levitation effects do not function.")
	if timerExpiry(timers, "Event").IsZero() {
		t.Error("a non-zone \"You have entered\" line cleared the timer")
	}
}

func TestTimerClearOnContextZoneThenStart(t *testing.T) {
	// The zone-in line and the event's opening line arrive in the same
	// poll: the old timer clears and the new one starts.
	char := "Osui"
	e, timers := policyEngine(t, &char, policyTrigger("Event", `^The event begins\.$`, true, true))
	start := time.Now()
	e.Handle(start, "The event begins.")
	e.Handle(start.Add(time.Second), "You have entered The Plane of Water.")
	e.Handle(start.Add(2*time.Second), "The event begins.")
	got := timerExpiry(timers, "Event")
	if want := start.Add(2*time.Second + 600*time.Second); !got.Equal(want) {
		t.Errorf("expiry = %v, want the new attempt's %v", got, want)
	}
}

func TestNormalizeTimerPolicy(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Trigger)
		want bool // options survive
	}{
		{"custom timer keeps options", func(*Trigger) {}, true},
		{"stacked timer clears options", func(tr *Trigger) { tr.TimerStack = true }, false},
		{"repeating timer clears options", func(tr *Trigger) { tr.TimerRepeat = true }, false},
		{"buff timer clears options", func(tr *Trigger) { tr.TimerType = TimerTypeBuff }, false},
		{"pipe trigger clears options", func(tr *Trigger) { tr.Source = SourcePipe }, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tr := policyTrigger("Event", `^x$`, true, true)
			tc.edit(tr)
			tr.NormalizeTimerPolicy()
			if tr.TimerIfIdle != tc.want || tr.TimerClearOnContextChange != tc.want {
				t.Errorf("if_idle=%v clear=%v, want both %v", tr.TimerIfIdle, tr.TimerClearOnContextChange, tc.want)
			}
		})
	}
}

func TestTimerPolicyStoreRoundTrip(t *testing.T) {
	store := openTestStore(t)
	tr := policyTrigger("Event", `^x$`, true, true)
	if err := store.Insert(tr); err != nil {
		t.Fatal(err)
	}
	got, err := store.Get(tr.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.TimerIfIdle || !got.TimerClearOnContextChange {
		t.Errorf("round trip lost options: if_idle=%v clear=%v", got.TimerIfIdle, got.TimerClearOnContextChange)
	}
}
