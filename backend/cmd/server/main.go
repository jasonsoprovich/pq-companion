// Command server starts the PQ Companion HTTP API server.
package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/jasonsoprovich/pq-companion/backend/internal/api"
	"github.com/jasonsoprovich/pq-companion/backend/internal/backup"
	"github.com/jasonsoprovich/pq-companion/backend/internal/combat"
	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/overlay"
	"github.com/jasonsoprovich/pq-companion/backend/internal/spelltimer"
	"github.com/jasonsoprovich/pq-companion/backend/internal/trigger"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
	"github.com/jasonsoprovich/pq-companion/backend/internal/zeal"
)

func main() {
	addr := flag.String("addr", "", "HTTP listen address (overrides config server_addr)")
	dbPath := flag.String("db", defaultDBPath(), "path to quarm.db")
	flag.Parse()

	cfgMgr, err := config.Load()
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}
	slog.Info("config loaded", "path", cfgMgr.Path())

	// CLI flag overrides config file address when explicitly provided.
	listenAddr := cfgMgr.Get().ServerAddr
	if *addr != "" {
		listenAddr = *addr
	}

	database, err := db.Open(*dbPath)
	if err != nil {
		slog.Error("open database", "err", err)
		os.Exit(1)
	}
	defer database.Close()

	hub := ws.NewHub()
	go hub.Run()

	zealWatcher := zeal.NewWatcher(cfgMgr, hub)
	go zealWatcher.Start(context.Background())

	backupMgr, err := backup.NewManager(cfgMgr)
	if err != nil {
		slog.Error("open backup manager", "err", err)
		os.Exit(1)
	}
	defer backupMgr.Close()
	go backupMgr.StartWatcher(context.Background())
	go backupMgr.StartScheduler(context.Background())

	// Trigger store: uses the same user.db as the backup manager but opens its
	// own connection (WAL mode handles concurrent access safely).
	home, err := os.UserHomeDir()
	if err != nil {
		slog.Error("get home dir", "err", err)
		os.Exit(1)
	}
	triggerStore, err := trigger.OpenStore(filepath.Join(home, ".pq-companion", "user.db"))
	if err != nil {
		slog.Error("open trigger store", "err", err)
		os.Exit(1)
	}
	defer triggerStore.Close()

	triggerEngine := trigger.NewEngine(triggerStore, hub)
	triggerEngine.Reload()

	// NPC overlay tracker: watches log events to infer the current combat target
	// and broadcasts overlay:npc_target WebSocket events with full NPC data.
	npcTracker := overlay.NewNPCTracker(hub, database)

	// Combat tracker: watches log events to group hits into fights, track
	// per-entity damage, and broadcast overlay:combat WebSocket events.
	combatTracker := combat.NewTracker(hub)

	// Spell timer engine: watches cast/resist/fade events, maintains countdown
	// timers per active spell, and broadcasts overlay:timers WebSocket events.
	timerEngine := spelltimer.NewEngine(hub, database)
	go timerEngine.Start(context.Background())

	// Log tailer: reads new lines from the EQ log file and broadcasts parsed
	// events to all connected WebSocket clients. Also feeds overlay trackers
	// and the trigger engine.
	tailer := logparser.NewTailer(cfgMgr, func(ev logparser.LogEvent) {
		hub.Broadcast(ws.Event{Type: string(ev.Type), Data: ev})
		npcTracker.Handle(ev)
		combatTracker.Handle(ev)
		timerEngine.Handle(ev)
	}, triggerEngine.Handle)
	go tailer.Start(context.Background())

	router := api.NewRouter(database, hub, cfgMgr, zealWatcher, backupMgr, tailer, npcTracker, combatTracker, timerEngine, triggerStore, triggerEngine)

	slog.Info("server starting", "addr", listenAddr, "db", *dbPath)
	if err := http.ListenAndServe(listenAddr, router); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}

// defaultDBPath returns the path to quarm.db relative to the executable's
// directory, falling back to the repo-relative development path.
func defaultDBPath() string {
	exe, err := os.Executable()
	if err == nil {
		candidate := filepath.Join(filepath.Dir(exe), "data", "quarm.db")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	// Development fallback: run from backend/ directory.
	return filepath.Join("data", "quarm.db")
}
