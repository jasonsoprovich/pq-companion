// Package api provides the HTTP REST API for the PQ Companion backend.
package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
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

// NewRouter builds and returns the chi router wired to all backend components.
func NewRouter(database *db.DB, hub *ws.Hub, cfgMgr *config.Manager, zealWatcher *zeal.Watcher, backupMgr *backup.Manager, tailer *logparser.Tailer, npcTracker *overlay.NPCTracker, combatTracker *combat.Tracker, timerEngine *spelltimer.Engine, triggerStore *trigger.Store, triggerEngine *trigger.Engine, charStore *character.Store) http.Handler {
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
	spells := &spellsHandler{db: database}
	npcs := &npcsHandler{db: database}
	zones := &zonesHandler{db: database}
	cfg := &configHandler{mgr: cfgMgr, hub: hub}
	charactersH := &charactersHandler{store: charStore, mgr: cfgMgr, db: database, tailer: tailer}
	search := &searchHandler{db: database}
	zealH := &zealHandler{watcher: zealWatcher}
	keysH := &keysHandler{watcher: zealWatcher}
	backupH := &backupHandler{mgr: backupMgr}
	logH := &logHandler{tailer: tailer}
	overlayH := &overlayHandler{npcTracker: npcTracker}
	combatH := &combatHandler{tracker: combatTracker}
	timerH := &timerHandler{engine: timerEngine}
	triggerH := &triggerHandler{store: triggerStore, engine: triggerEngine}
	tasksH := &tasksHandler{store: charStore}

	r.Route("/api", func(r chi.Router) {
		r.Use(middleware.SetHeader("Content-Type", "application/json"))
		r.Get("/search", search.global)
		r.Route("/items", func(r chi.Router) {
			r.Get("/", items.search)
			r.Get("/{id}", items.get)
			r.Get("/{id}/sources", items.sources)
		})
		r.Route("/spells", func(r chi.Router) {
			r.Get("/", spells.search)
			r.Get("/class/{classIndex}", spells.byClass)
			r.Get("/{id}", spells.get)
			r.Get("/{id}/items", spells.crossRefs)
		})
		r.Route("/npcs", func(r chi.Router) {
			r.Get("/", npcs.search)
			r.Get("/{id}", npcs.get)
			r.Get("/{id}/spawns", npcs.spawns)
			r.Get("/{id}/loot", npcs.loot)
			r.Get("/{id}/faction", npcs.faction)
		})
		r.Route("/zones", func(r chi.Router) {
			r.Get("/", zones.search)
			r.Get("/short/{name}", zones.getByShortName)
			r.Get("/short/{name}/npcs", zones.getNPCsByShortName)
			r.Get("/short/{name}/connections", zones.getConnections)
			r.Get("/short/{name}/ground-spawns", zones.getGroundSpawns)
			r.Get("/short/{name}/forage", zones.getForage)
			r.Get("/short/{name}/drops", zones.getDrops)
			r.Get("/{id}", zones.get)
		})
		r.Route("/config", func(r chi.Router) {
			r.Get("/", cfg.get)
			r.Put("/", cfg.update)
			r.Post("/validate-eq-path", cfg.validateEQPath)
		})
		r.Route("/characters", func(r chi.Router) {
			r.Get("/", charactersH.list)
			r.Post("/", charactersH.create)
			r.Get("/discover", charactersH.discover)
			r.Delete("/{id}", charactersH.del)
			r.Get("/{id}/aas", charactersH.aas)
			r.Get("/{id}/spell-modifiers", charactersH.spellModifiers)
			r.Get("/{id}/tasks", tasksH.list)
			r.Post("/{id}/tasks", tasksH.create)
			r.Put("/{id}/tasks/reorder", tasksH.reorder)
			r.Put("/{id}/tasks/{taskID}", tasksH.update)
			r.Delete("/{id}/tasks/{taskID}", tasksH.del)
			r.Post("/{id}/tasks/{taskID}/subtasks", tasksH.createSubtask)
			r.Put("/{id}/tasks/{taskID}/subtasks/{subtaskID}", tasksH.updateSubtask)
			r.Delete("/{id}/tasks/{taskID}/subtasks/{subtaskID}", tasksH.deleteSubtask)
		})
		r.Route("/zeal", func(r chi.Router) {
			r.Get("/inventory", zealH.inventory)
			r.Get("/spells", zealH.spellbook)
			r.Get("/all-inventories", zealH.allInventories)
			r.Get("/quarmy", zealH.quarmy)
		})
		r.Route("/keys", func(r chi.Router) {
			r.Get("/", keysH.list)
			r.Get("/progress", keysH.progress)
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
		r.Route("/log", func(r chi.Router) {
			r.Get("/status", logH.status)
			r.Get("/info", logH.info)
			r.Post("/cleanup", logH.cleanup)
		})
		r.Route("/overlay", func(r chi.Router) {
			r.Get("/npc/target", overlayH.npcTarget)
			r.Get("/combat", combatH.state)
			r.Get("/timers", timerH.state)
			r.Post("/timers/clear", timerH.clear)
		})
		r.Route("/combat", func(r chi.Router) {
			r.Post("/reset", combatH.reset)
		})
		r.Route("/triggers", func(r chi.Router) {
			r.Get("/", triggerH.list)
			r.Post("/", triggerH.create)
			r.Put("/{id}", triggerH.update)
			r.Delete("/{id}", triggerH.del)
			r.Get("/history", triggerH.history)
			r.Post("/import", triggerH.importPack)
			r.Post("/import-gina", triggerH.importGINA)
			r.Get("/export", triggerH.exportPack)
			r.Get("/packs", triggerH.listBuiltinPacks)
			r.Post("/packs/{name}", triggerH.installBuiltinPack)
		})
	})

	// WebSocket endpoint — no Content-Type middleware (upgrade handles headers).
	r.Get("/ws", ws.Handler(hub))

	return r
}
