package spelltimer

import (
	"testing"
	"time"
)

// With keep-expired on, pruneExpired drops an overdue row once it passes the
// linger cap ExpiredKeepProvider returns for its spell and category, instead
// of the fixed keepExpiredMaxOverdue.
func TestPruneExpired_UsesExpiredKeepProvider(t *testing.T) {
	e := newTestEngine()
	e.keepExpiredFn = func() bool { return true }
	e.SetExpiredKeepProvider(func(spellName string, detrimental bool) time.Duration {
		switch {
		case spellName == "Quiescence":
			return time.Minute
		case detrimental:
			return 5 * time.Minute
		default:
			return 30 * time.Minute
		}
	})

	// Every timer expired 3 minutes ago.
	started := time.Now().Add(-4 * time.Minute)
	e.StartExternal("Quiescence", "buff", 60, 0, started, nil, 0, "Osui", "", false, "", false)
	e.StartExternal("Talisman of Kragg", "buff", 60, 0, started, nil, 0, "Osui", "", false, "", false)
	e.StartExternal("Cripple", "debuff", 60, 0, started, nil, 0, "a gnoll", "", false, "", false)

	e.pruneExpired()
	// The overridden buff is past its 1-minute cap: it takes the one-tick
	// grace, then goes on the next prune.
	if tm := e.timers[timerKey("Quiescence", "Osui")]; tm != nil {
		tm.expiryGraceUntil = time.Now().Add(-time.Second)
	}
	e.pruneExpired()

	if e.timers[timerKey("Quiescence", "Osui")] != nil {
		t.Error("Quiescence should be dropped after its 1-minute override")
	}
	if e.timers[timerKey("Talisman of Kragg", "Osui")] == nil {
		t.Error("buff within its 30-minute cap should still linger")
	}
	if e.timers[timerKey("Cripple", "a gnoll")] == nil {
		t.Error("detrimental within its 5-minute cap should still linger")
	}

	// Push the detrimental past its 5-minute cap.
	e.timers[timerKey("Cripple", "a gnoll")].ExpiresAt = time.Now().Add(-6 * time.Minute)
	e.pruneExpired()
	if tm := e.timers[timerKey("Cripple", "a gnoll")]; tm != nil {
		tm.expiryGraceUntil = time.Now().Add(-time.Second)
	}
	e.pruneExpired()
	if e.timers[timerKey("Cripple", "a gnoll")] != nil {
		t.Error("detrimental past its 5-minute cap should be dropped")
	}
}
