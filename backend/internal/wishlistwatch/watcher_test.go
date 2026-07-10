package wishlistwatch

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/trigger"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// fakeBroadcaster records every event passed to Broadcast, so tests can
// assert on what a fire actually produced without a live WebSocket client.
type fakeBroadcaster struct {
	events []ws.Event
}

func (f *fakeBroadcaster) Broadcast(e ws.Event) {
	f.events = append(f.events, e)
}

// testWorld is a small in-memory wishlist the watcher's closures read from,
// so tests don't need a real character.Store or db.DB.
type testWorld struct {
	chars     []CharacterInfo
	wishlists map[int][]WishlistEntry
	items     map[int]ItemInfo
}

func newTestWatcher(t *testing.T, w *testWorld) (*Watcher, *fakeBroadcaster, *config.Manager) {
	t.Helper()
	cfgMgr, err := config.LoadFrom(filepath.Join(t.TempDir(), "config.yaml"))
	if err != nil {
		t.Fatalf("config.LoadFrom: %v", err)
	}

	fb := &fakeBroadcaster{}
	watcher := &Watcher{
		hub: fb,
		cfg: cfgMgr,
		listChars: func() ([]CharacterInfo, error) {
			return w.chars, nil
		},
		listWishlist: func(characterID int) ([]WishlistEntry, error) {
			return w.wishlists[characterID], nil
		},
		lookupItem: func(itemID int) (ItemInfo, bool) {
			info, ok := w.items[itemID]
			return info, ok
		},
		lastFired: make(map[int]time.Time),
	}
	return watcher, fb, cfgMgr
}

func enableWishlistWatch(t *testing.T, cfgMgr *config.Manager, mutate func(*config.WishlistWatchSettings)) {
	t.Helper()
	if err := cfgMgr.Modify(func(c *config.Config) {
		c.Preferences.WishlistWatch.Enabled = true
		mutate(&c.Preferences.WishlistWatch)
	}); err != nil {
		t.Fatalf("Modify config: %v", err)
	}
}

func TestWatcher_OwnWishlistFires(t *testing.T) {
	world := &testWorld{
		chars: []CharacterInfo{{ID: 1, Name: "Khura"}},
		wishlists: map[int][]WishlistEntry{
			1: {{CharacterID: 1, ItemID: 100}},
		},
		items: map[int]ItemInfo{100: {Name: "Robe of the Lost Circle"}},
	}
	watcher, fb, cfgMgr := newTestWatcher(t, world)
	watcher.activeChar = func() string { return "Khura" }
	enableWishlistWatch(t, cfgMgr, func(s *config.WishlistWatchSettings) {
		s.OverlayEnabled = true
	})
	watcher.Rebuild()

	watcher.HandleLine("You have looted a Robe of the Lost Circle.")

	if len(fb.events) != 1 {
		t.Fatalf("expected 1 fired event, got %d", len(fb.events))
	}
	fired, ok := fb.events[0].Data.(trigger.TriggerFired)
	if !ok {
		t.Fatalf("event data is %T, not trigger.TriggerFired", fb.events[0].Data)
	}
	if fired.MatchedLine != "Robe of the Lost Circle is on Khura's wishlist" {
		t.Errorf("unexpected alert text: %q", fired.MatchedLine)
	}
}

func TestWatcher_OtherCharGatedByToggle(t *testing.T) {
	world := &testWorld{
		chars: []CharacterInfo{{ID: 1, Name: "Khura"}, {ID: 2, Name: "Grokii"}},
		wishlists: map[int][]WishlistEntry{
			2: {{CharacterID: 2, ItemID: 200}},
		},
		items: map[int]ItemInfo{200: {Name: "Journeyman's Boots"}},
	}
	watcher, fb, cfgMgr := newTestWatcher(t, world)
	watcher.activeChar = func() string { return "Khura" }
	enableWishlistWatch(t, cfgMgr, func(s *config.WishlistWatchSettings) {
		s.OverlayEnabled = true
	})
	watcher.Rebuild()

	watcher.HandleLine("Someone auctions, 'WTS Journeyman's Boots 500pp'")
	if len(fb.events) != 0 {
		t.Fatalf("expected other-character item to be suppressed by default, got %d events", len(fb.events))
	}

	if err := cfgMgr.Modify(func(c *config.Config) {
		c.Preferences.WishlistWatch.IncludeOtherChars = true
	}); err != nil {
		t.Fatalf("Modify config: %v", err)
	}

	watcher.HandleLine("Someone auctions, 'WTS Journeyman's Boots 500pp'")
	if len(fb.events) != 1 {
		t.Fatalf("expected other-character item to fire once IncludeOtherChars is on, got %d events", len(fb.events))
	}
	fired := fb.events[0].Data.(trigger.TriggerFired)
	if fired.MatchedLine != "Journeyman's Boots is on Grokii's wishlist" {
		t.Errorf("unexpected alert text: %q", fired.MatchedLine)
	}
}

