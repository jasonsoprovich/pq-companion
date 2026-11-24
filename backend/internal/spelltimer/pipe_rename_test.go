package spelltimer

import (
	"testing"
	"time"
)

func seedBuffRow(e *Engine, name string, spellID int, target string, start time.Time, dur time.Duration) *ActiveTimer {
	tm := &ActiveTimer{
		ID: timerKey(name, target), SpellName: name, SpellID: spellID, TargetName: target,
		Category: CategoryBuff, StartsAt: start, ExpiresAt: start.Add(dur),
	}
	tm.DurationSeconds = dur.Seconds()
	e.timers[tm.ID] = tm
	return tm
}

// Another enchanter's Guard of Druzzil prints Group Resist Magic's land text;
// the window names the real buff, and the group burst follows the player's.
func TestPipeRenamesSharedLandTextBuff(t *testing.T) {
	tests := []struct {
		name            string
		logName, pipe   string
		logID           int
		sibling         string // the other member's row from the same cast
		wantSiblingName string
	}{
		{"Guard of Druzzil", "Group Resist Magic", "Guard of Druzzil", 72, "Resist Magic", "Guard of Druzzil"},
		{"Voice of Quellious", "Koadic's Endless Intellect", "Voice of Quellious", 2570,
			"Koadic's Endless Intellect", "Voice of Quellious"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e, _ := levelEngine(t, 13, 65)
			now := time.Now()
			start := now.Add(-3 * time.Second)
			self := seedBuffRow(e, tc.logName, tc.logID, "Tester", start, 36*time.Minute)
			sib := seedBuffRow(e, tc.sibling, tc.logID, "Astrael", start.Add(time.Second), 36*time.Minute)
			late := seedBuffRow(e, tc.sibling, tc.logID, "Tiliki", start.Add(-time.Hour), 2*time.Hour)

			e.setPipeBuffSlotsAt("Tester", []PipeBuff{{Name: tc.pipe, Ticks: 800}}, now)

			if self.SpellName != tc.pipe || e.timers[timerKey(tc.pipe, "Tester")] != self {
				t.Fatalf("self row = %q (keys %v), want %q", self.SpellName, keysOf(e.timers), tc.pipe)
			}
			if got := self.ExpiresAt.Sub(now); got != 800*eqTickDuration-eqTickDuration/2 {
				t.Fatalf("self not synced to the window's ticks: %v left", got)
			}
			if sib.SpellName != tc.wantSiblingName || !sib.ExpiresAt.Equal(self.ExpiresAt) {
				t.Fatalf("sibling = %q expiring %v, want %q at the player's expiry",
					sib.SpellName, sib.ExpiresAt, tc.wantSiblingName)
			}
			if late.SpellName != tc.sibling {
				t.Fatalf("an older cast's row was renamed to %q", late.SpellName)
			}
		})
	}
}

func TestPipeRenameLeavesCoveredAndUnrelatedBuffs(t *testing.T) {
	e, _ := levelEngine(t, 13, 65)
	now := time.Now()
	start := now.Add(-3 * time.Second)
	// The window's Guard of Druzzil already has its own timer: nothing to
	// rename the Group Resist Magic row to.
	grm := seedBuffRow(e, "Group Resist Magic", 72, "Tester", start, 36*time.Minute)
	seedBuffRow(e, "Guard of Druzzil", 3242, "Tester", start, 50*time.Minute)
	// A buff that simply hasn't reached the window shares no text with it.
	sow := seedBuffRow(e, "Spirit of Wolf", 278, "Tester", start, 36*time.Minute)

	e.setPipeBuffSlotsAt("Tester", []PipeBuff{{Name: "Guard of Druzzil", Ticks: 800}}, now)
	if grm.SpellName != "Group Resist Magic" || sow.SpellName != "Spirit of Wolf" {
		t.Fatalf("renamed: %q, %q", grm.SpellName, sow.SpellName)
	}
}
