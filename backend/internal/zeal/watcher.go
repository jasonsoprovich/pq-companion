package zeal

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/character"
	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

const pollInterval = 5 * time.Second

// Watcher polls Zeal export files for changes and keeps an in-memory
// snapshot of the latest inventory, spellbook, and quarmy data.
// It broadcasts a WebSocket event whenever any file is updated.
type Watcher struct {
	cfgMgr    *config.Manager
	hub       *ws.Hub
	charStore *character.Store

	// onQuarmyChanged is invoked after a successful Quarmy refresh. Used by
	// the spell timer engine to invalidate its cached buffmod contributors.
	// nil-safe.
	onQuarmyChanged func(charName string)

	mu               sync.RWMutex
	inventory        *Inventory
	spellbook        *Spellbook
	quarmy           *QuarmyData
	spellsets        *SpellsetFile
	invModTime       time.Time
	spellModTime     time.Time
	quarmyModTime    time.Time
	spellsetsModTime time.Time
}

// NewWatcher creates a Watcher. Call Start to begin polling.
func NewWatcher(cfgMgr *config.Manager, hub *ws.Hub, charStore *character.Store) *Watcher {
	return &Watcher{
		cfgMgr:    cfgMgr,
		hub:       hub,
		charStore: charStore,
	}
}

