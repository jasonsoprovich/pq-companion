// Package api provides the HTTP REST API for the PQ Companion backend.
package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
)

// NewRouter builds and returns the chi router wired to the given DB.
func NewRouter(database *db.DB) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.SetHeader("Content-Type", "application/json"))

	items := &itemsHandler{db: database}
	spells := &spellsHandler{db: database}
	npcs := &npcsHandler{db: database}
	zones := &zonesHandler{db: database}

	r.Route("/api", func(r chi.Router) {
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
			r.Get("/{id}", zones.get)
		})
	})

	return r
}
