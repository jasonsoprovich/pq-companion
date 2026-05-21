package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jasonsoprovich/pq-companion/backend/internal/character"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
)

// wishlistHandler handles per-character wishlist endpoints.
type wishlistHandler struct {
	store *character.Store
	db    *db.DB
}

// validSlotBuckets is the closed set of bucket names the API accepts. Mirrors
// the EQ canonical worn-slot order plus a "General" bucket for non-equippable
// items. Matches frontend/src/lib/wishlistSlots.ts — keep in sync.
var validSlotBuckets = map[string]bool{
	"Charm": true, "Ear": true, "Head": true, "Face": true, "Neck": true,
	"Shoulder": true, "Arms": true, "Back": true, "Wrist": true, "Range": true,
	"Hands": true, "Primary": true, "Secondary": true, "Finger": true,
	"Chest": true, "Legs": true, "Feet": true, "Waist": true, "Ammo": true,
	"General": true,
}

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
	Entries []wishlistRow `json:"entries"`
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
	writeJSON(w, http.StatusOK, wishlistListResponse{Entries: rows})
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
	// Sanity-check that the item exists before creating wishlist rows for it.
	if _, err := h.db.GetItem(req.ItemID); err != nil {
		writeError(w, http.StatusBadRequest, "item not found")
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
	w.WriteHeader(http.StatusNoContent)
}

type wishlistReorderRequest struct {
	SlotBucket string `json:"slot_bucket"`
	OrderedIDs []int  `json:"ordered_ids"`
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
	if !validSlotBuckets[req.SlotBucket] {
		writeError(w, http.StatusBadRequest, "invalid slot bucket")
		return
	}
	if err := h.store.ReorderWishlistSlot(charID, req.SlotBucket, req.OrderedIDs); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