// SetQuarmyCallback registers a callback fired whenever the Quarmy export is
// successfully refreshed (i.e. inventory + AAs have new data). Replaces any
// previously-registered callback. Pass nil to clear.
func (w *Watcher) SetQuarmyCallback(fn func(charName string)) {
	w.mu.Lock()
	w.onQuarmyChanged = fn
	w.mu.Unlock()
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

// Quarmy returns the most recently parsed quarmy data, or nil if none.
func (w *Watcher) Quarmy() *QuarmyData {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.quarmy
}

// Spellsets returns the most recently parsed spellsets for the active character, or nil if none.
func (w *Watcher) Spellsets() *SpellsetFile {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.spellsets
}

// AllSpellsets scans the EQ directory for every character's spellsets export.
// Returns a non-configured response when the EQ path is empty.
func (w *Watcher) AllSpellsets() (*AllSpellsetsResponse, error) {
	cfg := w.cfgMgr.Get()
	resp := &AllSpellsetsResponse{
		Configured: cfg.EQPath != "",
		Characters: []*SpellsetFile{},
	}
	if cfg.EQPath == "" {
		return resp, nil
	}
	chars, err := ScanAllSpellsets(cfg.EQPath)
	if err != nil {
		return nil, err
	}
	if chars != nil {
		resp.Characters = chars
	}
	return resp, nil
}

// RefreshAllPersonas parses every stored character's Quarmy export (when
// present on disk) and persists their level/class/race/stats/AAs to user.db.
// The active-character watcher loop only persists data for whoever is logged
// in right now; without this sweep, non-active characters keep whatever level
// they had when first imported (often 1) and the Characters page misreads
// them. Errors per character are logged and skipped.
func (w *Watcher) RefreshAllPersonas() {
	if w.charStore == nil {
		return
	}
	cfg := w.cfgMgr.Get()
	if cfg.EQPath == "" {
		return
	}
	chars, err := w.charStore.List()
	if err != nil {
		slog.Warn("zeal: refresh personas: list characters", "err", err)
		return
	}
	for _, c := range chars {
		path := QuarmyPath(cfg.EQPath, c.Name)
		if ModTime(path).IsZero() {
			continue
		}
		data, err := ParseQuarmy(path, c.Name)
		if err != nil {
			slog.Warn("zeal: refresh personas: parse quarmy", "character", c.Name, "err", err)
			continue
		}
		s := data.Stats
		if err := w.charStore.UpdateStats(c.ID, s.BaseSTR, s.BaseSTA, s.BaseCHA, s.BaseDEX, s.BaseINT, s.BaseAGI, s.BaseWIS); err != nil {
			slog.Warn("zeal: refresh personas: save stats", "character", c.Name, "err", err)
		}
		if data.Level > 0 && data.Class > 0 && data.Race > 0 {
			if err := w.charStore.UpdatePersona(c.ID, data.Class-1, data.Race, data.Level); err != nil {
				slog.Warn("zeal: refresh personas: save persona", "character", c.Name, "err", err)
			}
		}
		aas := make([]character.AAEntry, len(data.AAs))
		for i, aa := range data.AAs {
			aas[i] = character.AAEntry{AAID: aa.ID, Rank: aa.Rank}
		}
		if err := w.charStore.ReplaceAAs(c.ID, aas); err != nil {
			slog.Warn("zeal: refresh personas: save aas", "character", c.Name, "err", err)
		}
	}
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
	quarmyPath := QuarmyPath(cfg.EQPath, character)
	spellsetsPath := SpellsetPath(cfg.EQPath, character)

	w.checkInventory(invPath, character)
	w.checkSpellbook(spellPath, character)
	w.checkQuarmy(quarmyPath, character)
	w.checkSpellsets(spellsetsPath, character)
}

func (w *Watcher) checkSpellsets(path, character string) {
	mt := ModTime(path)
	if mt.IsZero() {
		return
	}

	w.mu.RLock()
	unchanged := mt.Equal(w.spellsetsModTime)
	w.mu.RUnlock()

	if unchanged {
		return
	}

	sf, err := ParseSpellsets(path, character)
	if err != nil {
		slog.Warn("zeal: parse spellsets", "path", path, "err", err)
		return
	}

	w.mu.Lock()
	w.spellsets = sf
	w.spellsetsModTime = mt
	w.mu.Unlock()

	slog.Info("zeal: spellsets updated", "character", character, "sets", len(sf.Spellsets))
	w.hub.Broadcast(ws.Event{Type: "zeal:spellsets", Data: sf})
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

func (w *Watcher) checkQuarmy(path, charName string) {
	mt := ModTime(path)
	if mt.IsZero() {
		return
	}

	w.mu.RLock()
	unchanged := mt.Equal(w.quarmyModTime)
	w.mu.RUnlock()

	if unchanged {
		return
	}

	data, err := ParseQuarmy(path, charName)
	if err != nil {
		slog.Warn("zeal: parse quarmy", "path", path, "err", err)
		return
	}

	w.mu.Lock()
	w.quarmy = data
	w.quarmyModTime = mt
	w.mu.Unlock()

	slog.Info("zeal: quarmy updated", "character", charName, "aas", len(data.AAs))
	w.hub.Broadcast(ws.Event{Type: "zeal:quarmy", Data: data})

	w.mu.RLock()
	cb := w.onQuarmyChanged
	w.mu.RUnlock()
	if cb != nil {
		cb(charName)
	}

	// Persist stats and AAs to user.db if character store is available.
	if w.charStore == nil {
		return
	}
	char, found, err := w.charStore.GetByName(charName)
	if err != nil {
		slog.Warn("zeal: lookup character for quarmy import", "name", charName, "err", err)
		return
	}
	if !found {
		return
	}
	s := data.Stats
	if err := w.charStore.UpdateStats(char.ID, s.BaseSTR, s.BaseSTA, s.BaseCHA, s.BaseDEX, s.BaseINT, s.BaseAGI, s.BaseWIS); err != nil {
		slog.Warn("zeal: save character stats", "character", charName, "err", err)
	}
	// Quarmy stores Class as the EQ 1-indexed ID (1=WAR … 15=BST); the app
	// uses a 0-indexed scheme (0=WAR … 14=BST). Race uses the same 1-indexed
	// scheme on both sides. Level is a direct copy.
	if data.Level > 0 && data.Class > 0 && data.Race > 0 {
		if err := w.charStore.UpdatePersona(char.ID, data.Class-1, data.Race, data.Level); err != nil {
			slog.Warn("zeal: save character persona", "character", charName, "err", err)
		}
	}
	aas := make([]character.AAEntry, len(data.AAs))
	for i, aa := range data.AAs {
		aas[i] = character.AAEntry{AAID: aa.ID, Rank: aa.Rank}
	}
	if err := w.charStore.ReplaceAAs(char.ID, aas); err != nil {
		slog.Warn("zeal: save character aas", "character", charName, "err", err)
	}
}
