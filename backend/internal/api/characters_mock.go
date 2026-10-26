package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/jasonsoprovich/pq-companion/backend/internal/buffmod"
	"github.com/jasonsoprovich/pq-companion/backend/internal/character"
	"github.com/jasonsoprovich/pq-companion/backend/internal/zeal"
)

// Mocked-loadout ("what-if") support for the Spell Modifiers Test Resolution
// panel. A mock is a set of overrides on top of the character's real Quarmy
// export: gear keyed by the upgrade finder's physical slot keys (ear1, head,
// finger2, …; item id 0 = slot left empty) and AA ranks keyed by eqmacid
// (rank 0 = untrained). Mocks never touch the character's stored data — they
// only change the contributor list handed to buffmod.Resolve.

// mockItemBrief is the item slice the what-if editor shows per slot.
type mockItemBrief struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Icon      int    `json:"icon"`
	Slots     int    `json:"slots"`
	FocusID   int    `json:"focus_id,omitempty"`
	FocusName string `json:"focus_name,omitempty"`
}

type mockBaseSlot struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	// Mask is the equip-slot bitmask an item must overlap to fit this slot
	// (feeds the item picker's slot filter).
	Mask int `json:"mask"`
	// Bucket is the wishlist slot bucket an item from this slot files under.
	Bucket string         `json:"bucket"`
	Item   *mockItemBrief `json:"item"` // actually worn item; null = empty
}

type mockBaseAA struct {
	buffmod.MockableAA
	Rank int `json:"rank"` // actually trained rank
}

type mockBaseResponse struct {
	HasQuarmy bool           `json:"has_quarmy"`
	Slots     []mockBaseSlot `json:"slots"`
	AAs       []mockBaseAA   `json:"aas"`
}

// mockOverrides is the what-if payload shared by resolve and saved loadouts.
type mockOverrides struct {
	Gear map[string]int `json:"gear"`
	AAs  map[int]int    `json:"aas"`
}

type mockResolveRequest struct {
	SpellID int `json:"spell_id"`
	mockOverrides
}

type mockResolveResponse struct {
	Actual           buffmod.Resolution `json:"actual"`
	Mock             buffmod.Resolution `json:"mock"`
	MockContributors []buffmod.Modifier `json:"mock_contributors"`
}

// wishlistBucketForLocation maps a Zeal worn location to its wishlist bucket
// (the wishlist uses singular Shoulder/Finger).
func wishlistBucketForLocation(loc string) string {
	switch loc {
	case "Shoulders":
		return "Shoulder"
	case "Fingers":
		return "Finger"
	}
	return loc
}

// loadQuarmyForMock parses the character's Quarmy export. A missing export is
// not an error: the what-if then starts from bare gear and no AAs.
func (h *charactersHandler) loadQuarmyForMock(charName string) (*zeal.QuarmyData, error) {
	cfg := h.mgr.Get()
	if cfg.EQPath == "" {
		return nil, nil
	}
	path := zeal.FindQuarmyFile(cfg.EQPath, charName)
	if path == "" {
		return nil, nil
	}
	return zeal.ParseQuarmy(path, charName)
}

// wornBySlot maps each upgrade slot key to the item id actually worn there.
func wornBySlot(q *zeal.QuarmyData) map[string]int {
	out := make(map[string]int, len(upgradeSlots))
	if q == nil {
		return out
	}
	byLoc := make(map[string][]int)
	for _, e := range q.Inventory {
		if e.ID > 0 {
			byLoc[e.Location] = append(byLoc[e.Location], e.ID)
		}
	}
	for _, s := range upgradeSlots {
		if ids := byLoc[s.Location]; s.Index < len(ids) {
			out[s.Key] = ids[s.Index]
		}
	}
	return out
}

