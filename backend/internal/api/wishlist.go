package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jasonsoprovich/pq-companion/backend/internal/character"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/wishlistwatch"
	"github.com/jasonsoprovich/pq-companion/backend/internal/ws"
)

// wishlistHandler handles per-character wishlist endpoints.
type wishlistHandler struct {
	store   *character.Store
	db      *db.DB
	hub     *ws.Hub
	watcher *wishlistwatch.Watcher // may be nil (e.g. in tests)
}

// broadcastChanged notifies listeners (e.g. the NPC loot overlay, which
// highlights wishlisted drops) that the character's wishlist membership
// changed and any cached item-id set should be refetched. Also rebuilds the
// wishlist watcher's item-name match set. Fired only on add/remove —
// reordering doesn't change membership.
func (h *wishlistHandler) broadcastChanged(charID int) {
	if h.hub != nil {
		h.hub.Broadcast(ws.Event{
			Type: "wishlist:changed",
			Data: map[string]int{"character_id": charID},
		})
	}
	if h.watcher != nil {
		h.watcher.Rebuild()
	}
}

// validSlotBuckets is the closed set of bucket names the API accepts. Derived
// from character.CanonicalWishlistSlotOrder — single source of truth.
var validSlotBuckets = func() map[string]bool {
	m := make(map[string]bool, len(character.CanonicalWishlistSlotOrder))
	for _, b := range character.CanonicalWishlistSlotOrder {
		m[b] = true
	}
	return m
}()

// wishlistItemBrief is the lightweight item slice bundled with each entry —
// just enough to render a row without the frontend fetching the full Item.
type wishlistItemBrief struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Icon      int    `json:"icon"`
	Slots     int    `json:"slots"`
	ItemClass int    `json:"item_class"`
	ItemType  int    `json:"item_type"`
}

type wishlistRow struct {
	ID         int                `json:"id"`
	ItemID     int                `json:"item_id"`
	SlotBucket string             `json:"slot_bucket"`
	SortOrder  int                `json:"sort_order"`
	CreatedAt  int64              `json:"created_at"`
	Item       *wishlistItemBrief `json:"item,omitempty"`
}

type wishlistListResponse struct {
	Entries    []wishlistRow                  `json:"entries"`
	SlotLayout []character.WishlistSlotLayout `json:"slot_layout"`
}

func (h *wishlistHandler) list(w http.ResponseWriter, r *http.Request) {
	charID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid character id")
		return
	}
	if _, ok, err := h.store.Get(charID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	} else if !ok {
		writeError(w, http.StatusNotFound, "character not found")
		return
	}

	entries, err := h.store.ListWishlist(charID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	layout, err := h.store.ListWishlistSlotLayout(charID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	rows := make([]wishlistRow, 0, len(entries))
	for _, e := range entries {
		row := wishlistRow{
			ID:         e.ID,
			ItemID:     e.ItemID,
			SlotBucket: e.SlotBucket,
			SortOrder:  e.SortOrder,
			CreatedAt:  e.CreatedAt,
		}
		if it, err := h.db.GetItem(e.ItemID); err == nil && it != nil {
			row.Item = &wishlistItemBrief{
				ID:        it.ID,
				Name:      it.Name,
				Icon:      it.Icon,
				Slots:     it.Slots,
				ItemClass: it.ItemClass,
				ItemType:  it.ItemType,
			}
		}
		rows = append(rows, row)
	}
	writeJSON(w, http.StatusOK, wishlistListResponse{Entries: rows, SlotLayout: layout})
}

type wishlistAddRequest struct {
	ItemID int      `json:"item_id"`
	Slots  []string `json:"slots"`
}

func (h *wishlistHandler) add(w http.ResponseWriter, r *http.Request) {
	charID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid character id")
		return
	}
	if _, ok, err := h.store.Get(charID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	} else if !ok {
		writeError(w, http.StatusNotFound, "character not found")
		return
	}
	var req wishlistAddRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ItemID <= 0 {
		writeError(w, http.StatusBadRequest, "item_id is required")
		return
	}
	if len(req.Slots) == 0 {
		writeError(w, http.StatusBadRequest, "slots is required")
		return
	}
	if _, err := h.db.GetItem(req.ItemID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeError(w, http.StatusBadRequest, "item not found")
		} else {
			writeError(w, http.StatusInternalServerError, "failed to look up item")
		}
		return
	}
	for _, slot := range req.Slots {
		if !validSlotBuckets[slot] {
			writeError(w, http.StatusBadRequest, "invalid slot bucket: "+slot)
			return
		}
	}
	created := make([]character.WishlistEntry, 0, len(req.Slots))
	for _, slot := range req.Slots {
		entry, err := h.store.AddWishlistEntry(charID, req.ItemID, slot)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		created = append(created, entry)
	}
	h.broadcastChanged(charID)
	writeJSON(w, http.StatusCreated, map[string]any{"entries": created})
}

func (h *wishlistHandler) del(w http.ResponseWriter, r *http.Request) {
	charID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid character id")
		return
	}
	entryID, err := strconv.Atoi(chi.URLParam(r, "entryID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid entry id")
		return
	}
	if err := h.store.DeleteWishlistEntry(charID, entryID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.broadcastChanged(charID)
	w.WriteHeader(http.StatusNoContent)
}

type wishlistReorderRequest struct {
	OrderedIDs []int `json:"ordered_ids"`
}

func (h *wishlistHandler) reorder(w http.ResponseWriter, r *http.Request) {
	charID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid character id")
		return
	}
	var req wishlistReorderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := h.store.ReorderWishlist(charID, req.OrderedIDs); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type wishlistSlotLayoutRequest struct {
	Layout []character.WishlistSlotLayout `json:"layout"`
}

func (h *wishlistHandler) updateSlotLayout(w http.ResponseWriter, r *http.Request) {
	charID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid character id")
		return
	}
	if _, ok, err := h.store.Get(charID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	} else if !ok {
		writeError(w, http.StatusNotFound, "character not found")
		return
	}
	var req wishlistSlotLayoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	seen := make(map[string]bool, len(req.Layout))
	for _, l := range req.Layout {
		if !validSlotBuckets[l.SlotBucket] {
			writeError(w, http.StatusBadRequest, "invalid slot bucket: "+l.SlotBucket)
			return
		}
		if seen[l.SlotBucket] {
			writeError(w, http.StatusBadRequest, "duplicate slot bucket in layout: "+l.SlotBucket)
			return
		}
		seen[l.SlotBucket] = true
	}
	if err := h.store.ReplaceWishlistSlotLayout(charID, req.Layout); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
