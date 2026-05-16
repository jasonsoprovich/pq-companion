// Command server starts the PQ Companion HTTP API server.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/api"
	"github.com/jasonsoprovich/pq-companion/backend/internal/appbackup"
	"github.com/jasonsoprovich/pq-companion/backend/internal/backup"
	"github.com/jasonsoprovich/pq-companion/backend/internal/character"
	"github.com/jasonsoprovich/pq-companion/backend/internal/combat"
	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/overlay"
	"github.com/jasonsoprovich/pq-companion/backend/internal/players"
	"github.com/jasonsoprovich/pq-companion/backend/internal/rolltracker"
	"github.com/jasonsoprovich/pq-companion/backend/internal/spelltimer"
	"github.com/jasonsoprovich/pq-companion/backend/internal/trigger"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
	"github.com/jasonsoprovich/pq-companion/backend/internal/zeal"
	"github.com/jasonsoprovich/pq-companion/backend/internal/zealpipe"
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

	// Apply any pending app-state import BEFORE opening user.db connections.
	// Sentinel + staging files live under ~/.pq-companion. A pending import
	// is the result of the user choosing "Import" in the Backup Manager and
	// then restarting — the actual file swap happens here so it can run
	// without DB connections in flight.
	homeForImport, hErr := os.UserHomeDir()
	if hErr == nil {
		appHome := filepath.Join(homeForImport, ".pq-companion")
		userDBPath := filepath.Join(appHome, "user.db")
		appBackup := appbackup.New(userDBPath, exeBackupsDir(), appHome, runtimeAppVersion())
		applied, err := appBackup.ApplyPendingImport()
		if err != nil {
			slog.Error("apply pending app import", "err", err)
		} else if applied {
			slog.Info("applied pending app-state import; swapped user.db and backups dir")
		}
	}

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

	// Player tracker: persists every /who row to user.db so the user can
	// build a history of players they've encountered. Non-fatal — failing
	// here only disables the Players page; the rest of the app runs fine.
	playerStore, err := players.OpenStore(filepath.Join(home, ".pq-companion", "user.db"))
	var playersConsumer *players.Consumer
	if err != nil {
		slog.Warn("open player tracker (disabled)", "err", err)
	} else {
		defer playerStore.Close()
		playersConsumer = players.NewConsumer(playerStore)
	}

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

	// Forward declaration so the tailer pointer can be referenced inside the
	// closures passed to the combat tracker and timer engine. The tailer is
	// created below after both are wired up.
	var tailer *logparser.Tailer

	// Combat tracker: watches log events to group hits into fights, track
	// per-entity damage, and broadcast overlay:combat WebSocket events. The
	// player-name closure lets it relabel "You" rows with the active
	// character name so they merge with pet rows on the frontend rollup.
	combatTracker := combat.NewTracker(hub, func() string {
		if tailer != nil {
			if name := tailer.ActiveCharacter(); name != "" {
				return name
			}
		}
		return cfgMgr.Get().Character
	})

	// Combat history store: persists every archived fight to user.db so the
	// in-memory ring buffer is no longer the only record. Failure here is
	// non-fatal — the tracker still works in memory-only mode without it.
	historyStore, err := combat.OpenHistoryStore(filepath.Join(home, ".pq-companion", "user.db"))
	if err != nil {
		slog.Warn("open combat history (persistence disabled)", "err", err)
	} else {
		defer historyStore.Close()
		combatTracker.SetHistoryStore(historyStore)
		// Prune anything older than the configured retention window on
		// startup, then once per hour while the server runs. Running on a
		// goroutine so a slow / contended user.db can't delay startup.
		go pruneCombatHistory(context.Background(), historyStore, cfgMgr)
	}

	// Spell timer engine: watches cast/resist/fade events, maintains countdown
	// timers per active spell, and broadcasts overlay:timers WebSocket events.
	// The CharacterContext closure feeds buffmod the active char + EQ path so
	// AA/item duration focuses extend the displayed timer.
	timerEngine := spelltimer.NewEngine(hub, database, func() (string, string, int) {
		cfg := cfgMgr.Get()
		var charName string
		if tailer != nil {
			charName = tailer.ActiveCharacter()
		} else {
			charName = cfg.Character
		}
		class := -1
		if charName != "" {
			if c, ok, err := charStore.GetByName(charName); err == nil && ok {
				class = c.Class
			}
		}
		return cfg.EQPath, charName, class
	}, func() string {
		return cfgMgr.Get().SpellTimer.TrackingScope
	}, func() (bool, int) {
		// Class filter resolves the active character's class index from the
		// character store on every cast; an unset / unknown class returns -1
		// so the engine treats the filter as inactive even when enabled.
		cfg := cfgMgr.Get()
		if !cfg.SpellTimer.ClassFilter {
			return false, -1
		}
		var charName string
		if tailer != nil {
			charName = tailer.ActiveCharacter()
		} else {
			charName = cfg.Character
		}
		if charName == "" {
			return true, -1
		}
		c, ok, err := charStore.GetByName(charName)
		if err != nil || !ok {
			return true, -1
		}
		return true, c.Class
	}, func() string {
		return cfgMgr.Get().SpellTimer.TrackingMode
	})
	go timerEngine.Start(context.Background())

	// Zeal IPC supervisor: discovers the eqgame.exe Zeal pipe and forwards
	// live state into every downstream consumer that benefits from real-time,
	// authoritative game data instead of log inference.
	//   Stage A: target name -> npcTracker
	//   Stage B: target HP%, pet owner -> npcTracker
	//   Stage C: target + player pet name -> combatTracker
	//   Stage D: casting name + buff slots -> timerEngine (observability)
	pipeSupervisor := zealpipe.NewSupervisor(func(env zealpipe.Envelope) {
		if env.Type != zealpipe.MsgLabel {
			return
		}
		labels, err := zealpipe.DecodeLabels(env.Data)
		if err != nil {
			return
		}
		// Aggregate per-envelope state. Zeal omits labels with empty values
		// (Zeal/named_pipe.cpp:280), so absence of a label means the slot is
		// empty — we still need to push that as a snapshot so the engine
		// learns the prior value is gone.
		var castingName string
		var buffSlots []string
		for _, l := range labels {
			switch l.Type {
			case zealpipe.LabelTargetName:
				if l.Value == "" {
					npcTracker.ClearPipeTarget()
				} else {
					npcTracker.SetPipeTarget(l.Value)
				}
				combatTracker.SetPipeTarget(l.Value)
			case zealpipe.LabelTargetHPPerc:
				if n, err := strconv.Atoi(strings.TrimSpace(l.Value)); err == nil {
					npcTracker.SetPipeHPPercent(n)
				}
			case zealpipe.LabelTargetPetOwner:
				npcTracker.SetPipePetOwner(l.Value)
			case zealpipe.LabelPlayerPetName:
				combatTracker.SetPipePetName(l.Value)
			case zealpipe.LabelCastingName:
				castingName = l.Value
			default:
				// Buff slots: 45-59 (Buff0-14) and 135-140 (Buff15-20).
				id := int(l.Type)
				if (id >= int(zealpipe.LabelBuff0) && id <= int(zealpipe.LabelBuff14)) ||
					(id >= int(zealpipe.LabelBuff15) && id <= int(zealpipe.LabelBuff20)) {
					buffSlots = append(buffSlots, l.Value)
				}
			}
		}
		timerEngine.SetPipeCasting(castingName)
		timerEngine.SetPipeBuffSlots(buffSlots)
	})
	pipeSupervisor.OnDisconnect(func() {
		// Drop pipe-only state from all consumers so stale values don't
		// linger after Zeal goes away. Log-driven paths continue to work via
		// the normal Handle() flow.
		npcTracker.ResetPipeFields()
		combatTracker.ResetPipeState()
		timerEngine.SetPipeCasting("")
		timerEngine.SetPipeBuffSlots(nil)
	})
	go pipeSupervisor.Start(context.Background())

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

	// One-time additive default updates for built-in packs. Each is keyed
	// and runs at most once, only ever appending to list-typed fields on
	// installed pack triggers — preserves user customizations while still
	// letting hotfixes (e.g. new exclude patterns for "Incoming Tell")
	// reach existing installs without a destructive reinstall.
	if mutated, err := trigger.ApplyDefaultUpdates(triggerStore, trigger.DefaultUpdates()); err != nil {
		slog.Warn("trigger default updates failed", "err", err)
	} else if mutated > 0 {
		slog.Info("trigger default updates applied", "mutated_triggers", mutated)
	}

	triggerEngine := trigger.NewEngine(triggerStore, hub, timerEngine, func() string {
		if tailer != nil {
			return tailer.ActiveCharacter()
		}
		return cfgMgr.Get().Character
	})
	triggerEngine.Reload()

	// Roll tracker: groups /random results into per-range sessions and
	// broadcasts overlay:rolls WebSocket events. Stateless across restarts.
	rollTracker := rolltracker.New(hub)

	// Log tailer: reads new lines from the EQ log file and broadcasts parsed
	// events to all connected WebSocket clients. Also feeds overlay trackers
	// and the trigger engine.
	tailer = logparser.NewTailer(cfgMgr, func(ev logparser.LogEvent) {
		hub.Broadcast(ws.Event{Type: string(ev.Type), Data: ev})
		npcTracker.Handle(ev)
		combatTracker.Handle(ev)
		timerEngine.Handle(ev)
		rollTracker.Handle(ev)
		if playersConsumer != nil {
			playersConsumer.Handle(ev)
		}
	}, triggerEngine.Handle, func(character string) {
		slog.Info("logparser: auto-detected active character", "character", character)
		hub.Broadcast(ws.Event{Type: "config:character_detected", Data: map[string]string{"character": character}})
		// Active character changed — drop cached buffmod contributors so the
		// next cast recomputes against the new character's inventory + AAs.
		timerEngine.RefreshModifiers()
	})
	go tailer.Start(context.Background())

	// Always bind to 127.0.0.1 explicitly — the API is consumed by the
	// local renderer only, so there's no reason to listen on every
	// interface, and a single-stack loopback bind is the only reliable
	// way to detect a port conflict with another local app across
	// macOS / Linux / Windows (Go's default :port = dual-stack IPv6 with
	// v6only=false, which can silently coexist with an IPv4-only listener
	// on the same port).
	_, portStr, splitErr := net.SplitHostPort(listenAddr)
	if splitErr != nil {
		portStr = strings.TrimPrefix(listenAddr, ":")
	}
	bindAddr := "127.0.0.1:" + portStr
	// If the preferred port is busy (e.g. Calibre on 8080), fall back to
	// an OS-assigned free port on loopback so the app still launches.
	// The chosen port is written to stdout as a single `BACKEND_PORT=N`
	// line so the Electron main process can read it back and tell the
	// renderer where to send API requests.
	listener, err := net.Listen("tcp", bindAddr)
	if err != nil {
		slog.Warn("preferred port unavailable, falling back to auto-assigned localhost port",
			"preferred", bindAddr, "err", err)
		listener, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			slog.Error("could not bind any TCP port", "err", err)
			os.Exit(1)
		}
	}
	actualPort := listener.Addr().(*net.TCPAddr).Port
	fmt.Fprintf(os.Stdout, "BACKEND_PORT=%d\n", actualPort)

	// Also write the port to ~/.pq-companion/server-port so consumers that
	// aren't this process's parent — chiefly `npm run dev`, where Electron
	// did not spawn the backend and has no stdout to parse — can discover
	// the actual bound port. Best-effort: a failure here doesn't impact
	// the production sidecar flow, which uses BACKEND_PORT=N over stdout.
	if home, err := os.UserHomeDir(); err == nil {
		portFile := filepath.Join(home, ".pq-companion", "server-port")
		if err := os.WriteFile(portFile, []byte(strconv.Itoa(actualPort)), 0o644); err != nil {
			slog.Warn("could not write port discovery file", "path", portFile, "err", err)
		}
	}

	// Live app-backup manager for export / import endpoints.
	appBackupMgr := appbackup.New(
		filepath.Join(home, ".pq-companion", "user.db"),
		exeBackupsDir(),
		filepath.Join(home, ".pq-companion"),
		runtimeAppVersion(),
	)

	router := api.NewRouter(database, hub, cfgMgr, zealWatcher, pipeSupervisor, backupMgr, tailer, npcTracker, combatTracker, historyStore, timerEngine, triggerStore, triggerEngine, charStore, rollTracker, appBackupMgr, playerStore, actualPort)

	slog.Info("server starting", "addr", listener.Addr().String(), "db", *dbPath)
	if err := http.Serve(listener, router); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}