// mockEquipment applies overrides to the real export and returns the worn
// inventory + AA list to feed buffmod.FromEquipment.
func mockEquipment(q *zeal.QuarmyData, o mockOverrides) ([]zeal.InventoryEntry, []zeal.AAEntry) {
	worn := wornBySlot(q)
	inv := make([]zeal.InventoryEntry, 0, len(upgradeSlots))
	for _, s := range upgradeSlots {
		id := worn[s.Key]
		if v, ok := o.Gear[s.Key]; ok {
			id = v
		}
		if id > 0 {
			inv = append(inv, zeal.InventoryEntry{Location: s.Location, ID: id})
		}
	}

	ranks := map[int]int{}
	if q != nil {
		for _, aa := range q.AAs {
			ranks[aa.ID] = aa.Rank
		}
	}
	for id, rank := range o.AAs {
		ranks[id] = rank
	}
	aas := make([]zeal.AAEntry, 0, len(ranks))
	for id, rank := range ranks {
		if rank > 0 {
			aas = append(aas, zeal.AAEntry{ID: id, Rank: rank})
		}
	}
	return inv, aas
}

// validateMockOverrides rejects unknown slots/AAs, out-of-range ranks, and
// items that don't exist or can't be worn in the slot they're mocked into.
func (h *charactersHandler) validateMockOverrides(o mockOverrides) (int, string) {
	for key, itemID := range o.Gear {
		slot, ok := upgradeSlotByKey(key)
		if !ok {
			return http.StatusBadRequest, "unknown slot: " + key
		}
		if itemID < 0 {
			return http.StatusBadRequest, "invalid item id for " + key
		}
		if itemID == 0 {
			continue
		}
		item, err := h.db.GetItem(itemID)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return http.StatusBadRequest, "item not found: " + strconv.Itoa(itemID)
			}
			return http.StatusInternalServerError, "failed to look up item"
		}
		if item.Slots&slot.Mask == 0 {
			return http.StatusBadRequest, item.Name + " can't be worn in " + slot.Label
		}
	}
	maxRank := map[int]int{}
	for _, aa := range buffmod.MockableAAs() {
		maxRank[aa.ID] = aa.MaxRank
	}
	for id, rank := range o.AAs {
		max, ok := maxRank[id]
		if !ok {
			return http.StatusBadRequest, "AA " + strconv.Itoa(id) + " doesn't affect spell modifiers"
		}
		if rank < 0 || rank > max {
			return http.StatusBadRequest, "invalid rank for AA " + strconv.Itoa(id)
		}
	}
	return 0, ""
}

func (h *charactersHandler) mockItemBrief(itemID int) *mockItemBrief {
	if itemID <= 0 {
		return nil
	}
	item, err := h.db.GetItem(itemID)
	if err != nil || item == nil {
		return nil
	}
	b := &mockItemBrief{ID: item.ID, Name: item.Name, Icon: item.Icon, Slots: item.Slots}
	if item.FocusEffect > 0 {
		if sp, err := h.db.GetSpell(item.FocusEffect); err == nil && sp != nil {
			b.FocusID = sp.ID
			b.FocusName = sp.Name
		}
	}
	return b
}

// charFromURL loads the {id} character, writing the error response itself.
func (h *charactersHandler) charFromURL(w http.ResponseWriter, r *http.Request) (character.Character, bool) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return character.Character{}, false
	}
	char, ok, err := h.store.Get(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return character.Character{}, false
	}
	if !ok {
		writeError(w, http.StatusNotFound, "character not found")
		return character.Character{}, false
	}
	return char, true
}

// mockBase returns the starting point for the what-if editor: the item
// actually worn in every slot and the trained rank of every mockable AA.
func (h *charactersHandler) mockBase(w http.ResponseWriter, r *http.Request) {
	char, ok := h.charFromURL(w, r)
	if !ok {
		return
	}
	q, err := h.loadQuarmyForMock(char.Name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "parse quarmy: "+err.Error())
		return
	}
	worn := wornBySlot(q)
	resp := mockBaseResponse{HasQuarmy: q != nil, Slots: []mockBaseSlot{}, AAs: []mockBaseAA{}}
	for _, s := range upgradeSlots {
		resp.Slots = append(resp.Slots, mockBaseSlot{
			Key: s.Key, Label: s.Label, Mask: s.Mask,
			Bucket: wishlistBucketForLocation(s.Location),
			Item:   h.mockItemBrief(worn[s.Key]),
		})
	}
	trained := map[int]int{}
	if q != nil {
		for _, aa := range q.AAs {
			trained[aa.ID] = aa.Rank
		}
	}
	for _, aa := range buffmod.MockableAAs() {
		resp.AAs = append(resp.AAs, mockBaseAA{MockableAA: aa, Rank: trained[aa.ID]})
	}
	writeJSON(w, http.StatusOK, resp)
}

