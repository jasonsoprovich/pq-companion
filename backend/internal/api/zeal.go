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

// GET /api/zeal/all-inventories
// Scans the configured EQ directory for all Zeal inventory exports and returns
// per-character inventories plus a deduplicated SharedBank.
func (h *zealHandler) allInventories(w http.ResponseWriter, r *http.Request) {
	resp, err := h.watcher.AllInventories()
	if err != nil {
		http.Error(w, `{"error":"failed to scan inventories"}`, http.StatusInternalServerError)
		return
	}
	json.NewEncoder(w).Encode(resp)
}

// GET /api/zeal/quarmy
// Returns the most recently parsed quarmy export (stats, inventory, AAs).
// Returns {"quarmy": null} if no quarmy file has been found yet.
func (h *zealHandler) quarmy(w http.ResponseWriter, r *http.Request) {
	q := h.watcher.Quarmy()
	json.NewEncoder(w).Encode(struct {
		Quarmy interface{} `json:"quarmy"`
	}{Quarmy: q})
}
