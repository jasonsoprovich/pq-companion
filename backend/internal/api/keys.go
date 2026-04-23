package api

import (
	"encoding/json"
	"net/http"

	"github.com/jasonsoprovich/pq-companion/backend/internal/keys"
	"github.com/jasonsoprovich/pq-companion/backend/internal/zeal"
)

type keysHandler struct {
	watcher *zeal.Watcher
}

// GET /api/keys
// Returns all key definitions (name, description, components).
func (h *keysHandler) list(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(struct {
		Keys []keys.KeyDef `json:"keys"`
	}{Keys: keys.All()})
}

// characterProgress tracks which component item IDs a single character holds.
type characterProgress struct {
	Character string         `json:"character"`
	HasExport bool           `json:"has_export"`
	HaveIDs   map[int]bool   `json:"-"` // internal lookup set
}

// keyProgress is the per-key section of the progress response.
type keyProgress struct {
	KeyID      string                      `json:"key_id"`
	Characters []characterKeyProgress      `json:"characters"`
}

// characterKeyProgress is the per-character view of one key.
// FinalItem is non-nil only when the KeyDef defines a FinalItem; if Have or
// SharedBank is true, the character is considered fully keyed regardless of
// component progress.
// IntermediateItem is non-nil when the KeyDef defines an IntermediateItem; if
// Have or SharedBank is true, the first IntermediateCoverCount components are
// considered complete (the intermediate combine consumed them).
type characterKeyProgress struct {
	Character      string            `json:"character"`
	HasExport      bool              `json:"has_export"`
	Components     []componentStatus `json:"components"`
	FinalItem      *componentStatus  `json:"final_item,omitempty"`
	IntermediateItem *componentStatus `json:"intermediate_item,omitempty"`
}

// componentStatus is the status of one component for one character.
type componentStatus struct {
	ItemID      int    `json:"item_id"`
	ItemName    string `json:"item_name"`
	Have        bool   `json:"have"`        // present in this character's inventory
	SharedBank  bool   `json:"shared_bank"` // present in shared bank (counts for all)
}

// progressResponse is the full GET /api/keys/progress response.
type progressResponse struct {
	Configured bool          `json:"configured"`
	Keys       []keyProgress `json:"keys"`
}

// GET /api/keys/progress
// Returns per-key, per-character component status cross-referenced with Zeal inventory exports.
func (h *keysHandler) progress(w http.ResponseWriter, r *http.Request) {
	allInv, err := h.watcher.AllInventories()
	if err != nil {
		http.Error(w, `{"error":"failed to scan inventories"}`, http.StatusInternalServerError)
		return
	}

	resp := progressResponse{
		Configured: allInv.Configured,
		Keys:       []keyProgress{},
	}

	if !allInv.Configured || len(allInv.Characters) == 0 {
		// Still encode keys with empty character lists so the frontend can render.
		for _, kd := range keys.All() {
			resp.Keys = append(resp.Keys, keyProgress{KeyID: kd.ID, Characters: []characterKeyProgress{}})
		}
		json.NewEncoder(w).Encode(resp)
		return
	}

	// Build shared-bank item ID set.
	sharedIDs := make(map[int]bool, len(allInv.SharedBank))
	for _, e := range allInv.SharedBank {
		sharedIDs[e.ID] = true
	}

	// Build per-character item ID sets.
	chars := make([]characterProgress, 0, len(allInv.Characters))
	for _, inv := range allInv.Characters {
		cp := characterProgress{
			Character: inv.Character,
			HasExport: true,
			HaveIDs:   make(map[int]bool, len(inv.Entries)),
		}
		for _, e := range inv.Entries {
			cp.HaveIDs[e.ID] = true
		}
		chars = append(chars, cp)
	}

	for _, kd := range keys.All() {
		kp := keyProgress{
			KeyID:      kd.ID,
			Characters: make([]characterKeyProgress, 0, len(chars)),
		}

		for _, cp := range chars {
			ckp := characterKeyProgress{
				Character:  cp.Character,
				HasExport:  cp.HasExport,
				Components: make([]componentStatus, 0, len(kd.Components)),
			}
			for _, comp := range kd.Components {
				cs := componentStatus{
					ItemID:   comp.ItemID,
					ItemName: comp.ItemName,
					Have:     cp.HaveIDs[comp.ItemID],
					// SharedBank is true if the item is in shared bank AND this char doesn't
					// already have their own copy. The frontend shows the SharedBank badge
					// when the only source is the shared bank.
					SharedBank: !cp.HaveIDs[comp.ItemID] && sharedIDs[comp.ItemID],
				}
				ckp.Components = append(ckp.Components, cs)
			}
			if kd.IntermediateItem != nil {
				ii := *kd.IntermediateItem
				ckp.IntermediateItem = &componentStatus{
					ItemID:     ii.ItemID,
					ItemName:   ii.ItemName,
					Have:       cp.HaveIDs[ii.ItemID],
					SharedBank: !cp.HaveIDs[ii.ItemID] && sharedIDs[ii.ItemID],
				}
			}
			if kd.FinalItem != nil {
				fi := *kd.FinalItem
				ckp.FinalItem = &componentStatus{
					ItemID:     fi.ItemID,
					ItemName:   fi.ItemName,
					Have:       cp.HaveIDs[fi.ItemID],
					SharedBank: !cp.HaveIDs[fi.ItemID] && sharedIDs[fi.ItemID],
				}
			}
			kp.Characters = append(kp.Characters, ckp)
		}

		resp.Keys = append(resp.Keys, kp)
	}

	json.NewEncoder(w).Encode(resp)
}
