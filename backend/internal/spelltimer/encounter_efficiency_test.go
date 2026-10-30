package spelltimer

import (
	"testing"
	"time"
)

func TestEncounterNoOpSnapshot(t *testing.T) {
	for _, kind := range []string{"missing_stop", "missing_owner", "context_without_opt_in", "empty_retirement_owner"} {
		t.Run(kind, func(t *testing.T) {
			e := newTestEngine()
			at := time.Now()
			e.StartExternal("Legacy", "buff", 900, 0, at, nil, 0, "", "", false, "", false)
			if !e.StartOwnedExternal("owner", "Owned", 350, 0, at, nil, 0, "Alpha", "", false, "", true, false) {
				t.Fatal("seed rejected")
			}
			calls := 0
			e.keepExpiredFn = func() bool { calls++; return false }
			switch kind {
			case "missing_stop":
				e.StopOwnedExternal("owner", "Owned", "Other")
			case "missing_owner":
				e.RetireOwnedExternal("missing", false)
			case "context_without_opt_in":
				e.RetireOwnedExternal("", true)
			case "empty_retirement_owner":
				e.RetireOwnedExternal("", false)
			}
			if calls != 0 {
				t.Fatalf("no-op built %d snapshots; want zero", calls)
			}
			if len(e.timers) != 2 {
				t.Fatal("no-op changed real rows")
			}
			e.StopOwnedExternal("owner", "Owned", "Alpha")
			if calls != 1 || len(e.timers) != 1 {
				t.Fatal("real removal must snapshot once and preserve legacy row", calls, len(e.timers))
			}
		})
	}
	t.Run("changed_retirement", func(t *testing.T) {
		e := newTestEngine()
		at := time.Now()
		e.StartOwnedExternal("owner", "Owned", 350, 0, at, nil, 0, "Alpha", "", false, "", true, true)
		calls := 0
		e.keepExpiredFn = func() bool { calls++; return false }
		e.RetireOwnedExternal("", true)
		if calls != 1 || len(e.timers) != 0 {
			t.Fatal("changed retirement must snapshot once", calls, len(e.timers))
		}
	})
}
