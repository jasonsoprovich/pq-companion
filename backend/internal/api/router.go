// Package api provides the HTTP REST API for the PQ Companion backend.
package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jasonsoprovich/pq-companion/backend/internal/backup"
	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
	"github.com/jasonsoprovich/pq-companion/backend/internal/zeal"
)

// NewRouter builds and returns the chi router wired to the given DB, Hub, Config, Zeal watcher, and Backup manager.
func NewRouter(database *db.DB, hub *ws.Hub, cfgMgr *config.Manager, zealWatcher *zeal.Watcher, backupMgr *backup.Manager) http.Handler {
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
	cfg := &configHandler{mgr: cfgMgr}
	search := &searchHandler{db: database}
	zealH := &zealHandler{watcher: zealWatcher}
	keysH := &keysHandler{watcher: zealWatcher}
	backupH := &backupHandler{mgr: backupMgr}

	r.Route("/api", func(r chi.Router) {
		r.Use(middleware.SetHeader("Content-Type", "application/json"))
		r.Get("/search", search.global)
		r.Route("/items", func(r chi.Router) {
			r.Get("/", items.search)
			r.Get("/{id}", items.get)
		})
		r.Route("/spells", func(r chi.Router) {
			r.Get("/", spells.search)
			r.Get("/class/{classIndex}", spells.byClass)
			r.Get("/{id}", spells.get)
		})
		r.Route("/npcs", func(r chi.Router) {
			r.Get("/", npcs.search)
			r.Get("/{id}", npcs.get)
		})
		r.Route("/zones", func(r chi.Router) {
			r.Get("/", zones.search)
			r.Get("/short/{name}", zones.getByShortName)
			r.Get("/short/{name}/npcs", zones.getNPCsByShortName)
			r.Get("/{id}", zones.get)
		})
		r.Route("/config", func(r chi.Router) {
			r.Get("/", cfg.get)
			r.Put("/", cfg.update)
		})
		r.Route("/zeal", func(r chi.Router) {
			r.Get("/inventory", zealH.inventory)
			r.Get("/spells", zealH.spellbook)
			r.Get("/all-inventories", zealH.allInventories)
		})
		r.Route("/keys", func(r chi.Router) {
			r.Get("/", keysH.list)
			r.Get("/progress", keysH.progress)
		})
		r.Route("/backups", func(r chi.Router) {
			r.Get("/", backupH.list)
			r.Post("/", backupH.create)
			r.Get("/{id}", backupH.get)
			r.Delete("/{id}", backupH.delete)
			r.Post("/{id}/restore", backupH.restore)
		})
	})

	// WebSocket endpoint — no Content-Type middleware (upgrade handles headers).
	r.Get("/ws", ws.Handler(hub))

	return r
}
