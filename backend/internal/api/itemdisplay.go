package api

import (
	"encoding/json"
	"net/http"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/itemdisplay"
	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

type itemDisplayHandler struct {
	tracker *itemdisplay.Tracker
	db      *db.DB
	tailer  *logparser.Tailer
}

// last handles GET /api/item-display/last — the item most recently opened in
// game, so a freshly opened Item Info overlay has something to show. Responds
// 204 before the first event.
func (h *itemDisplayHandler) last(w http.ResponseWriter, r *http.Request) {
	s := h.tracker.Last()
	if s == nil {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	writeJSON(w, http.StatusOK, s)
}

// simulate handles POST /api/item-display/simulate {"item_id": N} — a
// developer tool that fakes the Zeal item event, so the overlay can be built
// and tested without a Zeal build that sends it. Doesn't mark Zeal as
// supporting the message.
func (h *itemDisplayHandler) simulate(w http.ResponseWriter, r *http.Request) {
	var body struct {
		ItemID int `json:"item_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.ItemID <= 0 {
		writeError(w, http.StatusBadRequest, "item_id required")
		return
	}
	item, err := h.db.GetItem(body.ItemID)
	if err != nil || item == nil {
		writeError(w, http.StatusNotFound, "item not found")
		return
	}
	character := ""
	if h.tailer != nil {
		character = h.tailer.ActiveCharacter()
	}
	writeJSON(w, http.StatusOK, h.tracker.Simulate(item.ID, item.Name, character))
}
