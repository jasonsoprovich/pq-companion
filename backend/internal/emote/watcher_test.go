package emote

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// TestWatcherSwitchingEQPathDoesNotFalselyFlagExternalChange is a regression
// test: pointing EQPath at a second directory whose spells_en.txt is
// byte-identical to the one just captured as the pristine default (e.g. the
// user changes their configured EQ directory) must NOT be treated as a
// server patch — the file's mtime differs (it's a distinct file on disk) but
// its content does not, so there is nothing to reconcile.
func TestWatcherSwitchingEQPathDoesNotFalselyFlagExternalChange(t *testing.T) {
	content := readFixture(t)
	dirA := t.TempDir()
	dirB := t.TempDir()
	if err := os.WriteFile(filepath.Join(dirA, "spells_en.txt"), []byte(content), 0o644); err != nil {
		t.Fatalf("write dirA fixture: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dirB, "spells_en.txt"), []byte(content), 0o644); err != nil {
		t.Fatalf("write dirB fixture: %v", err)
	}

	cfgMgr, err := config.LoadFrom(filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatalf("config.LoadFrom: %v", err)
	}
	if err := cfgMgr.Modify(func(c *config.Config) { c.EQPath = dirA }); err != nil {
		t.Fatalf("set EQPath to dirA: %v", err)
	}

	store, err := OpenStore(filepath.Join(t.TempDir(), "user.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	service := NewService(store, cfgMgr, filepath.Join(t.TempDir(), "spell-emotes"), newTestGameDB(t))
	watcher := NewWatcher(cfgMgr, ws.NewHub(), service)

	// First poll: no default backup exists yet, so this is captured as the
	// pristine default rather than an external change.
	watcher.check()
	st, err := service.Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.PendingExternalChange {
		t.Fatal("initial default capture must not be flagged as an external change")
	}
	if !st.HasDefaultBackup {
		t.Fatal("expected default backup to be captured on first sighting")
	}

	// Switch EQPath to a different directory holding byte-identical content.
	if err := cfgMgr.Modify(func(c *config.Config) { c.EQPath = dirB }); err != nil {
		t.Fatalf("set EQPath to dirB: %v", err)
	}
	watcher.check()

	st, err = service.Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if st.PendingExternalChange {
		t.Fatal("switching to a directory with byte-identical content must not flag an external change")
	}
}

// TestWatcherDetectsGenuineExternalChange is the positive-case companion to
// the regression test above: content that actually differs from the last
// known write must still be flagged.
func TestWatcherDetectsGenuineExternalChange(t *testing.T) {
	content := readFixture(t)
	dir := t.TempDir()
	livePath := filepath.Join(dir, "spells_en.txt")
	if err := os.WriteFile(livePath, []byte(content), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	cfgMgr, err := config.LoadFrom(filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatalf("config.LoadFrom: %v", err)
	}
	if err := cfgMgr.Modify(func(c *config.Config) { c.EQPath = dir }); err != nil {
		t.Fatalf("set EQPath: %v", err)
	}

	store, err := OpenStore(filepath.Join(t.TempDir(), "user.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	service := NewService(store, cfgMgr, filepath.Join(t.TempDir(), "spell-emotes"), newTestGameDB(t))
	watcher := NewWatcher(cfgMgr, ws.NewHub(), service)

	watcher.check() // initial capture

	newEmote := " has had Divine Aura cast on them."
	patched := applyOverrides(content, map[int]OverrideRow{207: {SpellID: 207, CastOnOther: &newEmote}})
	if err := os.WriteFile(livePath, []byte(patched), 0o644); err != nil {
		t.Fatalf("simulate external patch: %v", err)
	}
	watcher.check()

	st, err := service.Status()
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !st.PendingExternalChange {
		t.Fatal("expected a genuinely changed file to be flagged as an external change")
	}
}
