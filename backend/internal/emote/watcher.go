package emote

import (
	"context"
	"log/slog"
	"os"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

const pollInterval = 5 * time.Second

// Watcher polls spells_en.txt for changes so a server patch that republishes
// the file (wiping any hand-edited emotes) can be detected and, at the
// user's choice, have their overrides re-applied on top of it.
type Watcher struct {
	cfgMgr  *config.Manager
	hub     *ws.Hub
	service *Service

	lastModTime time.Time
}

// NewWatcher creates a Watcher. Call Start to begin polling.
func NewWatcher(cfgMgr *config.Manager, hub *ws.Hub, service *Service) *Watcher {
	return &Watcher{cfgMgr: cfgMgr, hub: hub, service: service}
}

// Start begins the polling loop. It blocks until ctx is cancelled.
// Run it in a goroutine: go watcher.Start(ctx).
func (w *Watcher) Start(ctx context.Context) {
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

func (w *Watcher) check() {
	cfg := w.cfgMgr.Get()
	if cfg.EQPath == "" {
		return
	}
	path, err := w.service.livePath()
	if err != nil {
		return
	}
	info, err := os.Stat(path)
	if err != nil {
		return
	}
	mt := info.ModTime()
	if mt.Equal(w.lastModTime) {
		return
	}
	w.lastModTime = mt

	b, err := os.ReadFile(path)
	if err != nil {
		slog.Warn("emote: read spells_en.txt", "err", err)
		return
	}
	content := string(b)

	lastWrite, ok, err := w.service.store.GetMeta(metaLastWriteHash)
	if err != nil {
		slog.Warn("emote: read last-write hash", "err", err)
		return
	}
	if ok && hashContent(content) == lastWrite {
		return // our own write; nothing external happened
	}

	// First-ever sighting of the file (no default backup yet): capture it as
	// the pristine default rather than treating it as an "external change."
	if _, hasDefault, err := readBackup(w.service.defaultBackupPath()); err == nil && !hasDefault {
		if err := w.service.captureAsDefault(content); err != nil {
			slog.Warn("emote: capture initial default backup", "err", err)
		}
		return
	}

	w.service.MarkExternalChange(content)
	slog.Info("emote: spells_en.txt changed externally")
	w.hub.Broadcast(ws.Event{Type: "emote:external-change", Data: map[string]any{
		"detected_at": time.Now().UTC().Unix(),
	}})
}
