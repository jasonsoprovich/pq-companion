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
	"github.com/jasonsoprovich/pq-companion/backend/internal/character"
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

	// zealWatcher is initialized after charStore is opened; see below.


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

	charStore, err := character.OpenStore(filepath.Join(home, ".pq-companion", "user.db"))
	if err != nil {
		slog.Error("open character store", "err", err)
		os.Exit(1)
	}
	defer charStore.Close()

	// Build the spell-landed detection index from the read-only spells_new
	// table. Failure here is non-fatal — without the index, ParseLine simply
	// won't emit EventSpellLanded events and the engine falls back to the
	// (less accurate) cast-begin path.
	if msgs, err := database.LoadCastMessages(); err != nil {
		slog.Warn("load cast messages (spell-landed detection disabled)", "err", err)
	} else {
		idxMsgs := make([]logparser.CastMessage, 0, len(msgs))
		for _, m := range msgs {
			idxMsgs = append(idxMsgs, logparser.CastMessage{
				SpellID:     m.SpellID,
				SpellName:   m.SpellName,
				CastOnYou:   m.CastOnYou,
				CastOnOther: m.CastOnOther,
			})
		}
		logparser.SetCastIndex(logparser.NewCastIndex(idxMsgs))
		slog.Info("spell-landed index loaded", "entries", len(idxMsgs))
	}

	zealWatcher := zeal.NewWatcher(cfgMgr, hub, charStore)
	// Sync every stored character's persona/stats/AAs from their Quarmy export
	// at startup. The polling loop only refreshes the active character, so
	// without this initial sweep the Characters page reads stale levels for
	// anyone not currently logged in.
	go zealWatcher.RefreshAllPersonas()
	go zealWatcher.Start(context.Background())

	// NPC overlay tracker: watches log events to infer the current combat target
	// and broadcasts overlay:npc_target WebSocket events with full NPC data.
	npcTracker := overlay.NewNPCTracker(hub, database)

	// Combat tracker: watches log events to group hits into fights, track
	// per-entity damage, and broadcast overlay:combat WebSocket events.
	combatTracker := combat.NewTracker(hub)

	// Forward declaration so the tailer pointer can be referenced inside the
	// CharacterContext closure passed to the timer engine. The tailer is
	// created below after the engine is wired up.
	var tailer *logparser.Tailer

	// Spell timer engine: watches cast/resist/fade events, maintains countdown
	// timers per active spell, and broadcasts overlay:timers WebSocket events.
	// The CharacterContext closure feeds buffmod the active char + EQ path so
	// AA/item duration focuses extend the displayed timer.
	timerEngine := spelltimer.NewEngine(hub, database, func() (string, string) {
		cfg := cfgMgr.Get()
		var charName string
		if tailer != nil {
			charName = tailer.ActiveCharacter()
		} else {
			charName = cfg.Character
		}
		return cfg.EQPath, charName
	}, func() string {
		return cfgMgr.Get().SpellTimer.TrackingScope
	})
	go timerEngine.Start(context.Background())

	// Invalidate the engine's modifier cache whenever the Quarmy export is
	// refreshed (new inventory or AAs). Without this, equipping/unequipping a
	// focus item wouldn't take effect until the app restarts.
	zealWatcher.SetQuarmyCallback(func(_ string) {
		timerEngine.RefreshModifiers()
	})

	// One-time backfill: existing triggers (created before per-character
	// support) get every known character checked, so the user can prune from
	// there. Skipped on fresh installs with no characters yet.
	if chars, err := charStore.List(); err == nil {
		names := make([]string, 0, len(chars))
		for _, c := range chars {
			if c.Name != "" {
				names = append(names, c.Name)
			}
		}
		if err := triggerStore.BackfillCharactersIfNeeded(names); err != nil {
			slog.Warn("trigger character backfill failed", "err", err)
		}
	}

	triggerEngine := trigger.NewEngine(triggerStore, hub, timerEngine, func() string {
		if tailer != nil {
			return tailer.ActiveCharacter()
		}
		return cfgMgr.Get().Character
	})
	triggerEngine.Reload()

	// Log tailer: reads new lines from the EQ log file and broadcasts parsed
	// events to all connected WebSocket clients. Also feeds overlay trackers
	// and the trigger engine.
	tailer = logparser.NewTailer(cfgMgr, func(ev logparser.LogEvent) {
		hub.Broadcast(ws.Event{Type: string(ev.Type), Data: ev})
		npcTracker.Handle(ev)
		combatTracker.Handle(ev)
		timerEngine.Handle(ev)
	}, triggerEngine.Handle, func(character string) {
		slog.Info("logparser: auto-detected active character", "character", character)
		hub.Broadcast(ws.Event{Type: "config:character_detected", Data: map[string]string{"character": character}})
		// Active character changed — drop cached buffmod contributors so the
		// next cast recomputes against the new character's inventory + AAs.
		timerEngine.RefreshModifiers()
	})
	go tailer.Start(context.Background())

	router := api.NewRouter(database, hub, cfgMgr, zealWatcher, backupMgr, tailer, npcTracker, combatTracker, timerEngine, triggerStore, triggerEngine, charStore)

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