func TestWatcher_WordBoundaryNoSubstringFalsePositive(t *testing.T) {
	world := &testWorld{
		chars: []CharacterInfo{{ID: 1, Name: "Khura"}},
		wishlists: map[int][]WishlistEntry{
			1: {{CharacterID: 1, ItemID: 100}},
		},
		items: map[int]ItemInfo{100: {Name: "Cloak"}},
	}
	watcher, fb, cfgMgr := newTestWatcher(t, world)
	watcher.activeChar = func() string { return "Khura" }
	enableWishlistWatch(t, cfgMgr, func(s *config.WishlistWatchSettings) {
		s.OverlayEnabled = true
	})
	watcher.Rebuild()

	watcher.HandleLine("You enter the Cloakroom.")
	if len(fb.events) != 0 {
		t.Fatalf("expected no match for a substring inside a longer word, got %d events", len(fb.events))
	}

	watcher.HandleLine("You receive a Cloak from the corpse.")
	if len(fb.events) != 1 {
		t.Fatalf("expected a whole-word match to fire, got %d events", len(fb.events))
	}
}

func TestWatcher_LongestNameWinsOverShorterWishlistedSubstring(t *testing.T) {
	world := &testWorld{
		chars: []CharacterInfo{{ID: 1, Name: "Khura"}},
		wishlists: map[int][]WishlistEntry{
			1: {
				{CharacterID: 1, ItemID: 100},
				{CharacterID: 1, ItemID: 101},
			},
		},
		items: map[int]ItemInfo{
			100: {Name: "Robe"},
			101: {Name: "Robe of the Lost Circle"},
		},
	}
	watcher, fb, cfgMgr := newTestWatcher(t, world)
	watcher.activeChar = func() string { return "Khura" }
	enableWishlistWatch(t, cfgMgr, func(s *config.WishlistWatchSettings) {
		s.OverlayEnabled = true
	})
	watcher.Rebuild()

	watcher.HandleLine("You have looted a Robe of the Lost Circle.")

	if len(fb.events) != 1 {
		t.Fatalf("expected exactly 1 event (longest name only), got %d", len(fb.events))
	}
	fired := fb.events[0].Data.(trigger.TriggerFired)
	if fired.TriggerID != "system:wishlist:101" {
		t.Errorf("expected the longer item (101) to win, fired for %q instead", fired.TriggerID)
	}
}

func TestWatcher_CooldownSuppressesRepeat(t *testing.T) {
	world := &testWorld{
		chars: []CharacterInfo{{ID: 1, Name: "Khura"}},
		wishlists: map[int][]WishlistEntry{
			1: {{CharacterID: 1, ItemID: 100}},
		},
		items: map[int]ItemInfo{100: {Name: "Journeyman's Boots"}},
	}
	watcher, fb, cfgMgr := newTestWatcher(t, world)
	watcher.activeChar = func() string { return "Khura" }
	enableWishlistWatch(t, cfgMgr, func(s *config.WishlistWatchSettings) {
		s.OverlayEnabled = true
		s.CooldownSecs = 60
	})
	watcher.Rebuild()

	watcher.HandleLine("You have looted a Journeyman's Boots.")
	watcher.HandleLine("You have looted a Journeyman's Boots.")

	if len(fb.events) != 1 {
		t.Fatalf("expected the second immediate match to be suppressed by cooldown, got %d events", len(fb.events))
	}
}

func TestWatcher_DisabledDoesNothing(t *testing.T) {
	world := &testWorld{
		chars: []CharacterInfo{{ID: 1, Name: "Khura"}},
		wishlists: map[int][]WishlistEntry{
			1: {{CharacterID: 1, ItemID: 100}},
		},
		items: map[int]ItemInfo{100: {Name: "Journeyman's Boots"}},
	}
	watcher, fb, _ := newTestWatcher(t, world)
	watcher.activeChar = func() string { return "Khura" }
	// Note: enableWishlistWatch is NOT called, so Enabled stays false.
	watcher.Rebuild()

	watcher.HandleLine("You have looted a Journeyman's Boots.")

	if len(fb.events) != 0 {
		t.Fatalf("expected no events while disabled, got %d", len(fb.events))
	}
}

func TestWatcher_NoActionsConfiguredFiresNothing(t *testing.T) {
	world := &testWorld{
		chars: []CharacterInfo{{ID: 1, Name: "Khura"}},
		wishlists: map[int][]WishlistEntry{
			1: {{CharacterID: 1, ItemID: 100}},
		},
		items: map[int]ItemInfo{100: {Name: "Journeyman's Boots"}},
	}
	watcher, fb, cfgMgr := newTestWatcher(t, world)
	watcher.activeChar = func() string { return "Khura" }
	// Enabled, but no overlay/TTS/sound action turned on.
	enableWishlistWatch(t, cfgMgr, func(s *config.WishlistWatchSettings) {})
	watcher.Rebuild()

	watcher.HandleLine("You have looted a Journeyman's Boots.")

	if len(fb.events) != 0 {
		t.Fatalf("expected no broadcast when no alert action is enabled, got %d", len(fb.events))
	}
}
