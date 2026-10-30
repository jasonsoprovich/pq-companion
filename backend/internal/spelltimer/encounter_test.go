package spelltimer

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestEncounterRealRowAtomicAdmission(t *testing.T) {
	e := newTestEngine()
	now := time.Now()
	var accepts atomic.Int32
	var wg sync.WaitGroup
	for range 32 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if e.StartOwnedExternal("owner", "Name", 7380, 0, now, nil, 0, "Alpha", "", false, "", true, true) {
				accepts.Add(1)
			}
		}()
	}
	wg.Wait()
	if accepts.Load() != 1 || len(e.GetState().Timers) != 1 {
		t.Fatal("non-atomic real-row admission", accepts.Load())
	}
	e.StartExternal("Name", "custom", 100, 0, now, nil, 0, "Alpha", "", false, "", false)
	e.StartOwnedExternal("other", "Name", 100, 0, now, nil, 0, "Alpha", "", false, "", true, false)
	e.StartOwnedExternal("owner", "Name", 100, 0, now, nil, 0, "Beta", "", false, "", true, true)
	if len(e.GetState().Timers) != 4 {
		t.Fatal("owner/key/target identities collided")
	}
	e.StopExternal("Name", 0)
	if len(e.GetState().Timers) != 3 {
		t.Fatal("legacy stop crossed managed identity")
	}
	e.SetExternalRepeat("Name", "Alpha", 5)
	for _, row := range e.GetState().Timers {
		if row.Repeat {
			t.Fatal("legacy repeat touched managed timer")
		}
	}
	e.StopOwnedExternal("owner", "Name", "Beta")
	if len(e.GetState().Timers) != 2 {
		t.Fatal("owned stop crossed target")
	}
	e.RetireOwnedExternal("", true)
	if len(e.GetState().Timers) != 1 {
		t.Fatal("context crossed unopted owner")
	}
	// Expired overdue rows are not active latches, even with keep-expired on.
	e.keepExpiredFn = func() bool { return true }
	key := ownedTimerKey("other", "Name", "Alpha")
	e.mu.Lock()
	e.timers[key].ExpiresAt = time.Now().Add(-time.Second)
	e.mu.Unlock()
	if !e.StartOwnedExternal("other", "Name", 100, 0, time.Now(), nil, 0, "Alpha", "", false, "", true, false) {
		t.Fatal("expired real row blocked immediate retry")
	}
	e.RetireOwnedExternal("other", false)
	if len(e.GetState().Timers) != 0 {
		t.Fatal("definition retirement orphaned row")
	}
}