// pruneCombatHistory runs an immediate retention sweep, then loops once an
// hour to keep the combat_fights table within the configured window. The
// retention setting is read on each tick so config changes take effect
// without restarting. Logs failures via slog but never aborts — a flapping
// disk shouldn't take down the server.
func pruneCombatHistory(ctx context.Context, store *combat.HistoryStore, cfgMgr *config.Manager) {
	prune := func() {
		days := cfgMgr.Get().Combat.RetentionDays
		if days <= 0 {
			return
		}
		cutoff := time.Now().AddDate(0, 0, -days)
		removed, err := store.PruneOlderThan(cutoff)
		if err != nil {
			slog.Warn("prune combat history", "err", err)
			return
		}
		if removed > 0 {
			slog.Info("pruned combat history", "removed", removed, "older_than_days", days)
		}
	}
	prune()
	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			prune()
		}
	}
}

// exeBackupsDir matches backup.exeBackupDir's logic — the EQ-config backups
// dir is under the running executable. Kept locally rather than exported
// from the backup package to avoid creating a public surface for a single
// caller.
func exeBackupsDir() string {
	exe, err := os.Executable()
	if err == nil {
		return filepath.Join(filepath.Dir(exe), "backups")
	}
	return "backups"
}

// runtimeAppVersion returns the app version Electron passed via the
// PQ_APP_VERSION env var when spawning the sidecar. Falls back to "dev"
// when running standalone (typical during `go run ./cmd/server`).
func runtimeAppVersion() string {
	if v := os.Getenv("PQ_APP_VERSION"); v != "" {
		return v
	}
	return "dev"
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
