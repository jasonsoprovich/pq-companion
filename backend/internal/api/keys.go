package api

import (
	"encoding/json"
	"net/http"

	"github.com/jasonsoprovich/pq-companion/backend/internal/keyring"
	"github.com/jasonsoprovich/pq-companion/backend/internal/keys"
	"github.com/jasonsoprovich/pq-companion/backend/internal/zeal"
)

type keysHandler struct {
	watcher *zeal.Watcher
	// keyring is the per-character /keys key-ring store. Assembled keys are
	// keyring items: once earned they're consumed out of inventory onto the
	// key ring, so they never appear in the Zeal inventory export. Without
	// this cross-reference a fully-keyed character reads as "not assembled".
	keyring *keyring.Store
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
	Character  string           `json:"character"`
	HasExport  bool             `json:"has_export"`
	HaveIDs    map[int]bool     `json:"-"` // internal lookup set
	HaveLocs   map[int][]string `json:"-"` // item ID → raw Zeal locations (bag/bank slots)
	KeyRingIDs map[int]bool     `json:"-"` // key_items on this character's /keys key ring
}

// keyProgress is the per-key section of the progress response.
type keyProgress struct {
	KeyID      string                 `json:"key_id"`
	Characters []characterKeyProgress `json:"characters"`
}

// characterKeyProgress is the per-character view of one key.
// FinalItem is non-nil only when the KeyDef defines a FinalItem; if Have or
// SharedBank is true, the character is considered fully keyed regardless of
// component progress.
// IntermediateItem is non-nil when the KeyDef defines an IntermediateItem; if
// Have or SharedBank is true, the first IntermediateCoverCount components are
// considered complete (the intermediate combine consumed them).
type characterKeyProgress struct {
	Character        string            `json:"character"`
	HasExport        bool              `json:"has_export"`
	Components       []componentStatus `json:"components"`
	FinalItem        *componentStatus  `json:"final_item,omitempty"`
	IntermediateItem *componentStatus  `json:"intermediate_item,omitempty"`
}

// componentStatus is the status of one component for one character.
type componentStatus struct {
	ItemID     int      `json:"item_id"`
	ItemName   string   `json:"item_name"`
	Have       bool     `json:"have"`                  // present in this character's inventory
	SharedBank bool     `json:"shared_bank"`           // present in shared bank (counts for all)
	OnKeyRing  bool     `json:"on_key_ring,omitempty"` // on the /keys key ring (consumed from inventory)
	Locations  []string `json:"locations,omitempty"`   // raw Zeal locations of the held item (bag/bank slots)
}

// progressResponse is the full GET /api/keys/progress response.
type progressResponse struct {
	Configured bool          `json:"configured"`
	Keys       []keyProgress `json:"keys"`
}

// holdsComponent returns true when the id-set contains the component's
// canonical ItemID or any of its AltItemIDs.
func holdsComponent(ids map[int]bool, comp keys.Component) bool {
	if ids[comp.ItemID] {
		return true
	}
	for _, alt := range comp.AltItemIDs {
		if ids[alt] {
			return true
		}
	}
	return false
}

// componentLocations gathers every Zeal location at which the component's
// canonical ItemID or any AltItemID is held. Same-name medallions live at
// distinct item IDs, so each component row resolves to its own slot(s); the
// list also covers genuine duplicate stacks of the same ID.
func componentLocations(locs map[int][]string, comp keys.Component) []string {
	out := append([]string(nil), locs[comp.ItemID]...)
	for _, alt := range comp.AltItemIDs {
		out = append(out, locs[alt]...)
	}
	return out
}

// keyRingIDs returns the set of key_item IDs on the given character's /keys
// key ring, as observed from parsed log output. Returns an empty (non-nil)
// set when the store is unavailable or the lookup fails, so callers can index
// it unconditionally. Assembled keys never appear in the inventory export —
// they're consumed onto the key ring — so this is how the tracker learns a
// character is fully keyed.
func (h *keysHandler) keyRingIDs(character string) map[int]bool {
	ids := map[int]bool{}
	if h.keyring == nil {
		return ids
	}
	entries, err := h.keyring.ListByCharacter(character)
	if err != nil {
		return ids
	}
	for _, e := range entries {
		ids[e.KeyItem] = true
	}
	return ids
}

