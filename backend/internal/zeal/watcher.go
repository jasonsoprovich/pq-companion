package zeal

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

const pollInterval = 5 * time.Second

// Watcher polls Zeal export files for changes and keeps an in-memory
// snapshot of the latest inventory and spellbook data.
// It broadcasts a WebSocket event whenever either file is updated.
type Watcher struct {
	cfgMgr *config.Manager
	hub    *ws.Hub

	mu           sync.RWMutex
	inventory    *Inventory
	spellbook    *Spellbook
	invModTime   time.Time
	spellModTime time.Time
}

// NewWatcher creates a Watcher. Call Start to begin polling.
func NewWatcher(cfgMgr *config.Manager, hub *ws.Hub) *Watcher {
	return &Watcher{
		cfgMgr: cfgMgr,
		hub:    hub,
	}
}

// Start begins the polling loop. It blocks until ctx is cancelled.
// Run it in a goroutine: go watcher.Start(ctx).
func (w *Watcher) Start(ctx context.Context) {
	// Do one check immediately so data is available on first API request.
	w.check()

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.check()
		}
	}
}

// Inventory returns the most recently parsed inventory, or nil if none.
func (w *Watcher) Inventory() *Inventory {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.inventory
}

// Spellbook returns the most recently parsed spellbook, or nil if none.
func (w *Watcher) Spellbook() *Spellbook {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.spellbook
}

// AllInventories scans the EQ directory for all character inventory exports and
// returns a combined response. Returns a non-configured response if EQPath is empty.
func (w *Watcher) AllInventories() (*AllInventoriesResponse, error) {
	cfg := w.cfgMgr.Get()
	resp := &AllInventoriesResponse{
		Configured: cfg.EQPath != "",
		Characters: []*Inventory{},
		SharedBank: []InventoryEntry{},
	}
	if cfg.EQPath == "" {
		return resp, nil
	}

	chars, sharedBank, err := ScanAllInventories(cfg.EQPath)
	if err != nil {
		return nil, err
	}
	if chars != nil {
		resp.Characters = chars
	}
	if sharedBank != nil {
		resp.SharedBank = sharedBank
	}
	return resp, nil
}

// check reads current config and re-parses files if their mod times have changed.
func (w *Watcher) check() {
	cfg := w.cfgMgr.Get()
	if cfg.EQPath == "" {
		return
	}

	character := cfg.Character
	if character == "" {
		character = logparser.ResolveActiveCharacter(cfg.EQPath)
		if character == "" {
			return
		}
	}

	invPath := InventoryPath(cfg.EQPath, character)
	spellPath := SpellbookPath(cfg.EQPath, character)

	w.checkInventory(invPath, character)
	w.checkSpellbook(spellPath, character)
}

func (w *Watcher) checkInventory(path, character string) {
	mt := ModTime(path)
	if mt.IsZero() {
		return // file does not exist yet
	}

	w.mu.RLock()
	unchanged := mt.Equal(w.invModTime)
	w.mu.RUnlock()

	if unchanged {
		return
	}

	inv, err := ParseInventory(path, character)
	if err != nil {
		slog.Warn("zeal: parse inventory", "path", path, "err", err)
		return
	}

	w.mu.Lock()
	w.inventory = inv
	w.invModTime = mt
	w.mu.Unlock()

	slog.Info("zeal: inventory updated", "character", character, "entries", len(inv.Entries))
	w.hub.Broadcast(ws.Event{Type: "zeal:inventory", Data: inv})
}

func (w *Watcher) checkSpellbook(path, character string) {
	mt := ModTime(path)
	if mt.IsZero() {
		return
	}

	w.mu.RLock()
	unchanged := mt.Equal(w.spellModTime)
	w.mu.RUnlock()

	if unchanged {
		return
	}

	sb, err := ParseSpellbook(path, character)
	if err != nil {
		slog.Warn("zeal: parse spellbook", "path", path, "err", err)
		return
	}

	w.mu.Lock()
	w.spellbook = sb
	w.spellModTime = mt
	w.mu.Unlock()

	slog.Info("zeal: spellbook updated", "character", character, "spells", len(sb.SpellIDs))
	w.hub.Broadcast(ws.Event{Type: "zeal:spellbook", Data: sb})
}
