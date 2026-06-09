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
	"github.com/jasonsoprovich/pq-companion/backend/internal/applog"
	"github.com/jasonsoprovich/pq-companion/backend/internal/backfill"
	"github.com/jasonsoprovich/pq-companion/backend/internal/backup"
	"github.com/jasonsoprovich/pq-companion/backend/internal/character"
	"github.com/jasonsoprovich/pq-companion/backend/internal/chat"
	"github.com/jasonsoprovich/pq-companion/backend/internal/chchain"
	"github.com/jasonsoprovich/pq-companion/backend/internal/combat"
	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/keyring"
	"github.com/jasonsoprovich/pq-companion/backend/internal/lockout"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/loot"
	"github.com/jasonsoprovich/pq-companion/backend/internal/overlay"
	"github.com/jasonsoprovich/pq-companion/backend/internal/players"
	"github.com/jasonsoprovich/pq-companion/backend/internal/respawn"
	"github.com/jasonsoprovich/pq-companion/backend/internal/rolltracker"
	"github.com/jasonsoprovich/pq-companion/backend/internal/sandbox"
	"github.com/jasonsoprovich/pq-companion/backend/internal/savedquery"
	"github.com/jasonsoprovich/pq-companion/backend/internal/skills"
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

	logPath, logCloser, logErr := applog.Init(runtimeAppVersion())
	if logCloser != nil {
		defer logCloser.Close()
	}
	if logErr != nil {
		slog.Warn("file logging unavailable; stderr only", "err", logErr)
	} else {
		slog.Info("file logging enabled", "path", logPath)
	}

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
		backupsDir := filepath.Join(appHome, "backups")
		// Move any backups from the legacy <exe_dir>/backups location before
		// the import swap runs, so a pending import sees the up-to-date set.
		backup.MigrateLegacyDir(backupsDir)
		appBackup := appbackup.New(userDBPath, backupsDir, appHome, runtimeAppVersion())
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

	if st, statErr := os.Stat(*dbPath); statErr != nil {
		slog.Error("quarm.db stat failed",
			"path", *dbPath,
			"err", statErr,
		)
	} else {
		slog.Info("quarm.db located",
			"path", *dbPath,
			"size_bytes", st.Size(),
			"mode", st.Mode().String(),
			"mod_time", st.ModTime().Format(time.RFC3339),
		)
	}

	database, err := db.Open(*dbPath)
	if err != nil {
		slog.Error("open database",
			"err", err,
			"path", *dbPath,
			"hint", "If error is 'unable to open database file' the file exists "+
				"but the process cannot open it. Common causes: AV ACL on the "+
				"file, OneDrive Files-on-Demand placeholder, stale -journal/-wal "+
				"sibling, or install on a read-only / unusual filesystem.",
		)
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

	// Chat history: persists player chat across tells + guild/raid/group/ooc/
	// auction/shout and named channels so the user can browse history. Non-fatal
	// — failing here only disables the Chat History page.
	chatStore, err := chat.OpenStore(filepath.Join(home, ".pq-companion", "user.db"))
	var chatConsumer *chat.Consumer
	if err != nil {
		slog.Warn("open chat history (disabled)", "err", err)
		chatStore = nil
	} else {
		defer chatStore.Close()
	}

	// Loot tracker: persists "has looted" lines into a searchable feed.
	// Non-fatal — failing here only disables the Loot Tracker page.
	lootStore, err := loot.OpenStore(filepath.Join(home, ".pq-companion", "user.db"))
	var lootConsumer *loot.Consumer
	if err != nil {
		slog.Warn("open loot tracker (disabled)", "err", err)
		lootStore = nil
	} else {
		defer lootStore.Close()
	}

	// Skill tracker: persists each character's skill values from "You have
	// become better at X! (N)" lines. Non-fatal — failing here only disables
	// the character Skills tab.
	skillsStore, err := skills.OpenStore(filepath.Join(home, ".pq-companion", "user.db"))
	var skillsConsumer *skills.Consumer
	if err != nil {
		slog.Warn("open skill tracker (disabled)", "err", err)
		skillsStore = nil
	} else {
		defer skillsStore.Close()
	}

	// Keyring tracker: persists per-character /keys snapshots. Master list
	// is loaded from quarm.db keyring_data once at startup (read-only data).
	// Non-fatal — failing here only disables the Keyring section in the Key
	// Tracker page.
	keyringStore, err := keyring.OpenStore(filepath.Join(home, ".pq-companion", "user.db"))
	var (
		keyringConsumer *keyring.Consumer
		keyringMaster   []keyring.MasterEntry
	)
	if err != nil {
		slog.Warn("open keyring tracker (disabled)", "err", err)
	} else {
		defer keyringStore.Close()
		keyringMaster, err = keyring.LoadMaster(database.DB)
		if err != nil {
			slog.Warn("load keyring master list (tracker disabled)", "err", err)
			keyringStore.Close()
			keyringStore = nil
		} else {
			slog.Info("keyring master list loaded", "count", len(keyringMaster))
		}
	}

	// Lockout tracker: persists per-character /sll loot & legacy-item lockout
	// snapshots. Non-fatal — failing here only disables the Lockouts page.
	lockoutStore, err := lockout.OpenStore(filepath.Join(home, ".pq-companion", "user.db"))
	var lockoutConsumer *lockout.Consumer
	if err != nil {
		slog.Warn("open lockout tracker (disabled)", "err", err)
		lockoutStore = nil
	} else {
		defer lockoutStore.Close()
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

	// Class resolvers stamp the canonical base class onto every EntityStats
	// row so the DPS meter can render each combatant's bar in the user's
	// per-class colour. selfClassFn covers the active character; the
	// resolver falls back to the /who tracker for other players.
	combatTracker.SetClassResolvers(
		func() string {
			cfg := cfgMgr.Get()
			var charName string
			if tailer != nil {
				charName = tailer.ActiveCharacter()
			}
			if charName == "" {
				charName = cfg.Character
			}
			if charName == "" {
				return ""
			}
			if c, ok, err := charStore.GetByName(charName); err == nil && ok {
				return players.ClassNameByIndex(c.Class)
			}
			return ""
		},
		func(name string) string {
			if playerStore == nil || name == "" {
				return ""
			}
			s, err := playerStore.Get(name)
			if err != nil || s == nil {
				return ""
			}
			return players.BaseClassOf(s.Class)
		},
	)

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

	// CH-chain matcher: watches raid chat for chain-call lines and creates
	// ch_chain countdown timers in the engine. Reads its regex/cadence/enabled
	// state live from config so Settings changes take effect without a restart.
	chChainMatcher := chchain.New(timerEngine, func() config.CHChainSettings {
		return cfgMgr.Get().CHChain
	})

	// Respawn (death) timer engine: starts a countdown when a mob is killed,
	// using the spawn data's respawn time for the player's current zone.
	respawnEngine := respawn.NewEngine(hub, database)
	go respawnEngine.Start(context.Background())

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

	// One-time rename of the legacy "Group Awareness" pack to "General
	// Triggers", plus insertion of Spell Resist / Spell Interrupt triggers
	// for users who already had the pack installed. Must run before
	// ApplyDefaultUpdates so the renamed pack name matches the DefaultUpdate
	// entry that targets "General Triggers".
	if err := triggerStore.MigrateGroupAwarenessToGeneralTriggers(); err != nil {
		slog.Warn("trigger general-triggers migration failed", "err", err)
	}
	if err := triggerStore.MigrateMezBrokeTTSPronunciation(); err != nil {
		slog.Warn("trigger mez-broke tts migration failed", "err", err)
	}
	if err := triggerStore.MigrateRemoveDuplicateClassPackTriggers(); err != nil {
		slog.Warn("trigger class-pack dupe removal failed", "err", err)
	}
	// In-place fix for packs imported before the debuff-pattern broadening and
	// the Bard 54s→18s song-duration correction. Only rewrites the pattern /
	// duration columns of un-customized rows; never touches user actions/alerts.
	if err := triggerStore.MigrateBroadenDebuffPatternsAndBardDurations(); err != nil {
		slog.Warn("trigger debuff-pattern/bard-duration migration failed", "err", err)
	}

	// One-time additive default updates for built-in packs. Each is keyed
	// and runs at most once, making only additive edits to installed pack
	// triggers (appending exclude patterns, filling in a missing cooldown)
	// — preserves user customizations while still letting hotfixes (e.g.
	// new "Incoming Tell" excludes, the monk Feign Death reuse timer) reach
	// existing installs without a destructive reinstall.
	if mutated, err := trigger.ApplyDefaultUpdates(triggerStore, trigger.DefaultUpdates()); err != nil {
		slog.Warn("trigger default updates failed", "err", err)
	} else if mutated > 0 {
		slog.Info("trigger default updates applied", "mutated_triggers", mutated)
	}

	activeChar := func() string {
		if tailer != nil {
			return tailer.ActiveCharacter()
		}
		return cfgMgr.Get().Character
	}
	triggerEngine := trigger.NewEngine(triggerStore, hub, timerEngine, activeChar)
	triggerEngine.Reload()

	if keyringStore != nil {
		keyringConsumer = keyring.NewConsumer(keyringStore, keyringMaster, activeChar)
		keyringConsumer.SetOnSnapshot(func(character string) {
			hub.Broadcast(ws.Event{
				Type: "keyring.snapshot",
				Data: map[string]any{"character": character},
			})
		})
	}

	if lockoutStore != nil {
		lockoutConsumer = lockout.NewConsumer(lockoutStore, activeChar)
		lockoutConsumer.SetOnSnapshot(func(character string) {
			hub.Broadcast(ws.Event{
				Type: "lockouts.snapshot",
				Data: map[string]any{"character": character},
			})
		})
	}

	if chatStore != nil {
		chatConsumer = chat.NewConsumer(chatStore, activeChar)
		chatConsumer.SetOnInsert(func(m chat.Message) {
			hub.Broadcast(ws.Event{Type: "chat:new", Data: m})
		})
	}

	if lootStore != nil {
		lootConsumer = loot.NewConsumer(lootStore, activeChar)
		lootConsumer.SetOnInsert(func(e loot.Entry) {
			hub.Broadcast(ws.Event{Type: "loot:new", Data: e})
		})
	}

	if skillsStore != nil {
		skillsConsumer = skills.NewConsumer(skillsStore, activeChar)
		skillsConsumer.SetOnUpdate(func(u skills.Update) {
			hub.Broadcast(ws.Event{Type: "skills:update", Data: u})
		})
	}

	// Backfill registry: powers Settings → Log Backfill. Each tracker that can
	// be retroactively populated from a character's log registers a dedup-safe,
	// timestamp-aware handler here. Upcoming trackers (loot, tradeskills) plug
	// in the same way.
	backfillRegistry := backfill.NewRegistry()
	if chatStore != nil {
		backfillRegistry.Register(backfill.Section{
			Key:        "chat",
			Label:      "Chat History",
			NewHandler: func(character string) backfill.Handler { return chat.NewBackfillHandler(chatStore, character) },
		})
	}
	if playerStore != nil {
		backfillRegistry.Register(backfill.Section{
			Key:        "players",
			Label:      "Player Tracker",
			NewHandler: func(string) backfill.Handler { return players.NewBackfillConsumer(playerStore) },
		})
	}
	if lootStore != nil {
		backfillRegistry.Register(backfill.Section{
			Key:        "loot",
			Label:      "Loot Tracker",
			NewHandler: func(character string) backfill.Handler { return loot.NewBackfillHandler(lootStore, character) },
		})
	}
	if skillsStore != nil {
		backfillRegistry.Register(backfill.Section{
			Key:        "skills",
			Label:      "Skill Tracker",
			NewHandler: func(character string) backfill.Handler { return skills.NewBackfillHandler(skillsStore, character) },
		})
	}

	// Chat history retention: purge messages older than the configured window
	// (default 30 days; 0 = keep forever) on startup and once a day, keeping the
	// single table lean so queries stay fast.
	if chatStore != nil {
		go func() {
			purge := func() {
				days := cfgMgr.Get().ChatRetentionDays
				if days <= 0 {
					return // keep forever
				}
				cutoff := time.Now().AddDate(0, 0, -days)
				if n, err := chatStore.Purge(cutoff); err != nil {
					slog.Warn("chat retention purge failed", "err", err)
				} else if n > 0 {
					slog.Info("chat retention purge", "deleted", n, "older_than_days", days)
				}
			}
			purge()
			ticker := time.NewTicker(24 * time.Hour)
			defer ticker.Stop()
			for range ticker.C {
				purge()
			}
		}()
	}

	// Zeal IPC supervisor: discovers the eqgame.exe Zeal pipe and forwards
	// live state into every downstream consumer that benefits from real-time,
	// authoritative game data instead of log inference.
	//   Stage A: target name -> npcTracker
	//   Stage B: target HP%, pet owner -> npcTracker
	//   Stage C: target + player pet name -> combatTracker
	//   Stage D: casting name + buff slots -> timerEngine (observability)
	//   Stage E: target + buff transitions + /pipe -> triggerEngine
	pipeSupervisor := zealpipe.NewSupervisor(func(env zealpipe.Envelope) {
		switch env.Type {
		case zealpipe.MsgCmd:
			// In-game /pipe <text> command — feed to the trigger engine.
			cmd, err := zealpipe.DecodePipeCmd(env.Data)
			if err != nil || cmd.Text == "" {
				return
			}
			triggerEngine.HandlePipeCommand(cmd.Text, env.Character, time.Now())
			return
		case zealpipe.MsgPlayer:
			// Per-tick player snapshot. Feed zone + position into the NPC
			// tracker so it can disambiguate same-name NPC targets by
			// proximity to known spawn points. Doesn't trigger any
			// broadcast on its own — used at next target acquire.
			p, err := zealpipe.DecodePlayer(env.Data)
			if err != nil {
				return
			}
			npcTracker.SetPipePlayerSnapshot(p.Zone, p.Location.X, p.Location.Y, p.Location.Z)
			respawnEngine.SetPipeZone(p.Zone)
			return
		case zealpipe.MsgLabel:
			// Fall through to the label aggregator below.
		default:
			return
		}
		labels, err := zealpipe.DecodeLabels(env.Data)
		if err != nil {
			return
		}
		// Aggregate per-envelope state. Zeal omits labels with empty values
		// (Zeal/named_pipe.cpp:280), so absence of a label means the slot is
		// empty — we still need to push that as a snapshot so consumers
		// learn the prior value is gone.
		var castingName, targetName string
		targetHP := -1
		var buffSlots []string
		for _, l := range labels {
			switch l.Type {
			case zealpipe.LabelTargetName:
				targetName = l.Value
				if l.Value == "" {
					npcTracker.ClearPipeTarget()
				} else {
					npcTracker.SetPipeTarget(l.Value)
				}
				combatTracker.SetPipeTarget(l.Value)
			case zealpipe.LabelTargetHPPerc:
				if n, err := strconv.Atoi(strings.TrimSpace(l.Value)); err == nil {
					targetHP = n
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
		// Corpse-target death signal: drops detrimental timers when a boss
		// dies while still selected, even if the slain-line is out of log
		// range (raid casters far from the target). See HandlePipeTarget.
		timerEngine.HandlePipeTarget(targetName)
		triggerEngine.HandlePipeTarget(targetName, targetHP, env.Character, time.Now())
		triggerEngine.HandlePipeBuffSlots(buffSlots, env.Character, time.Now())
	})
	pipeSupervisor.OnConnect(func(pipeName string, pid uint32) {
		// Frontend listens for this and shows a one-shot "Zeal connected"
		// notification. Re-fires on every reconnect — the toast component
		// dedupes if it's been shown recently.
		hub.Broadcast(ws.Event{Type: "zeal:connected", Data: map[string]any{
			"pipe_name": pipeName,
			"pid":       pid,
		}})
	})
	pipeSupervisor.OnDisconnect(func() {
		// Drop pipe-only state from all consumers so stale values don't
		// linger after Zeal goes away. Log-driven paths continue to work via
		// the normal Handle() flow.
		npcTracker.ResetPipeFields()
		combatTracker.ResetPipeState()
		respawnEngine.ResetPipeZone()
		timerEngine.SetPipeCasting("")
		timerEngine.SetPipeBuffSlots(nil)
		triggerEngine.HandlePipeReset()
		hub.Broadcast(ws.Event{Type: "zeal:disconnected", Data: map[string]any{}})
	})
	go pipeSupervisor.Start(context.Background())

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
		respawnEngine.Handle(ev)
		rollTracker.Handle(ev)
		if playersConsumer != nil {
			playersConsumer.Handle(ev)
		}
		if chatConsumer != nil {
			chatConsumer.HandleEvent(ev)
		}
		if lootConsumer != nil {
			lootConsumer.HandleEvent(ev)
		}
		if skillsConsumer != nil {
			skillsConsumer.Handle(ev)
		}
	}, func(ts time.Time, msg string) {
		triggerEngine.Handle(ts, msg)
		chChainMatcher.HandleLine(ts, msg)
		if keyringConsumer != nil {
			keyringConsumer.HandleLine(ts, msg)
		}
		if lockoutConsumer != nil {
			lockoutConsumer.HandleLine(ts, msg)
		}
		if chatConsumer != nil {
			chatConsumer.HandleLine(ts, msg)
		}
		if lootConsumer != nil {
			lootConsumer.HandleLine(ts, msg)
		}
	}, func(character string) {
		slog.Info("logparser: auto-detected active character", "character", character)
		// If the player switched to a character that doesn't match the
		// configured manual override, drop the override so the new in-game
		// character becomes the active selection across the app. Without
		// this, the dropdown and character pages would keep showing the
		// previously-pinned character after a camp/login cycle.
		cfg := cfgMgr.Get()
		if cfg.Character != "" && !strings.EqualFold(cfg.Character, character) {
			cfg.Character = ""
			cfg.CharacterClass = -1
			if err := cfgMgr.Update(cfg); err != nil {
				slog.Warn("logparser: could not clear manual character override", "err", err)
			}
		}
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
		filepath.Join(home, ".pq-companion", "backups"),
		filepath.Join(home, ".pq-companion"),
		runtimeAppVersion(),
	)

	// SQL sandbox: opens its own read-only connection pool to quarm.db so a
	// runaway user query can't starve the main read pool. Failure here is
	// non-fatal — the Developer tab's SQL panel will just 503.
	sb, err := sandbox.Open(*dbPath)
	if err != nil {
		slog.Warn("open sql sandbox (developer SQL panel disabled)", "err", err)
		sb = nil
	} else {
		defer sb.Close()
	}

	// Saved-query store: persists user-authored SQL queries in user.db so
	// they survive app updates and can be exported as JSON packs. Non-fatal
	// — failure here just disables the Saved dropdown in the SQL panel.
	savedQueryStore, err := savedquery.OpenStore(filepath.Join(home, ".pq-companion", "user.db"))
	if err != nil {
		slog.Warn("open saved query store (Saved dropdown disabled)", "err", err)
		savedQueryStore = nil
	} else {
		defer savedQueryStore.Close()
	}

	router := api.NewRouter(database, hub, cfgMgr, zealWatcher, pipeSupervisor, backupMgr, tailer, npcTracker, combatTracker, historyStore, timerEngine, respawnEngine, triggerStore, triggerEngine, charStore, rollTracker, appBackupMgr, playerStore, chatStore, lootStore, backfillRegistry, keyringStore, keyringMaster, lockoutStore, sb, savedQueryStore, skillsStore, actualPort)

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