// GET /api/keys/progress
// Returns per-key, per-character component status cross-referenced with Zeal inventory exports.
func (h *keysHandler) progress(w http.ResponseWriter, r *http.Request) {
	allInv, err := h.watcher.AllInventories()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to scan inventories")
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

	// Build shared-bank item ID set and per-ID location list.
	sharedIDs := make(map[int]bool, len(allInv.SharedBank))
	sharedLocs := make(map[int][]string, len(allInv.SharedBank))
	for _, e := range allInv.SharedBank {
		sharedIDs[e.ID] = true
		sharedLocs[e.ID] = append(sharedLocs[e.ID], e.Location)
	}

	// Build per-character item ID sets and per-ID location lists.
	chars := make([]characterProgress, 0, len(allInv.Characters))
	for _, inv := range allInv.Characters {
		cp := characterProgress{
			Character:  inv.Character,
			HasExport:  true,
			HaveIDs:    make(map[int]bool, len(inv.Entries)),
			HaveLocs:   make(map[int][]string, len(inv.Entries)),
			KeyRingIDs: h.keyRingIDs(inv.Character),
		}
		for _, e := range inv.Entries {
			cp.HaveIDs[e.ID] = true
			cp.HaveLocs[e.ID] = append(cp.HaveLocs[e.ID], e.Location)
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
				// A component is held if the character (or shared bank) has the
				// canonical ItemID or any AltItemID — the latter handles "any one
				// of N" quest steps like Sleeper's Tomb talismans.
				have := holdsComponent(cp.HaveIDs, comp)
				inShared := !have && holdsComponent(sharedIDs, comp)
				cs := componentStatus{
					ItemID:   comp.ItemID,
					ItemName: comp.ItemName,
					Have:     have,
					// SharedBank is true if the only source is the shared bank.
					SharedBank: inShared,
				}
				// Record where the held item lives so the user can find the
				// right bag/bank slot — only meaningful when it's actually held.
				if have {
					cs.Locations = componentLocations(cp.HaveLocs, comp)
				} else if inShared {
					cs.Locations = componentLocations(sharedLocs, comp)
				}
				ckp.Components = append(ckp.Components, cs)
			}
			if kd.IntermediateItem != nil {
				ii := *kd.IntermediateItem
				have := cp.HaveIDs[ii.ItemID]
				inShared := !have && sharedIDs[ii.ItemID]
				cs := &componentStatus{
					ItemID:     ii.ItemID,
					ItemName:   ii.ItemName,
					Have:       have,
					SharedBank: inShared,
				}
				if have {
					cs.Locations = componentLocations(cp.HaveLocs, ii)
				} else if inShared {
					cs.Locations = componentLocations(sharedLocs, ii)
				}
				ckp.IntermediateItem = cs
			}
			if kd.FinalItem != nil {
				fi := *kd.FinalItem
				have := cp.HaveIDs[fi.ItemID]
				inShared := !have && sharedIDs[fi.ItemID]
				// Assembled keys are keyring items: once earned they're
				// consumed onto the /keys key ring and leave inventory, so a
				// fully-keyed character won't have the item in their Zeal
				// export. Treat a matching key-ring entry as "keyed".
				onRing := !have && !inShared && cp.KeyRingIDs[fi.ItemID]
				cs := &componentStatus{
					ItemID:     fi.ItemID,
					ItemName:   fi.ItemName,
					Have:       have,
					SharedBank: inShared,
					OnKeyRing:  onRing,
				}
				if have {
					cs.Locations = componentLocations(cp.HaveLocs, fi)
				} else if inShared {
					cs.Locations = componentLocations(sharedLocs, fi)
				}
				ckp.FinalItem = cs
			}
			kp.Characters = append(kp.Characters, ckp)
		}

		resp.Keys = append(resp.Keys, kp)
	}

	json.NewEncoder(w).Encode(resp)
}
