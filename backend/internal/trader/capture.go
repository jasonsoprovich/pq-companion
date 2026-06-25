package trader

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
	"github.com/jasonsoprovich/pq-companion/backend/internal/zeal"
)

// capturePollInterval is how often the capturer scans for changed trader
// exports. Trader snapshots only change on a manual /output inventory (or a
// camp), so this can be lazier than the 5s Zeal watcher.
const capturePollInterval = 10 * time.Second

// Capturer auto-records trader snapshots. It polls the EQ directory for every
// character that has a BZR price file and, when that character's inventory
// export changes in a way that affects satchel contents or coin, appends a new
// snapshot. It is independent of the Zeal watcher (which only tracks the active
// character) so a parked trader is captured no matter who is logged in.
type Capturer struct {
	cfgMgr *config.Manager
	store  *Store
	hub    *ws.Hub

	mu      sync.Mutex
	lastMod map[string]time.Time // character -> mod time of last-seen export
}

// NewCapturer creates a Capturer. Call Start to begin polling.
func NewCapturer(cfgMgr *config.Manager, store *Store, hub *ws.Hub) *Capturer {
	return &Capturer{
		cfgMgr:  cfgMgr,
		store:   store,
		hub:     hub,
		lastMod: make(map[string]time.Time),
	}
}

// Start runs the polling loop until ctx is cancelled. Run in a goroutine.
func (c *Capturer) Start(ctx context.Context) {
	if c == nil || c.store == nil {
		return
	}
	c.scan()
	ticker := time.NewTicker(capturePollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.scan()
		}
	}
}

// scan captures a snapshot for every BZR character whose export file changed.
func (c *Capturer) scan() {
	eqPath := c.cfgMgr.Get().EQPath
	if eqPath == "" {
		return
	}
	chars, err := FindBZRCharacters(eqPath)
	if err != nil {
		slog.Warn("trader: scan BZR files", "err", err)
		return
	}
	for _, char := range chars {
		path := FindExportFile(eqPath, char)
		if path == "" {
			continue
		}
		mt := zeal.ModTime(path)
		c.mu.Lock()
		unchanged := mt.Equal(c.lastMod[char])
		c.mu.Unlock()
		if unchanged {
			continue
		}
		if _, stored, err := c.capture(eqPath, char, path); err != nil {
			slog.Warn("trader: capture snapshot", "character", char, "err", err)
		} else if stored {
			slog.Info("trader: snapshot captured", "character", char)
		}
		c.mu.Lock()
		c.lastMod[char] = mt
		c.mu.Unlock()
	}
}

// Capture forces a snapshot capture for one character from its newest export,
// storing it only if it differs from the last stored snapshot. It returns the
// parsed snapshot, whether a new row was stored, and any error. Used by the
// manual "Capture now" endpoint.
func (c *Capturer) Capture(character string) (*Snapshot, bool, error) {
	eqPath := c.cfgMgr.Get().EQPath
	if eqPath == "" {
		return nil, false, nil
	}
	path := FindExportFile(eqPath, character)
	if path == "" {
		return nil, false, nil
	}
	snap, stored, err := c.capture(eqPath, character, path)
	if err == nil {
		c.mu.Lock()
		c.lastMod[character] = zeal.ModTime(path)
		c.mu.Unlock()
	}
	return snap, stored, err
}

// capture parses the export at path and appends it as a snapshot if its
// fingerprint differs from the character's latest stored snapshot.
func (c *Capturer) capture(eqPath, character, path string) (*Snapshot, bool, error) {
	snap, err := ParseSnapshot(path, character)
	if err != nil {
		return nil, false, err
	}
	latest, ok, err := c.store.LatestSnapshot(character)
	if err != nil {
		return nil, false, err
	}
	if ok && latest.Fingerprint() == snap.Fingerprint() {
		return snap, false, nil // no change since last capture
	}
	if _, err := c.store.AppendSnapshot(snap); err != nil {
		return nil, false, err
	}
	if c.hub != nil {
		c.hub.Broadcast(ws.Event{Type: "trader:snapshot", Data: snap})
	}
	return snap, true, nil
}
