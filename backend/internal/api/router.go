// Package api provides the HTTP REST API for the PQ Companion backend.
package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// NewRouter builds and returns the chi router wired to the given DB, Hub, and Config.
func NewRouter(database *db.DB, hub *ws.Hub, cfgMgr *config.Manager) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	// Allow requests from the Vite dev server and any local renderer.
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, PUT, OPTIONS")
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

	r.Route("/api", func(r chi.Router) {
		r.Use(middleware.SetHeader("Content-Type", "application/json"))
		r.Route("/items", func(r chi.Router) {
			r.Get("/", items.search)
			r.Get("/{id}", items.get)
		})
		r.Route("/spells", func(r chi.Router) {
			r.Get("/", spells.search)
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
	})

	// WebSocket endpoint — no Content-Type middleware (upgrade handles headers).
	r.Get("/ws", ws.Handler(hub))

	return r
}
