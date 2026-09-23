package popflag

import (
	"testing"
	"time"
)

func TestConsumerPopFlagsBlock(t *testing.T) {
	s := openTempStore(t)
	const char = "Drennik"
	c := NewConsumer(s, func() string { return char })

	base := time.Unix(1000, 0)
	for i, line := range []string{
		"=== Tier 1 Progression ===",
		"Mavuin's case: Mavuin's case is complete",
		"Seventh Hammer access: Unlocked",
	} {
		c.HandleLine(base.Add(time.Duration(i)*time.Millisecond), line)
	}
	// A non-matching line ends the block immediately (no need to wait for the
	// idle timer).
	c.HandleLine(base.Add(10*time.Millisecond), "You slash a gnoll for 150 points of damage.")

	rows := doneByID(t, s, char)
	if !rows["poj_trial_mark"].Done || rows["poj_trial_mark"].Source != SourcePopflags {
		t.Errorf("poj_trial_mark = %+v, want done/popflags", rows["poj_trial_mark"])
	}
	if !rows["poj_mark_of_justice"].Done {
		t.Errorf("poj_mark_of_justice (seventh) should be done")
	}
}

func TestConsumerSeerAndPopFlagsIndependent(t *testing.T) {
	s := openTempStore(t)
	const char = "Ohmalie"
	c := NewConsumer(s, func() string { return char })

	base := time.Unix(2000, 0)
	// A Seer line followed immediately by a #popflags line: the Seer buffer
	// should NOT swallow the popflags line, and vice versa — both blocks must
	// commit their own state (mavuin from the Seer line, bertox_key from the
	// popflags line), not just whichever buffer matched last.
	c.HandleLine(base, "Mavuin is grateful to you for taking his case before the Tribunal.")
	c.HandleLine(base.Add(time.Millisecond), "=== Tier 2 Progression ===")
	c.HandleLine(base.Add(2*time.Millisecond), "Lower Crypt access: Unlocked")
	c.HandleLine(base.Add(3*time.Millisecond), "You have entered The Plane of Knowledge.")

	rows := doneByID(t, s, char)
	// The popflags sync re-derives the WHOLE merged qglobal snapshot (mavuin=3
	// from the seer commit survives via GetSnapshot), so poj_mavuin_return
	// ends up re-tagged source=popflags — that's the intended "latest reading
	// wins" precedence, not a regression: it's still correctly Done.
	if row := rows["poj_mavuin_return"]; !row.Done {
		t.Errorf("poj_mavuin_return = %+v, want done (mavuin=3 from the seer line)", row)
	}
	if row := rows["cod_carpryn"]; !row.Done || row.Source != SourcePopflags {
		t.Errorf("cod_carpryn = %+v, want done/popflags", row)
	}
}

func TestConsumerPopFlagsIdleFlush(t *testing.T) {
	s := openTempStore(t)
	const char = "Vethra"
	c := NewConsumer(s, func() string { return char })
	defer c.Shutdown()

	c.HandleLine(time.Now(), "=== Plane of Time ===")
	c.HandleLine(time.Now(), "Plane of Time access: Unlocked")

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if doneByID(t, s, char)["potime"].Done {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("potime not committed after idle flush timeout")
}

func TestConsumerNoActiveCharacterSuppressesSnapshot(t *testing.T) {
	s := openTempStore(t)
	c := NewConsumer(s, func() string { return "" })

	c.HandleLine(time.Now(), "=== Plane of Time ===")
	c.HandleLine(time.Now(), "Plane of Time access: Unlocked")
	c.flushPopFlags()

	chars, err := s.Characters()
	if err != nil {
		t.Fatalf("Characters: %v", err)
	}
	if len(chars) != 0 {
		t.Errorf("expected no rows written with no active character, got %v", chars)
	}
}
