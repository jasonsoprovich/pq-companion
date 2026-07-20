// Command server starts the PQ Companion HTTP API server.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/api"
	"github.com/jasonsoprovich/pq-companion/backend/internal/appbackup"
	"github.com/jasonsoprovich/pq-companion/backend/internal/applog"
	"github.com/jasonsoprovich/pq-companion/backend/internal/backfill"
	"github.com/jasonsoprovich/pq-companion/backend/internal/backup"
	"github.com/jasonsoprovich/pq-companion/backend/internal/buffmod"
	"github.com/jasonsoprovich/pq-companion/backend/internal/changelog"
	"github.com/jasonsoprovich/pq-companion/backend/internal/character"
	"github.com/jasonsoprovich/pq-companion/backend/internal/chat"
	"github.com/jasonsoprovich/pq-companion/backend/internal/chchain"
	"github.com/jasonsoprovich/pq-companion/backend/internal/combat"
	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/factiontracker"
	"github.com/jasonsoprovich/pq-companion/backend/internal/keyring"
	"github.com/jasonsoprovich/pq-companion/backend/internal/lockout"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/loot"
	"github.com/jasonsoprovich/pq-companion/backend/internal/overlay"
	"github.com/jasonsoprovich/pq-companion/backend/internal/players"
	"github.com/jasonsoprovich/pq-companion/backend/internal/popflag"
	"github.com/jasonsoprovich/pq-companion/backend/internal/raidthreat"
	"github.com/jasonsoprovich/pq-companion/backend/internal/respawn"
	"github.com/jasonsoprovich/pq-companion/backend/internal/rolltracker"
	"github.com/jasonsoprovich/pq-companion/backend/internal/sandbox"
	"github.com/jasonsoprovich/pq-companion/backend/internal/savedquery"
	"github.com/jasonsoprovich/pq-companion/backend/internal/skills"
	"github.com/jasonsoprovich/pq-companion/backend/internal/spelltimer"
	"github.com/jasonsoprovich/pq-companion/backend/internal/threat"
	"github.com/jasonsoprovich/pq-companion/backend/internal/trader"
	"github.com/jasonsoprovich/pq-companion/backend/internal/trigger"
	"github.com/jasonsoprovich/pq-companion/backend/internal/tts"
	"github.com/jasonsoprovich/pq-companion/backend/internal/wishlistwatch"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
	"github.com/jasonsoprovich/pq-companion/backend/internal/zeal"
	"github.com/jasonsoprovich/pq-companion/backend/internal/zealpipe"
)

// watchParentDeath exits the process when stdin reaches EOF, which happens when
// the Electron parent (which spawns us with piped stdio) dies. It's armed only
// when stdin is a pipe: an interactive `go run` has a tty (char device) and
// /dev/null is also a char device, so neither triggers it — only a supervised
// sidecar does.
func watchParentDeath() {
	fi, err := os.Stdin.Stat()
	if err != nil || fi.Mode()&os.ModeCharDevice != 0 {
		return // terminal or /dev/null — not a supervised sidecar
	}
	go func() {
		buf := make([]byte, 256)
		for {
			// The parent never writes to our stdin, so this blocks until the
			// pipe's write end closes (EOF) or errors — i.e. the parent exited.
			if _, err := os.Stdin.Read(buf); err != nil {
				slog.Info("parent process closed stdin; exiting sidecar")
				os.Exit(0)
			}
		}
	}()
}

// noopBackfillHandler satisfies backfill.Handler as a silent no-op, for a
// backfill section that can't proceed for one particular character (e.g. no
// character row exists yet for the Faction Tracker) without failing the
// whole run.
type noopBackfillHandler struct{}

