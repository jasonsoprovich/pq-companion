package progress

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/character"
	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
)

// testdataDir points at the repo-root fixture directory shared across
// packages (see internal/trader/trader_test.go's testdataDir), containing
// real Osui-Quarmy.txt / Osui-Inventory.txt / Osui-Spellbook.txt exports.
const testdataDir = "../../../testdata"

func newTestRecorder(t *testing.T) (*Recorder, *Store, *character.Store) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "user.db")

	store, err := OpenStore(dbPath)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	charStore, err := character.OpenStore(dbPath)
	if err != nil {
		t.Fatalf("character.OpenStore: %v", err)
	}
	t.Cleanup(func() { charStore.Close() })

	if _, err := charStore.Create("Osui", -1, -1, 1); err != nil {
		t.Fatalf("Create character: %v", err)
	}

	cfgMgr, err := config.LoadFrom(filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatalf("config.LoadFrom: %v", err)
	}
	if err := cfgMgr.Modify(func(c *config.Config) { c.EQPath = testdataDir }); err != nil {
		t.Fatalf("modify config: %v", err)
	}

	return NewRecorder(cfgMgr, store, charStore), store, charStore
}

func TestRecorder_BuildSnapshot_ParsesRealFixture(t *testing.T) {
	r, _, _ := newTestRecorder(t)

	snap, ok := r.buildSnapshot(testdataDir, "Osui", time.Unix(1_700_000_000, 0))
	if !ok {
		t.Fatal("buildSnapshot returned ok=false")
	}
	if snap.Level != 60 {
		t.Errorf("Level = %d, want 60", snap.Level)
	}
	// On-person 200691 + bank 14209255 copper, from the fixture's General-Coin
	// / Bank-Coin rows.
	if want := int64(200691 + 14209255); snap.Copper != want {
		t.Errorf("Copper = %d, want %d", snap.Copper, want)
	}
	if snap.AARanks < 0 {
		t.Errorf("AARanks = %d, want >= 0", snap.AARanks)
	}
}

func TestRecorder_CaptureCharacter_SkipsUnchangedModTime(t *testing.T) {
	r, store, _ := newTestRecorder(t)

	r.captureCharacter(testdataDir, "Osui")
	first, ok, err := store.LatestSnapshot("Osui")
	if err != nil || !ok {
		t.Fatalf("LatestSnapshot after first capture: ok=%v err=%v", ok, err)
	}

	// Re-capturing without the file's mod time changing must not append a
	// second row — the recorder's poll loop calls this every tick.
	r.captureCharacter(testdataDir, "Osui")
	second, ok, err := store.LatestSnapshot("Osui")
	if err != nil || !ok {
		t.Fatalf("LatestSnapshot after second capture: ok=%v err=%v", ok, err)
	}
	if first.TakenAt != second.TakenAt {
		t.Errorf("second capture appended a new row: TakenAt %v -> %v", first.TakenAt, second.TakenAt)
	}

	baseline, ok, err := store.SnapshotAtOrBefore("Osui", time.Now().Add(time.Hour))
	if err != nil || !ok || baseline == nil {
		t.Fatalf("SnapshotAtOrBefore: baseline=%v ok=%v err=%v", baseline, ok, err)
	}
}

func TestRecorder_Start_HonorsInitialDelayAndContextCancel(t *testing.T) {
	r, store, _ := newTestRecorder(t)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		r.Start(ctx)
		close(done)
	}()

	// Cancel immediately — Start should return promptly rather than blocking
	// for recorderInitialDelay, and must not have scanned yet.
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return promptly after context cancellation")
	}

	if _, ok, _ := store.LatestSnapshot("Osui"); ok {
		t.Error("Start scanned before its initial delay elapsed")
	}
}
