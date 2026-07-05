// Package api provides the HTTP REST API for the PQ Companion backend.
package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jasonsoprovich/pq-companion/backend/internal/appbackup"
	"github.com/jasonsoprovich/pq-companion/backend/internal/backfill"
	"github.com/jasonsoprovich/pq-companion/backend/internal/backup"
	"github.com/jasonsoprovich/pq-companion/backend/internal/character"
	"github.com/jasonsoprovich/pq-companion/backend/internal/chat"
	"github.com/jasonsoprovich/pq-companion/backend/internal/combat"
	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/eqw"
	"github.com/jasonsoprovich/pq-companion/backend/internal/keyring"
	"github.com/jasonsoprovich/pq-companion/backend/internal/lockout"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
	"github.com/jasonsoprovich/pq-companion/backend/internal/loot"
	"github.com/jasonsoprovich/pq-companion/backend/internal/overlay"
	"github.com/jasonsoprovich/pq-companion/backend/internal/players"
	"github.com/jasonsoprovich/pq-companion/backend/internal/popflag"
	"github.com/jasonsoprovich/pq-companion/backend/internal/quarm"
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
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
	"github.com/jasonsoprovich/pq-companion/backend/internal/zeal"
	"github.com/jasonsoprovich/pq-companion/backend/internal/zealpipe"
)

