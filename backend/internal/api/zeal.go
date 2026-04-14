package api

import (
	"encoding/json"
	"net/http"

	"github.com/jasonsoprovich/pq-companion/backend/internal/zeal"
)

type zealHandler struct {
	watcher *zeal.Watcher
}

// GET /api/zeal/inventory
// Returns the most recently parsed Zeal inventory export.
// If no export file has been found yet (character or eq_path not configured,
// or file not yet written) returns {"inventory": null}.
func (h *zealHandler) inventory(w http.ResponseWriter, r *http.Request) {
	inv := h.watcher.Inventory()
	json.NewEncoder(w).Encode(struct {
		Inventory *zeal.Inventory `json:"inventory"`
	}{Inventory: inv})
}

// GET /api/zeal/spells
// Returns the most recently parsed Zeal spellbook export.
// If no export file has been found yet returns {"spellbook": null}.
func (h *zealHandler) spellbook(w http.ResponseWriter, r *http.Request) {
	sb := h.watcher.Spellbook()
	json.NewEncoder(w).Encode(struct {
		Spellbook *zeal.Spellbook `json:"spellbook"`
	}{Spellbook: sb})
}
