package zeal_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/character"
	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
	"github.com/jasonsoprovich/pq-companion/backend/internal/zeal"
)

// RefreshAllPersonas runs on every character list call; an unchanged Quarmy
// export must not be re-synced, and a changed one must be.
func TestRefreshAllPersonasSkipsUnchangedExport(t *testing.T) {
	eqDir := t.TempDir()
	// testdata/ is gitignored, so CI checkouts don't have the fixture.
	src, err := os.ReadFile("../../../testdata/TAKPv22/Osui-Quarmy_pq.proj.txt")
	if err != nil {
		t.Skipf("testdata fixture not present: %v", err)
	}
	quarmy := filepath.Join(eqDir, "Osui-Quarmy_pq.proj.txt")
	if err := os.WriteFile(quarmy, src, 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := character.OpenStore(filepath.Join(t.TempDir(), "user.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	c, err := store.Create("Osui", 0, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	mgr, err := config.LoadFrom(filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	cfg := mgr.Get()
	cfg.EQPath = eqDir
	if err := mgr.Update(cfg); err != nil {
		t.Fatal(err)
	}
	w := zeal.NewWatcher(mgr, ws.NewHub(), store)
	cha := func() int {
		got, _, err := store.GetByName("Osui")
		if err != nil {
			t.Fatal(err)
		}
		return got.BaseCHA
	}

	w.RefreshAllPersonas()
	want := cha()
	if want <= 0 {
		t.Fatalf("BaseCHA after first sync = %d, want > 0", want)
	}

	// Blank the row; an unchanged export is skipped, so it stays blank.
	if err := store.UpdateStats(c.ID, 0, 0, 0, 0, 0, 0, 0); err != nil {
		t.Fatal(err)
	}
	w.RefreshAllPersonas()
	if got := cha(); got != 0 {
		t.Fatalf("BaseCHA after unchanged refresh = %d, want 0 (skipped)", got)
	}

	// A newer export syncs again.
	later := time.Now().Add(time.Minute)
	if err := os.Chtimes(quarmy, later, later); err != nil {
		t.Fatal(err)
	}
	w.RefreshAllPersonas()
	if got := cha(); got != want {
		t.Fatalf("BaseCHA after changed export = %d, want %d", got, want)
	}
}