// NewRouter builds and returns the chi router wired to all backend components.
// combatHistory may be nil when persistence is disabled (e.g. user.db open
// failed); in that case the history endpoints respond 503.
func NewRouter(database *db.DB, hub *ws.Hub, cfgMgr *config.Manager, zealWatcher *zeal.Watcher, pipeSupervisor *zealpipe.Supervisor, backupMgr *backup.Manager, tailer *logparser.Tailer, replayer *logparser.Replayer, npcTracker *overlay.NPCTracker, combatTracker *combat.Tracker, combatHistory *combat.HistoryStore, threatTracker *threat.Tracker, raidThreatAssembler *raidthreat.Assembler, timerEngine *spelltimer.Engine, respawnEngine *respawn.Engine, triggerStore *trigger.Store, triggerEngine *trigger.Engine, charStore *character.Store, rollTracker *rolltracker.Tracker, appBackupMgr *appbackup.Manager, playerStore *players.Store, chatStore *chat.Store, lootStore *loot.Store, backfillRegistry *backfill.Registry, keyringStore *keyring.Store, keyringMaster []keyring.MasterEntry, lockoutStore *lockout.Store, sb *sandbox.Sandbox, savedQueryStore *savedquery.Store, skillsStore *skills.Store, traderStore *trader.Store, traderCapturer *trader.Capturer, popflagStore *popflag.Store, actualPort int) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	// Allow requests from the Vite dev server and any local renderer.
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, PUT, POST, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	})

	items := &itemsHandler{db: database}
	spells := &spellsHandler{db: database, cfgMgr: cfgMgr}
	npcs := &npcsHandler{db: database}
	resistCalc := &resistHandler{db: database, cfgMgr: cfgMgr}
	charmH := &charmHandler{db: database, cfgMgr: cfgMgr}
	traderH := &traderHandler{store: traderStore, capturer: traderCapturer, cfgMgr: cfgMgr, db: database}
	zones := &zonesHandler{db: database}
	recipes := &recipesHandler{db: database}
	quests := &questsHandler{db: database}
	favRecipes := &favoriteRecipesHandler{store: charStore, db: database}
	cfg := &configHandler{mgr: cfgMgr, hub: hub, backupMgr: backupMgr, actualPort: actualPort}
	charactersH := &charactersHandler{store: charStore, mgr: cfgMgr, db: database, watcher: zealWatcher}
	search := &searchHandler{db: database}
	zealH := &zealHandler{watcher: zealWatcher, cfgMgr: cfgMgr, db: database, pipe: pipeSupervisor, latest: zeal.NewLatestFetcher()}
	keysH := &keysHandler{watcher: zealWatcher}
	backupH := &backupHandler{mgr: backupMgr}
	appBackupH := &appBackupHandler{mgr: appBackupMgr}
	playersH := &playersHandler{store: playerStore}
	chatH := &chatHandler{store: chatStore, mgr: cfgMgr, tailer: tailer}
	lootH := &lootHandler{store: lootStore, mgr: cfgMgr, tailer: tailer}
	backfillH := &backfillHandler{registry: backfillRegistry, mgr: cfgMgr, tailer: tailer, hub: hub}
	keyringH := &keyringHandler{store: keyringStore, master: keyringMaster}
	lockoutsH := &lockoutsHandler{store: lockoutStore, db: database}
	logH := &logHandler{tailer: tailer, mgr: cfgMgr}
	replayH := &replayHandler{mgr: cfgMgr, replayer: replayer}
	overlayH := &overlayHandler{npcTracker: npcTracker}
	combatH := &combatHandler{tracker: combatTracker, historyStore: combatHistory}
	threatH := &threatHandler{tracker: threatTracker}
	raidThreatH := &raidThreatHandler{assembler: raidThreatAssembler}
	timerH := &timerHandler{engine: timerEngine}
	respawnH := &respawnHandler{engine: respawnEngine}
	triggerH := &triggerHandler{store: triggerStore, engine: triggerEngine, hub: hub, charStore: charStore, tailer: tailer, cfgMgr: cfgMgr}
	tasksH := &tasksHandler{store: charStore}
	skillsH := &skillsHandler{charStore: charStore, store: skillsStore, db: database}
	wishlistH := &wishlistHandler{store: charStore, db: database, hub: hub}
	rollsH := &rollsHandler{tracker: rollTracker}
	raw := &rawHandler{db: database}
	enumsH := &enumsHandler{}
	quarmH := &quarmHandler{cfgMgr: cfgMgr, fetcher: quarm.NewManifestFetcher()}
	eqwH := &eqwHandler{cfgMgr: cfgMgr, latest: eqw.NewLatestFetcher()}
	sandboxH := &sandboxHandler{sb: sb, cfgMgr: cfgMgr}
	savedQueryH := &savedQueryHandler{store: savedQueryStore, cfgMgr: cfgMgr}
	popflagH := &popflagHandler{store: popflagStore, hub: hub, mgr: cfgMgr}

	r.Route("/api", func(r chi.Router) {
		r.Use(middleware.SetHeader("Content-Type", "application/json"))
		r.Get("/search", search.global)
		r.Get("/enums", enumsH.get)
		r.Route("/items", func(r chi.Router) {
			r.Get("/", items.search)
			r.Get("/{id}", items.get)
			r.Get("/{id}/sources", items.sources)
			r.Get("/{id}/quests", items.quests)
			r.Get("/{id}/raw", raw.rowFromTable("items", "id"))
		})
		r.Route("/spells", func(r chi.Router) {
			r.Get("/", spells.search)
			r.Post("/stat-deltas", spells.statDeltas)
			r.Post("/shopping-route", spells.shoppingRoute)
			r.Get("/class/{classIndex}", spells.byClass)
			r.Get("/{id}", spells.get)
			r.Get("/{id}/items", spells.crossRefs)
			r.Get("/{id}/raw", raw.rowFromTable("spells_new", "id"))
		})
		r.Post("/resist-check", resistCalc.check)
		r.Get("/resist-debuffs", resistCalc.debuffs)
		// Charm Pet Finder: lists a zone's charmable NPCs for a charm
		// class/spell, ranked by DPS, with level-cap warnings and land odds.
		r.Route("/charm", func(r chi.Router) {
			r.Get("/spells", charmH.spells)
			r.Get("/pets", charmH.pets)
		})
		// Bazaar Trader Tracker (developer-tab feature). Routes only exist when
		// the snapshot store opened successfully (user.db available).
		if traderStore != nil {
			r.Route("/trader", func(r chi.Router) {
				r.Get("/characters", traderH.characters)
				r.Get("/{char}/listings", traderH.listings)
				r.Get("/{char}/sessions", traderH.sessions)
				r.Get("/{char}/snapshots", traderH.snapshots)
				r.Post("/{char}/capture", traderH.capture)
			})
		}
		r.Route("/npcs", func(r chi.Router) {
			r.Get("/", npcs.search)
			r.Get("/{id}", npcs.get)
			r.Get("/{id}/spawns", npcs.spawns)
			r.Get("/{id}/loot", npcs.loot)
			r.Get("/{id}/faction", npcs.faction)
			r.Get("/{id}/spells", npcs.spells)
			r.Get("/{id}/raw", raw.rowFromTable("npc_types", "id"))
		})
		r.Route("/zones", func(r chi.Router) {
			r.Get("/", zones.search)
			r.Get("/expansions", zones.expansions)
			r.Get("/short/{name}", zones.getByShortName)
			r.Get("/short/{name}/npcs", zones.getNPCsByShortName)
			r.Get("/short/{name}/connections", zones.getConnections)
			r.Get("/short/{name}/ground-spawns", zones.getGroundSpawns)
			r.Get("/short/{name}/forage", zones.getForage)
			r.Get("/short/{name}/drops", zones.getDrops)
			r.Get("/{id}", zones.get)
			r.Get("/{id}/raw", raw.rowFromTable("zone", "id"))
		})
		r.Route("/recipes", func(r chi.Router) {
			r.Get("/", recipes.search)
			r.Get("/tradeskills", recipes.tradeskills)
			r.Get("/{id}", recipes.get)
			r.Get("/{id}/raw", raw.rowFromTable("tradeskill_recipe", "id"))
		})
		r.Route("/quests", func(r chi.Router) {
			r.Get("/", quests.search)
		})
		r.Route("/favorite-recipes", func(r chi.Router) {
			r.Get("/", favRecipes.list)
			r.Post("/", favRecipes.add)
			r.Delete("/{id}", favRecipes.del)
		})
		r.Route("/config", func(r chi.Router) {
			r.Get("/", cfg.get)
			r.Put("/", cfg.update)
			r.Post("/validate-eq-path", cfg.validateEQPath)
			r.Get("/eq-diagnostics", cfg.diagnostics)
			r.Post("/set-logging", cfg.setLogging)
			r.Post("/set-export-on-camp", cfg.setExportOnCamp)
			r.Get("/server-info", cfg.serverInfo)
			r.Get("/test-port", cfg.testPort)
		})
		r.Route("/characters", func(r chi.Router) {
			r.Get("/", charactersH.list)
			r.Post("/", charactersH.create)
			r.Get("/discover", charactersH.discover)
			r.Delete("/{id}", charactersH.del)
			r.Get("/{id}/aas", charactersH.aas)
			r.Get("/{id}/tradeskills", charactersH.tradeskills)
			r.Get("/{id}/skills", skillsH.get)
			r.Get("/{id}/spell-modifiers", charactersH.spellModifiers)
			r.Get("/{id}/equipped-stats", charactersH.equippedStats)
			r.Get("/{id}/instrument-mods", charactersH.instrumentMods)
			r.Post("/{id}/derived-stats", charactersH.derivedStats)
			r.Get("/{id}/upgrades", charactersH.upgrades)
			r.Get("/{id}/upgrades/overview", charactersH.upgradesOverview)
			r.Get("/{id}/upgrade-weights", charactersH.upgradeWeights)
			r.Put("/{id}/upgrade-weights", charactersH.updateUpgradeWeights)
			r.Delete("/{id}/upgrade-weights", charactersH.resetUpgradeWeights)
			r.Get("/{id}/focus-options", charactersH.focusOptions)
			r.Get("/{id}/priority-focus", charactersH.priorityFocus)
			r.Put("/{id}/priority-focus", charactersH.updatePriorityFocus)
			r.Get("/{id}/raid-buffs", charactersH.raidBuffs)
			r.Put("/{id}/raid-buffs", charactersH.updateRaidBuffs)
			r.Get("/{id}/tasks", tasksH.list)
			r.Post("/{id}/tasks", tasksH.create)
			r.Put("/{id}/tasks/reorder", tasksH.reorder)
			r.Put("/{id}/tasks/{taskID}", tasksH.update)
			r.Delete("/{id}/tasks/{taskID}", tasksH.del)
			r.Post("/{id}/tasks/{taskID}/subtasks", tasksH.createSubtask)
			r.Put("/{id}/tasks/{taskID}/subtasks/{subtaskID}", tasksH.updateSubtask)
			r.Delete("/{id}/tasks/{taskID}/subtasks/{subtaskID}", tasksH.deleteSubtask)
			r.Get("/{id}/wishlist", wishlistH.list)
			r.Post("/{id}/wishlist", wishlistH.add)
			r.Put("/{id}/wishlist/reorder", wishlistH.reorder)
			r.Put("/{id}/wishlist/slot-layout", wishlistH.updateSlotLayout)
			r.Delete("/{id}/wishlist/{entryID}", wishlistH.del)
		})
		r.Route("/quarm", func(r chi.Router) {
			r.Get("/client-status", quarmH.clientStatus)
		})
		r.Route("/eqw", func(r chi.Router) {
			r.Get("/status", eqwH.status)
		})
		r.Route("/zeal", func(r chi.Router) {
			r.Get("/detect", zealH.detect)
			r.Get("/pipe-status", zealH.pipeStatus)
			r.Get("/inventory", zealH.inventory)
			r.Get("/spells", zealH.spellbook)
			r.Get("/all-inventories", zealH.allInventories)
			r.Get("/quarmy", zealH.quarmy)
			r.Get("/spellsets", zealH.spellsets)
			r.Put("/spellsets", zealH.updateSpellsets)
			r.Get("/spellsets/all", zealH.allSpellsets)
			r.Post("/spellsets/parse-file", zealH.parseSpellsetsFile)
			r.Get("/bandolier", zealH.bandolier)
			r.Put("/bandolier", zealH.updateBandolier)
			r.Get("/bandolier/all", zealH.allBandoliers)
			r.Get("/bandolier/slot-items", zealH.bandolierSlotItems)
			r.Get("/bandolier/bag", zealH.bandolierBag)
			r.Put("/bandolier/bag", zealH.updateBandolierBag)
			r.Post("/bandolier/parse-file", zealH.parseBandolierFile)
			r.Get("/macros", zealH.macros)
			r.Put("/macros", zealH.updateMacros)
			r.Get("/macros/all", zealH.allMacros)
			r.Post("/macros/parse-file", zealH.parseMacrosFile)
			r.Get("/text-colors", zealH.textColors)
		})
		r.Route("/keys", func(r chi.Router) {
			r.Get("/", keysH.list)
			r.Get("/progress", keysH.progress)
		})
		// Per-character keyring tracker driven by /keys log parsing. Distinct
		// from /api/keys above, which handles the older multi-component
		// quest-key progression view.
		r.Route("/keyring", func(r chi.Router) {
			r.Get("/master", keyringH.listMaster)
			r.Get("/characters", keyringH.listCharacters)
			r.Get("/characters/{name}", keyringH.getCharacter)
		})
		// Planes of Power flagging tracker (developer-tab feature). The dataset
		// route always works; per-character routes need user.db (store != nil).
		r.Route("/popflags", func(r chi.Router) {
			r.Get("/dataset", popflagH.dataset)
			r.Get("/{character}", popflagH.get)
			r.Post("/{character}/seer/preview", popflagH.seerPreview)
			r.Post("/{character}/seer/scan", popflagH.seerScan)
			r.Post("/{character}/seer/commit", popflagH.seerCommit)
			r.Post("/{character}/{flagID}", popflagH.setManual)
		})
		// Per-character loot/legacy lockout tracker driven by /sll log parsing.
		r.Route("/lockouts", func(r chi.Router) {
			r.Get("/characters", lockoutsH.listCharacters)
			r.Get("/characters/{name}", lockoutsH.getCharacter)
		})
		r.Route("/backups", func(r chi.Router) {
			r.Get("/", backupH.list)
			r.Post("/", backupH.create)
			r.Post("/prune", backupH.prune)
			r.Get("/{id}", backupH.get)
			r.Delete("/{id}", backupH.delete)
			r.Post("/{id}/restore", backupH.restore)
			r.Put("/{id}/lock", backupH.lock)
			r.Put("/{id}/unlock", backupH.unlock)
		})
		r.Route("/players", func(r chi.Router) {
			r.Get("/", playersH.list)
			r.Post("/clear", playersH.clear)
			r.Get("/{name}", playersH.get)
			r.Get("/{name}/history", playersH.history)
			r.Put("/{name}/note", playersH.upsertNote)
			r.Put("/{name}/manual", playersH.upsertManual)
			r.Delete("/{name}", playersH.delete)
		})
		r.Route("/chat", func(r chi.Router) {
			r.Get("/channels", chatH.channels)
			r.Get("/conversations", chatH.conversations)
			r.Get("/feed", chatH.feed)
			r.Get("/thread/{peer}", chatH.thread)
			r.Post("/clear", chatH.clear)
			r.Delete("/peer/{peer}", chatH.deletePeer)
		})
		r.Route("/loot", func(r chi.Router) {
			r.Get("/", lootH.list)
			r.Get("/meta", lootH.meta)
			r.Post("/clear", lootH.clear)
		})
		r.Route("/backfill", func(r chi.Router) {
			r.Get("/", backfillH.info)
			r.Post("/", backfillH.run)
		})
		r.Route("/app", func(r chi.Router) {
			r.Post("/export", appBackupH.export)
			r.Post("/import/preview", appBackupH.importPreview)
			r.Post("/import", appBackupH.stageImport)
			r.Get("/import/pending", appBackupH.pendingStatus)
			r.Delete("/import", appBackupH.cancelImport)
			r.Post("/reset", appBackupH.stageReset)
			r.Get("/reset/pending", appBackupH.resetPending)
			r.Delete("/reset", appBackupH.cancelReset)
		})
		r.Route("/log", func(r chi.Router) {
			r.Get("/status", logH.status)
			r.Get("/info", logH.info)
			r.Get("/browse", logH.browse)
			r.Post("/cleanup", logH.cleanup)
			r.Post("/raw-feed", logH.rawFeed)
		})
		r.Route("/replay", func(r chi.Router) {
			r.Get("/files", replayH.files)
			r.Get("/info", replayH.info)
			r.Get("/status", replayH.status)
			r.Post("/start", replayH.start)
			r.Post("/pause", replayH.pause)
			r.Post("/resume", replayH.resume)
			r.Post("/stop", replayH.stop)
		})
		r.Route("/overlay", func(r chi.Router) {
			r.Get("/npc/target", overlayH.npcTarget)
			r.Get("/combat", combatH.state)
			r.Get("/threat", threatH.state)
			r.Delete("/threat/{name}", threatH.removeMob)
			r.Get("/raidthreat", raidThreatH.state)
			r.Delete("/raidthreat", raidThreatH.dismissAll)
			r.Delete("/raidthreat/{name}", raidThreatH.dismissMob)
			r.Get("/timers", timerH.state)
			r.Post("/timers/clear", timerH.clear)
			r.Post("/timers/custom", timerH.startCustom)
			r.Delete("/timers/{id}", timerH.remove)
			r.Get("/respawns", respawnH.state)
			r.Delete("/respawns", respawnH.clear)
			r.Delete("/respawns/{id}", respawnH.remove)
		})
		r.Post("/threat/reset", threatH.reset)
		r.Route("/combat", func(r chi.Router) {
			r.Post("/reset", combatH.reset)
			r.Post("/end", combatH.end)
			r.Route("/history", func(r chi.Router) {
				r.Get("/", combatH.historyList)
				r.Delete("/", combatH.historyClear)
				r.Get("/facets", combatH.historyFacets)
				r.Get("/{id}", combatH.historyGet)
				r.Delete("/{id}", combatH.historyDelete)
			})
		})
		r.Route("/rolls", func(r chi.Router) {
			r.Get("/", rollsH.state)
			r.Delete("/", rollsH.clear)
			r.Put("/settings", rollsH.updateSettings)
			r.Route("/sessions/{id}", func(r chi.Router) {
				r.Post("/stop", rollsH.stop)
				r.Delete("/", rollsH.remove)
				r.Put("/item-name", rollsH.setItemName)
			})
		})
		r.Route("/sandbox", func(r chi.Router) {
			r.Get("/schema", sandboxH.schema)
			r.Post("/query", sandboxH.query)
			r.Route("/saved", func(r chi.Router) {
				r.Get("/", savedQueryH.list)
				r.Post("/", savedQueryH.create)
				r.Get("/export", savedQueryH.exportPack)
				r.Post("/import", savedQueryH.importPack)
				r.Put("/{id}", savedQueryH.update)
				r.Delete("/{id}", savedQueryH.delete)
			})
		})
		r.Route("/triggers", func(r chi.Router) {
			r.Get("/", triggerH.list)
			r.Post("/", triggerH.create)
			r.Delete("/", triggerH.clearAll)
			r.Put("/{id}", triggerH.update)
			r.Delete("/{id}", triggerH.del)
			r.Get("/history", triggerH.history)
			r.Post("/import", triggerH.importPack)
			r.Post("/import/preview", triggerH.importPreview)
			r.Post("/import/commit", triggerH.importCommit)
			r.Get("/export", triggerH.exportPack)
			r.Get("/action-templates", triggerH.listActionTemplates)
			r.Post("/action-templates", triggerH.createActionTemplate)
			r.Put("/action-templates/{id}", triggerH.updateActionTemplate)
			r.Delete("/action-templates/{id}", triggerH.deleteActionTemplate)
			r.Post("/bulk-actions", triggerH.bulkEditActions)
			r.Get("/packs", triggerH.listBuiltinPacks)
			// Static /packs/updates is registered before the {name} wildcard
			// so a pack can never shadow it.
			r.Get("/packs/updates", triggerH.listPackUpdates)
			r.Get("/packs/{name}/diff", triggerH.packDiff)
			r.Post("/packs/{name}/update", triggerH.applyPackUpdate)
			r.Post("/packs/{name}", triggerH.installBuiltinPack)
			r.Delete("/packs/{name}", triggerH.removePack)
			r.Get("/categories", triggerH.listCategories)
			r.Post("/categories", triggerH.createCategory)
			// Reorder routes use POST on a static sub-path so a category named
			// "order" can't shadow them (vs. the PUT/{name} rename route).
			r.Post("/categories/order", triggerH.reorderCategories)
			r.Post("/order", triggerH.reorderTriggers)
			r.Put("/categories/{name}", triggerH.renameCategory)
			r.Delete("/categories/{name}", triggerH.deleteCategory)
			r.Post("/test-overlay", triggerH.testOverlay)
			r.Get("/test-overlay/active", triggerH.testOverlayActive)
			r.Post("/test-overlay/position", triggerH.testOverlayPosition)
			r.Post("/test-overlay/end", triggerH.testOverlayEnd)
		})
	})

	// WebSocket endpoint — no Content-Type middleware (upgrade handles headers).
	r.Get("/ws", ws.Handler(hub))

	return r
}