// mockResolve resolves one spell twice — against the actual export and
// against the mocked loadout — so the panel can show them side by side.
func (h *charactersHandler) mockResolve(w http.ResponseWriter, r *http.Request) {
	char, ok := h.charFromURL(w, r)
	if !ok {
		return
	}
	var req mockResolveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.SpellID <= 0 {
		writeError(w, http.StatusBadRequest, "spell_id is required")
		return
	}
	if status, msg := h.validateMockOverrides(req.mockOverrides); status != 0 {
		writeError(w, status, msg)
		return
	}
	q, err := h.loadQuarmyForMock(char.Name)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "parse quarmy: "+err.Error())
		return
	}

	var actualRes *buffmod.Result
	if q != nil {
		actualRes = buffmod.FromEquipment(char.Name, q.Inventory, q.AAs, h.db)
	} else {
		actualRes = buffmod.FromEquipment(char.Name, nil, nil, h.db)
	}
	inv, aas := mockEquipment(q, req.mockOverrides)
	mockRes := buffmod.FromEquipment(char.Name, inv, aas, h.db)

	actual, status, msg := h.resolveSpellFor(char, req.SpellID, actualRes)
	if status != 0 {
		writeError(w, status, msg)
		return
	}
	mock, status, msg := h.resolveSpellFor(char, req.SpellID, mockRes)
	if status != 0 {
		writeError(w, status, msg)
		return
	}
	writeJSON(w, http.StatusOK, mockResolveResponse{
		Actual: actual, Mock: mock, MockContributors: mockRes.Contributors,
	})
}

// ── Saved loadouts ──────────────────────────────────────────────────────────

type mockLoadoutRequest struct {
	Name string `json:"name"`
	mockOverrides
}

func (h *charactersHandler) decodeMockLoadout(w http.ResponseWriter, r *http.Request) (mockLoadoutRequest, bool) {
	var req mockLoadoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return req, false
	}
	req.Name = strings.TrimSpace(req.Name)
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name is required")
		return req, false
	}
	if len(req.Name) > 64 {
		writeError(w, http.StatusBadRequest, "name too long (max 64)")
		return req, false
	}
	if status, msg := h.validateMockOverrides(req.mockOverrides); status != 0 {
		writeError(w, status, msg)
		return req, false
	}
	return req, true
}

func (h *charactersHandler) listMockLoadouts(w http.ResponseWriter, r *http.Request) {
	char, ok := h.charFromURL(w, r)
	if !ok {
		return
	}
	list, err := h.store.ListMockLoadouts(char.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"loadouts": list})
}

func (h *charactersHandler) createMockLoadout(w http.ResponseWriter, r *http.Request) {
	char, ok := h.charFromURL(w, r)
	if !ok {
		return
	}
	req, ok := h.decodeMockLoadout(w, r)
	if !ok {
		return
	}
	l, err := h.store.CreateMockLoadout(char.ID, req.Name, req.Gear, req.AAs)
	if errors.Is(err, character.ErrMockLoadoutNameTaken) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, l)
}

func (h *charactersHandler) updateMockLoadout(w http.ResponseWriter, r *http.Request) {
	char, ok := h.charFromURL(w, r)
	if !ok {
		return
	}
	loadoutID, err := strconv.Atoi(chi.URLParam(r, "loadoutID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid loadout id")
		return
	}
	req, ok := h.decodeMockLoadout(w, r)
	if !ok {
		return
	}
	l, found, err := h.store.UpdateMockLoadout(char.ID, loadoutID, req.Name, req.Gear, req.AAs)
	if errors.Is(err, character.ErrMockLoadoutNameTaken) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "loadout not found")
		return
	}
	writeJSON(w, http.StatusOK, l)
}

func (h *charactersHandler) deleteMockLoadout(w http.ResponseWriter, r *http.Request) {
	char, ok := h.charFromURL(w, r)
	if !ok {
		return
	}
	loadoutID, err := strconv.Atoi(chi.URLParam(r, "loadoutID"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid loadout id")
		return
	}
	if err := h.store.DeleteMockLoadout(char.ID, loadoutID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