func (noopBackfillHandler) HandleEvent(logparser.LogEvent) {}
func (noopBackfillHandler) HandleLine(time.Time, string)   {}
func (noopBackfillHandler) Finalize()                      {}
func (noopBackfillHandler) Inserted() int                  { return 0 }

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

	// Exit if the Electron parent dies. It spawns us with piped stdio, so a
	// crash / Task Manager kill of the main process closes our stdin's write
	// end and our read returns EOF. Without this the sidecar lingers, holding
	// user.db and a lock on the installed exe that wedges the next NSIS update.
	watchParentDeath()

	// Apply any pending app reset BEFORE config is loaded and user.db is opened.
	// A "data" reset wipes user.db + backups but keeps config.yaml; a "factory"
	// reset also moves config.yaml aside so config.Load below recreates defaults
	// and the app reopens to onboarding. Moving config.yaml aside must precede
	// config.Load, hence this runs first. Set-aside files get a .prereset suffix
	// for recovery.
	if home, hErr := os.UserHomeDir(); hErr == nil {
		appHome := filepath.Join(home, ".pq-companion")
		rm := appbackup.New(
			filepath.Join(appHome, "user.db"),
			filepath.Join(appHome, "backups"),
			appHome,
			filepath.Join(appHome, "config.yaml"),
			runtimeAppVersion(),
		)
		if mode, rErr := rm.ApplyPendingReset(); rErr != nil {
			slog.Error("apply pending app reset", "err", rErr)
		} else if mode != "" {
			slog.Info("applied pending app reset; moved data aside", "mode", mode)
		}
	}

	cfgMgr, err := config.Load()
	if err != nil {
		slog.Error("load config", "err", err)
		os.Exit(1)
	}
	slog.Info("config loaded", "path", cfgMgr.Path())
	// Honor the saved verbose-logging preference for the rest of this session;
	// the Settings toggle re-applies it live via the config update handler.
	applog.SetDebug(cfgMgr.Get().Preferences.DebugLogging)

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
		appBackup := appbackup.New(userDBPath, backupsDir, appHome, filepath.Join(appHome, "config.yaml"), runtimeAppVersion())
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

	// Note: the PoP item gate is no longer built at startup. It now loads from
	// a precomputed set embedded at build time (db/pop_gated.json, regenerated
	// by cmd/pop-index), so the gear-upgrade finder reads it instantly on first
	// use. Building it live was a multi-second loot/spawn join pass that, when
	// run here, delayed BACKEND_PORT and showed as a black launch screen.

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

	// Changelog: parsed once at startup for the "what's new" popup and the
	// Settings > Changelog tab. Non-fatal — a missing/unreadable file just
	// means those surfaces show nothing.
	changelogEntries, err := changelog.Load(defaultChangelogPath())
	if err != nil {
		slog.Warn("load changelog (feature disabled)", "err", err)
	} else {
		slog.Info("changelog loaded", "entries", len(changelogEntries))
	}

	// PoP flagging tracker: persists per-character planar-progression state.
	// Non-fatal — failing here only disables the PoP Flags page.
	popflagStore, err := popflag.OpenStore(filepath.Join(home, ".pq-companion", "user.db"))
	if err != nil {
		slog.Warn("open pop flag tracker (disabled)", "err", err)
		popflagStore = nil
	} else {
		defer popflagStore.Close()
	}

	// Lockout tracker: persists per-character /sll loot & legacy-item lockout
	// snapshots. Non-fatal — failing here only disables the Lockouts page.
	lockoutStore, err := lockout.OpenStore(filepath.Join(home, ".pq-companion", "user.db"))
	var lockoutConsumer *lockout.Consumer
	var popflagConsumer *popflag.Consumer
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
	// Fight inactivity timeout: read live from config so a settings change
	// applies to the next armed timer. This is the main way a parse ends now
	// that zoning/death don't force-clear fights. 0 falls back to the tracker's
	// built-in default; the config layer already backfills it to 60.
	combatTracker.SetFightTimeoutFn(func() time.Duration {
		secs := cfgMgr.Get().Combat.FightTimeoutSeconds
		if secs <= 0 {
			return 0
		}
		return time.Duration(secs) * time.Second
	})

	// Threat meter: estimates the active character's own per-mob hate from its
	// own combat log lines, reusing the same mob keys as the combat tracker.
	// Damage hate is observed directly; spell instant-hate comes from the game
	// DB. Always running so the overlay has data the moment it's opened.
	activeCharName := func() string {
		if tailer != nil {
			if name := tailer.ActiveCharacter(); name != "" {
				return name
			}
		}
		return cfgMgr.Get().Character
	}
	// Static hate modifier: the auto-detected Spell Casting Subtlety AA (the only
	// hate AA in Quarm) plus the user's manual gear modifier. Memoised — it only
	// changes when the character or their AAs change.
	staticHatemod := &staticHatemodProvider{
		activeName: activeCharName,
		manualPct:  func() int { return cfgMgr.Get().Preferences.ThreatHatemodPct },
		getChar:    charStore.GetByName,
		listAAs:    charStore.ListAAs,
		aaBonuses:  database.AAStatBonuses,
	}
	threatTracker := threat.NewTracker(hub, threat.NewCalculator(database, database), staticHatemod.value)
	// Melee swing hate is a flat per-swing value (equipped weapon damage +
	// primary-hand bonus), not the white damage rolled — so the meter needs the
	// active character's equipped primary weapon, level, and class. Resolved from
	// the Zeal inventory export + character row, memoised, with a graceful
	// fall-back to observed damage when any of those are unknown.
	meleeHate := &meleeSwingHateProvider{
		activeName: activeCharName,
		inventory:  zealWatcher.Inventory,
		getItem:    database.GetItem,
		getChar:    charStore.GetByName,
	}
	threatTracker.SetMeleeSwingHateFn(meleeHate.value)
	// Backstab hate is its flat base damage (skill/weapon-derived), not the large
	// rolled backstab number — so it needs the same primary-weapon lookup, gated
	// to a piercer.
	threatTracker.SetBackstabHateFn(meleeHate.backstab)
	// selfNameFn resolves the active character's display name so a taunt
	// emote (or, for the raid assembler below, a departure spell) naming the
	// player can be recognised as "us". Shared by both the personal meter and
	// the raid-wide assembler so they agree on who "You" is.
	selfNameFn := func() string {
		if tailer != nil {
			if name := tailer.ActiveCharacter(); name != "" {
				return name
			}
		}
		return cfgMgr.Get().Character
	}
	threatTracker.SetSelfNameFn(selfNameFn)
	// A successful Taunt sets hate to topHate+10 server-side, but this meter
	// only ever sees the active character's own outgoing damage — it has no
	// way to know what "the top" is on its own. Estimate it from the combat
	// tracker's raid-wide observed damage (the same data source the raid
	// threat assembler uses), which is always being collected for the DPS
	// meter regardless of whether the raid threat feature is enabled.
	threatTracker.SetTopHateFn(func(mob string) int64 {
		var top int64
		for _, md := range combatTracker.RaidThreatDamage() {
			if md.Mob != mob {
				continue
			}
			for _, atk := range md.Attackers {
				if atk.Name == "You" {
					continue
				}
				if atk.Damage > top {
					top = atk.Damage
				}
			}
			break
		}
		return top
	})
	// Drive the live (rolling-window) hate rate: rebroadcast once a second while
	// any mob is tracked so the per-second meter decays on screen between log
	// events instead of freezing at its last value.
	go threatTracker.RunLiveTicker(context.Background(), time.Second)

	// Coalesce combat-meter broadcasts: per-hit/heal updates mark the state
	// dirty and this ticker flushes them at most once a second, instead of
	// re-marshaling the whole combat state (merged fight + archived fights +
	// deaths) on every damage line during raid AoE spam. Fight-start/end and
	// death transitions still broadcast immediately.
	go combatTracker.RunLiveTicker(context.Background(), time.Second)

	// Raid-estimate threat: an experimental per-mob, per-player ESTIMATED hate
	// view assembled from the combat tracker's observed damage (×class/player
	// hate mods) plus this character's high-fidelity personal hate. Dev-gated;
	// reads its config live. Rebroadcasts once a second while combat is active.
	raidThreatAssembler := raidthreat.NewAssembler(hub, combatTracker, threatTracker,
		func() bool { return cfgMgr.Get().Preferences.RaidThreatEnabled },
		func() map[string]int { return cfgMgr.Get().Preferences.RaidThreatClassMods },
		func() map[string]int { return cfgMgr.Get().Preferences.RaidThreatPlayerMods },
		selfNameFn, // so a taunt emote naming the player maps to "You"
	)
	go raidThreatAssembler.RunTicker(context.Background(), time.Second)

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
	// classByNameCache memoizes per-name base-class lookups. stampClasses
	// re-resolves every combatant on each combat broadcast, and each miss is a
	// user.db read taken under the tracker lock — heavy during a raid with many
	// distinct combatants. A player's base class is stable within a session, so
	// a short TTL eliminates the per-frame reads while staying self-healing if an
	// early /who reported an odd class. Guarded by its own mutex since the
	// closure must be safe regardless of who calls it.
	type classCacheEntry struct {
		cls string
		exp time.Time
	}
	var classCacheMu sync.Mutex
	classByNameCache := map[string]classCacheEntry{}
	const classCacheTTL = 30 * time.Second

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
			classCacheMu.Lock()
			if e, ok := classByNameCache[name]; ok && time.Now().Before(e.exp) {
				classCacheMu.Unlock()
				return e.cls
			}
			classCacheMu.Unlock()
			s, err := playerStore.Get(name)
			if err != nil || s == nil {
				return ""
			}
			// EffectiveClass prefers the /who-discovered class but falls back
			// to the user's manual override for always-anonymous players, so
			// DPS bars colour correctly for hidden guildmates.
			cls := players.BaseClassOf(s.EffectiveClass)
			classCacheMu.Lock()
			classByNameCache[name] = classCacheEntry{cls: cls, exp: time.Now().Add(classCacheTTL)}
			classCacheMu.Unlock()
			return cls
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
	}, func() []int {
		// Owned item IDs for the active character, from the latest Zeal
		// inventory export — lets the engine break ties between clickies that
		// share land text by picking the one the player actually carries.
		inv := zealWatcher.Inventory()
		if inv == nil {
			return nil
		}
		var charName string
		if tailer != nil {
			charName = tailer.ActiveCharacter()
		}
		if charName != "" && inv.Character != "" &&
			!strings.EqualFold(inv.Character, charName) {
			return nil // stale export for a different character
		}
		ids := make([]int, 0, len(inv.Entries))
		for _, entry := range inv.Entries {
			if entry.ID > 0 {
				ids = append(ids, entry.ID)
			}
		}
		return ids
	}, func() bool {
		return cfgMgr.Get().SpellTimer.KeepExpiredTimers
	})
	go timerEngine.Start(context.Background())

	// CH-chain matcher: watches raid chat for chain-call lines and creates
	// ch_chain countdown timers in the engine. Reads its regex/cadence/enabled
	// state live from config so Settings changes take effect without a restart.
	chChainMatcher := chchain.New(timerEngine, func() config.CHChainSettings {
		return cfgMgr.Get().CHChain
	})

	// CH-chain heal watcher: confirms a chain timer's Complete Healing
	// actually landed so the overlay doesn't flag it a possible miss. See
	// internal/chchain/heal_watcher.go for why it's scoped to exactly one
	// spell's bystander message.
	chChainHealWatcher := chchain.NewHealWatcher(timerEngine, func() config.CHChainSettings {
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
	// In-place upgrade for buff triggers imported before target capture: wraps
	// the cast-on-other branch's name in a capture group and sets the target
	// field so group buffs show the "on <target>" overlay suffix. Only patches
	// un-customized built-in rows.
	if err := triggerStore.MigrateAddBuffTargetCapture(); err != nil {
		slog.Warn("trigger buff-target-capture migration failed", "err", err)
	}
	// In-place broadening of the Raid Alerts assist-call trigger to also match
	// "kill" calls and dash-arrow decorations. Only rewrites the pattern of the
	// un-customized built-in row.
	if err := triggerStore.MigrateBroadenAssistCallPattern(); err != nil {
		slog.Warn("trigger assist-call broaden migration failed", "err", err)
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
	// {target}/{t} tokens in action text resolve to the NPC overlay's
	// inferred combat target.
	triggerEngine.SetTargetProvider(func() string {
		return npcTracker.GetState().TargetName
	})
	triggerEngine.Reload()

	// Wishlist watcher: alerts when an item on any character's wishlist
	// appears in the active character's log. Not a trigger — see
	// internal/wishlistwatch for why — but it broadcasts a synthetic
	// trigger:fired event so the existing overlay/TTS/sound pipeline renders
	// it, the same approach the PVP warning below uses.
	wishlistWatcher := wishlistwatch.NewWatcher(
		hub, cfgMgr, activeChar,
		func() ([]wishlistwatch.CharacterInfo, error) {
			chars, err := charStore.List()
			if err != nil {
				return nil, err
			}
			out := make([]wishlistwatch.CharacterInfo, len(chars))
			for i, c := range chars {
				out[i] = wishlistwatch.CharacterInfo{ID: c.ID, Name: c.Name}
			}
			return out, nil
		},
		func(characterID int) ([]wishlistwatch.WishlistEntry, error) {
			entries, err := charStore.ListWishlist(characterID)
			if err != nil {
				return nil, err
			}
			out := make([]wishlistwatch.WishlistEntry, len(entries))
			for i, e := range entries {
				out[i] = wishlistwatch.WishlistEntry{CharacterID: e.CharacterID, ItemID: e.ItemID}
			}
			return out, nil
		},
		func(itemID int) (wishlistwatch.ItemInfo, bool) {
			it, err := database.GetItem(itemID)
			if err != nil || it == nil {
				return wishlistwatch.ItemInfo{}, false
			}
			return wishlistwatch.ItemInfo{Name: it.Name}, true
		},
	)
	wishlistWatcher.Rebuild()

	// Faction Tracker: a per-character tally of "Your faction standing with X
	// got better/worse" lines for EVERY faction the character has killed
	// toward or /con'd — not just pinned ones, the same "record everything
	// encountered" approach as the Lockout and Player trackers — with a
	// best-effort point estimate for changes that correlate to a resolved
	// kill (quarm.db npc_faction_entries), persisted in user.db across
	// restarts and character switches. Plus a /con bucket reading per
	// faction (see logparser.FactionBucket), flagged suspect while the
	// player is illusioned. Pinning (the faction wishlist) is purely a
	// display favorite resolved by the API/frontend — it never gates what
	// the engine records. EQ never exposes a faction's absolute value, so
	// none of this is ever a claim about real standing — see
	// internal/factiontracker. Dev-gated.
	// Shared resolvers — used by both the live engine below and the
	// Faction Tracker's log-backfill handler (registered further down),
	// so the two never drift on how a kill/consider/quest-dialogue line
	// resolves to a faction.
	factionKillResolver := func(mobName string) ([]factiontracker.NPCFactionHit, bool) {
		id, ok := database.GetNPCIDByName(mobName)
		if !ok {
			return nil, false
		}
		nf, err := database.GetNPCFaction(id)
		if err != nil || nf == nil || len(nf.Hits) == 0 {
			return nil, false
		}
		hits := make([]factiontracker.NPCFactionHit, len(nf.Hits))
		for i, h := range nf.Hits {
			hits[i] = factiontracker.NPCFactionHit{FactionID: h.FactionID, FactionName: h.FactionName, Value: h.Value}
		}
		return hits, true
	}
	factionPrimaryResolver := func(npcName string) (int, string, bool) {
		id, ok := database.GetNPCIDByName(npcName)
		if !ok {
			return 0, "", false
		}
		nf, err := database.GetNPCFaction(id)
		if err != nil || nf == nil || nf.PrimaryFactionName == "" {
			return 0, "", false
		}
		return nf.PrimaryFactionID, nf.PrimaryFactionName, true
	}
	// Quest turn-in faction changes get an exact numeric delta (not a
	// kill-correlated estimate) by matching the NPC's spoken log line
	// against the quest script's own dialogue text — see
	// db.ResolveQuestFactionDialogue and quest_sources.json.
	factionDialogueResolver := func(npcName, text string) ([]factiontracker.NPCFactionHit, bool) {
		deltas, ok := db.ResolveQuestFactionDialogue(npcName, text)
		if !ok || len(deltas) == 0 {
			return nil, false
		}
		hits := make([]factiontracker.NPCFactionHit, 0, len(deltas))
		for _, d := range deltas {
			f, err := database.GetFactionByID(d.FactionID)
			if err != nil || f == nil {
				continue
			}
			hits = append(hits, factiontracker.NPCFactionHit{FactionID: f.ID, FactionName: f.Name, Value: d.Delta})
		}
		return hits, len(hits) > 0
	}

	factionEngine := factiontracker.NewEngine(hub, factionKillResolver)
	factionEngine.SetPrimaryFactionResolver(factionPrimaryResolver)
	factionEngine.SetFactionIDResolver(database.GetFactionIDByName)
	factionEngine.SetQuestDialogueResolver(factionDialogueResolver)
	// A /con reading is unreliable while the player is illusioned — check
	// every currently active buff timer's spell effects for SPA 58
	// (Illusion), the same check buffmod already uses for the Permanent
	// Illusion AA override.
	factionEngine.SetIllusionProvider(func() bool {
		for _, t := range timerEngine.GetState().Timers {
			if t.Category != spelltimer.CategoryBuff || t.SpellID == 0 {
				continue
			}
			sp, err := database.GetSpell(t.SpellID)
			if err != nil || sp == nil {
				continue
			}
			if buffmod.HasIllusionEffect(sp.EffectIDs[:]) {
				return true
			}
		}
		return false
	})
	factionEngine.SetPersistFunc(func(characterID int, tally factiontracker.Tally) {
		row := character.FactionTallyRow{
			CharacterID:         characterID,
			FactionID:           tally.FactionID,
			FactionName:         tally.FactionName,
			Better:              tally.Better,
			Worse:               tally.Worse,
			EstimatedNet:        tally.EstimatedNet,
			Unresolved:          tally.Unresolved,
			LastBucket:          tally.LastBucket,
			LastConsiderSuspect: tally.LastConsiderSuspect,
		}
		if tally.LastConsideredAt != nil {
			row.LastConsideredAt = tally.LastConsideredAt.Unix()
		}
		if err := charStore.UpsertFactionTally(row); err != nil {
			slog.Warn("persist faction tally", "err", err)
		}
	})
	factionEngine.SetClearPersistedFunc(func(characterID int) {
		if err := charStore.ClearFactionTallies(characterID); err != nil {
			slog.Warn("clear faction tallies", "err", err)
		}
	})
	reloadFactionTracking := func() {
		charName := activeChar()
		if charName == "" {
			factionEngine.SetCharacter(0, nil)
			return
		}
		c, ok, err := charStore.GetByName(charName)
		if err != nil || !ok {
			factionEngine.SetCharacter(0, nil)
			return
		}
		tallyRows, err := charStore.ListFactionTallies(c.ID)
		if err != nil {
			slog.Warn("load faction tallies", "err", err)
			return
		}
		tallies := make([]factiontracker.Tally, len(tallyRows))
		for i, row := range tallyRows {
			t := factiontracker.Tally{
				FactionID:           row.FactionID,
				FactionName:         row.FactionName,
				Better:              row.Better,
				Worse:               row.Worse,
				EstimatedNet:        row.EstimatedNet,
				Unresolved:          row.Unresolved,
				LastBucket:          row.LastBucket,
				LastConsiderSuspect: row.LastConsiderSuspect,
			}
			if row.LastConsideredAt > 0 {
				ts := time.Unix(row.LastConsideredAt, 0)
				t.LastConsideredAt = &ts
			}
			tallies[i] = t
		}
		factionEngine.SetCharacter(c.ID, tallies)
	}
	reloadFactionTracking()

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

	if popflagStore != nil {
		popflagConsumer = popflag.NewConsumer(popflagStore, activeChar)
		popflagConsumer.SetOnSnapshot(func(character string) {
			hub.Broadcast(ws.Event{
				Type: "popflag.snapshot",
				Data: map[string]any{"character": character},
			})
		})
	}

	if chatStore != nil {
		chatConsumer = chat.NewConsumer(chatStore, activeChar)
		chatConsumer.SetOnInsert(func(m chat.Message) {
			hub.Broadcast(ws.Event{Type: "chat:new", Data: m})
			// Direct tells double as player-tracker interactions, so people
			// the user actually talks to show up in the tracker even if they
			// never appear in a /who.
			if playersConsumer != nil && m.Channel == chat.ChannelTell {
				playersConsumer.RecordTell(m.Peer, time.Unix(m.TS, 0))
			}
		})
	}

	// PVP warning: when a PVP-flagged player shows up in a live /who, fire a
	// synthetic trigger:fired event so the existing trigger overlay + audio
	// engine handle the visual and TTS warning — no trigger pack or regex
	// upkeep, the consumer matches flagged names exactly.
	if playersConsumer != nil {
		playersConsumer.SetOnPVPSighting(func(name, zone, source string) {
			// Checked at fire time (not wiring time) so the Players page
			// toggle takes effect without a restart.
			if cfgMgr.Get().Preferences.PVPWarningDisabled {
				return
			}
			where := "in /who"
			if source == "group" {
				where = "joined your group"
			}
			text := fmt.Sprintf("PVP: %s %s", name, where)
			if zone != "" && source == "who" {
				text += " — " + zone
			}
			hub.Broadcast(ws.Event{Type: trigger.WSEventTriggerFired, Data: trigger.TriggerFired{
				TriggerID:   "system:pvp-warning",
				TriggerName: "PVP Warning",
				MatchedLine: text,
				Actions: []trigger.Action{
					{Type: trigger.ActionOverlayText, Text: "⚔ " + text, DurationSecs: 8, Color: "#ef4444"},
					{Type: trigger.ActionTextToSpeech, Text: fmt.Sprintf("P V P warning: %s", name)},
				},
				FiredAt: time.Now(),
			}})
			slog.Info("pvp warning fired", "name", name, "zone", zone, "source", source)
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
	if charStore != nil {
		// Unlike the other trackers above (keyed by log-owning character
		// name), the Faction Tracker's storage is keyed by charStore's
		// internal character id — resolve it once per backfill run. If the
		// character has never been seen by charStore (no character row
		// yet), skip faction backfill for them entirely rather than erroring.
		backfillRegistry.Register(backfill.Section{
			Key:   "factions",
			Label: "Faction Tracker",
			NewHandler: func(charName string) backfill.Handler {
				c, ok, err := charStore.GetByName(charName)
				if err != nil || !ok {
					return noopBackfillHandler{}
				}
				characterID := c.ID
				return factiontracker.NewBackfillHandler(
					factionKillResolver, factionPrimaryResolver, database.GetFactionIDByName, factionDialogueResolver,
					func(tally factiontracker.Tally) (bool, error) {
						row := character.FactionTallyRow{
							CharacterID:  characterID,
							FactionID:    tally.FactionID,
							FactionName:  tally.FactionName,
							Better:       tally.Better,
							Worse:        tally.Worse,
							EstimatedNet: tally.EstimatedNet,
							Unresolved:   tally.Unresolved,
						}
						if tally.LastBucket != "" {
							row.LastBucket = tally.LastBucket
						}
						if tally.LastConsideredAt != nil {
							row.LastConsideredAt = tally.LastConsideredAt.Unix()
							row.LastConsiderSuspect = tally.LastConsiderSuspect
						}
						return charStore.MergeBackfillFactionTally(row)
					},
				)
			},
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

	// Local-TTS cache retention: reclaim generated WAVs (from every local-TTS
	// provider — Piper, Kokoro — sharing the one tts-cache dir) that have gone
	// unused for a while (touched on every cache hit, so anything still in
	// active use never ages out — see internal/tts/cache.go). Same
	// run-once-then-daily shape as the chat purge above; independent of
	// whether any provider is currently enabled, since disabling one shouldn't
	// strand its old cache files forever.
	go func() {
		sweep := func() {
			if n, err := tts.SweepOldCache(filepath.Dir(cfgMgr.Path())); err != nil {
				slog.Warn("local tts cache sweep failed", "err", err)
			} else if n > 0 {
				slog.Info("local tts cache sweep", "deleted", n)
			}
		}
		sweep()
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			sweep()
		}
	}()

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
		var castingName, targetName, targetPetOwner string
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
				threatTracker.SetPipeTarget(l.Value)
				raidThreatAssembler.SetPipeTarget(l.Value)
				raidThreatAssembler.Broadcast()
			case zealpipe.LabelTargetHPPerc:
				if n, err := strconv.Atoi(strings.TrimSpace(l.Value)); err == nil {
					targetHP = n
					npcTracker.SetPipeHPPercent(n)
				}
			case zealpipe.LabelTargetPetOwner:
				npcTracker.SetPipePetOwner(l.Value)
				targetPetOwner = l.Value
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
		// When the current target is a pet, Zeal reports its owner. Bind it for
		// DPS attribution (the NPC overlay already consumes the same label) so
		// targeting your own charm pet to heal/buff it is enough to roll its
		// damage up to you. Done after the loop so it doesn't depend on label
		// order within the envelope.
		if targetName != "" && targetPetOwner != "" {
			combatTracker.SetPipeTargetPetOwner(targetName, targetPetOwner)
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
		npcTracker.SetPipeConnected(true)
		hub.Broadcast(ws.Event{Type: "zeal:connected", Data: map[string]any{
			"pipe_name": pipeName,
			"pid":       pid,
		}})
	})
	pipeSupervisor.OnDisconnect(func() {
		// Drop pipe-only state from all consumers so stale values don't
		// linger after Zeal goes away. Log-driven paths continue to work via
		// the normal Handle() flow.
		npcTracker.SetPipeConnected(false)
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
	// broadcasts overlay:rolls WebSocket events. Sessions are stateless
	// across restarts, but the winner-rule / mode / timer preferences are
	// seeded from config and persisted back whenever the user changes them
	// (e.g. a guild that always rolls lowest-wins).
	rollTracker := rolltracker.New(hub)
	{
		p := cfgMgr.Get().Preferences
		var profile rolltracker.RollProfile
		if p.RollTrackerProfile != "" {
			if err := json.Unmarshal([]byte(p.RollTrackerProfile), &profile); err != nil {
				slog.Warn("parse roll tracker profile, ignoring", "err", err)
				profile = rolltracker.RollProfile{}
			}
		}
		rollTracker.Configure(
			rolltracker.WinnerRule(p.RollTrackerWinnerRule),
			rolltracker.Mode(p.RollTrackerMode),
			p.RollTrackerAutoStopSeconds,
			profile,
		)
		rollTracker.SetOnChange(func(rule rolltracker.WinnerRule, mode rolltracker.Mode, secs int, profile rolltracker.RollProfile) {
			err := cfgMgr.Modify(func(c *config.Config) {
				c.Preferences.RollTrackerWinnerRule = string(rule)
				c.Preferences.RollTrackerMode = string(mode)
				c.Preferences.RollTrackerAutoStopSeconds = secs
				// Persist the profile as a JSON blob; simple-mode profiles are
				// stored empty so the config stays clean.
				if profile.Mode == rolltracker.ProfileTiered {
					if b, err := json.Marshal(profile); err == nil {
						c.Preferences.RollTrackerProfile = string(b)
					}
				} else {
					c.Preferences.RollTrackerProfile = ""
				}
			})
			if err != nil {
				slog.Warn("persist roll tracker settings", "err", err)
			}
		})
		// Best-effort loot-item auto-suggest: resolve a raid/chat line like
		// "Robe of the Lost Circle 333" to the canonical item name so the
		// matching roll session is labeled automatically. Best-effort and
		// always user-overridable.
		rollTracker.SetItemMatcher(database.MatchItemNameInText)
	}

	// Central dispatch callbacks, shared by the live tailer and the log
	// replayer so a replayed line drives exactly the same consumers as a
	// live one.
	dispatchEvent := func(ev logparser.LogEvent) {
		hub.Broadcast(ws.Event{Type: string(ev.Type), Data: ev})
		npcTracker.Handle(ev)
		combatTracker.Handle(ev)
		threatTracker.Handle(ev)
		raidThreatAssembler.Handle(ev)
		timerEngine.Handle(ev)
		respawnEngine.Handle(ev)
		rollTracker.Handle(ev)
		factionEngine.Handle(ev)
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
		// PoP flag auto-detection: boss kills (and zone-ins) optimistically set
		// 'auto'-sourced flags for the active character.
		if popflagConsumer != nil {
			switch ev.Type {
			case logparser.EventKill:
				if kd, ok := ev.Data.(logparser.KillData); ok {
					popflagConsumer.HandleEvent("kill", kd.Target)
				}
			case logparser.EventZone:
				if zd, ok := ev.Data.(logparser.ZoneData); ok {
					popflagConsumer.HandleEvent("zone", zd.ZoneName)
				}
			}
		}
	}
	dispatchLine := func(ts time.Time, msg string) {
		// Opt-in raw passthrough: when enabled, surface lines that match no
		// known event pattern to the live feed as log:raw so they're visible
		// and searchable there. Classified lines already arrive via
		// dispatchEvent, so only broadcast the ones ParseLine would drop.
		if tailer != nil && tailer.RawFeed() {
			if _, ok := logparser.ClassifyMessage(msg); !ok {
				hub.Broadcast(ws.Event{Type: string(logparser.EventRaw), Data: logparser.LogEvent{
					Type:      logparser.EventRaw,
					Timestamp: ts,
					Message:   msg,
				}})
			}
		}
		triggerEngine.Handle(ts, msg)
		wishlistWatcher.HandleLine(msg)
		chChainMatcher.HandleLine(ts, msg)
		chChainHealWatcher.HandleLine(msg)
		rollTracker.HandleLine(ts, msg)
		if keyringConsumer != nil {
			keyringConsumer.HandleLine(ts, msg)
		}
		if lockoutConsumer != nil {
			lockoutConsumer.HandleLine(ts, msg)
		}
		if popflagConsumer != nil {
			popflagConsumer.HandleLine(ts, msg)
		}
		if chatConsumer != nil {
			chatConsumer.HandleLine(ts, msg)
		}
		if lootConsumer != nil {
			lootConsumer.HandleLine(ts, msg)
		}
		if playersConsumer != nil {
			playersConsumer.HandleLine(ts, msg)
		}
	}

	// Log tailer: reads new lines from the EQ log file and broadcasts parsed
	// events to all connected WebSocket clients. Also feeds overlay trackers
	// and the trigger engine.
	tailer = logparser.NewTailer(cfgMgr, dispatchEvent, dispatchLine, func(character string) {
		slog.Info("logparser: auto-detected active character", "character", character)
		// If the player switched to a character that doesn't match the
		// configured manual override, drop the override so the new in-game
		// character becomes the active selection across the app. Without
		// this, the dropdown and character pages would keep showing the
		// previously-pinned character after a camp/login cycle.
		cfg := cfgMgr.Get()
		if cfg.Character != "" && !strings.EqualFold(cfg.Character, character) {
			err := cfgMgr.Modify(func(c *config.Config) {
				// Re-check under the lock: another writer may have already
				// changed Character since the Get() above.
				if c.Character != "" && !strings.EqualFold(c.Character, character) {
					c.Character = ""
					c.CharacterClass = -1
				}
			})
			if err != nil {
				slog.Warn("logparser: could not clear manual character override", "err", err)
			}
		}
		hub.Broadcast(ws.Event{Type: "config:character_detected", Data: map[string]string{"character": character}})
		// Active character changed — drop cached buffmod contributors so the
		// next cast recomputes against the new character's inventory + AAs.
		timerEngine.RefreshModifiers()
		// Recompile trigger patterns: {c}/{char}/{self} tokens expand to the
		// active character name at compile time.
		triggerEngine.Reload()
		// Faction Tracker follows the active character: reload its wishlist
		// and load its persisted tallies, so a camp/login cycle switches to
		// that character's own faction history instead of carrying over the
		// previous character's tracked-faction set.
		reloadFactionTracking()
	})
	go tailer.Start(context.Background())

	// Log replayer: streams a historical log segment through the same
	// dispatch callbacks at log-timestamp pace, so triggers, timers, and the
	// combat meter behave as if the session were live — for testing and
	// debugging trigger setups against real gameplay. Live tailing pauses
	// for the duration of a session; replay-driven state (timers, current
	// fight) is cleared when the session ends so ghosts don't linger.
	replayer := logparser.NewReplayer(dispatchEvent, dispatchLine, func(active bool) {
		tailer.SetPaused(active)
		if !active {
			timerEngine.ClearAll()
			combatTracker.Reset()
		}
	}, func(st logparser.ReplayStatus) {
		hub.Broadcast(ws.Event{Type: "replay:status", Data: st})
	})

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
		filepath.Join(home, ".pq-companion", "config.yaml"),
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

	// Bazaar Trader Tracker store + auto-capturer (developer-tab feature):
	// records trader inventory snapshots and infers bazaar sales by diffing
	// them. Non-fatal — failure here just disables the Trader Tracker page.
	// The capturer scans on its own goroutine so it never blocks startup.
	traderStore, err := trader.OpenStore(filepath.Join(home, ".pq-companion", "user.db"))
	var traderCapturer *trader.Capturer
	if err != nil {
		slog.Warn("open trader tracker (disabled)", "err", err)
		traderStore = nil
	} else {
		defer traderStore.Close()
		traderCapturer = trader.NewCapturer(cfgMgr, traderStore, hub)
		go traderCapturer.Start(context.Background())
	}

	router := api.NewRouter(database, hub, cfgMgr, zealWatcher, pipeSupervisor, backupMgr, tailer, replayer, npcTracker, combatTracker, historyStore, threatTracker, raidThreatAssembler, timerEngine, respawnEngine, triggerStore, triggerEngine, charStore, rollTracker, appBackupMgr, playerStore, chatStore, lootStore, backfillRegistry, keyringStore, keyringMaster, lockoutStore, sb, savedQueryStore, skillsStore, traderStore, traderCapturer, popflagStore, wishlistWatcher, changelogEntries, factionEngine, actualPort)

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

// defaultChangelogPath returns the path to CHANGELOG.md relative to the
// executable's directory (see the `bin/CHANGELOG.md` extraResources mapping
// in electron-builder.yml), falling back to the repo-relative development
// path (CHANGELOG.md lives at the repo root, one level up from backend/).
func defaultChangelogPath() string {
	exe, err := os.Executable()
	if err == nil {
		candidate := filepath.Join(filepath.Dir(exe), "CHANGELOG.md")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return filepath.Join("..", "CHANGELOG.md")
}
