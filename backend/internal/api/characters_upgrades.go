package api

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/jasonsoprovich/pq-companion/backend/internal/character"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db/enums"
	"github.com/jasonsoprovich/pq-companion/backend/internal/upgrade"
	"github.com/jasonsoprovich/pq-companion/backend/internal/zeal"
)

// upgradeSlot is one logical worn slot the upgrade finder can target. Dual
// slots (Ear/Wrist/Fingers) collapse to a single logical slot whose Mask ORs
// both item bits and whose Location matches the canonical Quarmy name (Zeal
// normalizes Ear1/Ear2 → "Ear", Finger1/2 → "Fingers", etc.).
type upgradeSlot struct {
	Key      string
	Label    string
	Mask     int
	Location string
}

var upgradeSlots = []upgradeSlot{
	{"charm", "Charm", 0x000001, "Charm"},
	{"ear", "Ear", 0x000002 | 0x000010, "Ear"},
	{"head", "Head", 0x000004, "Head"},
	{"face", "Face", 0x000008, "Face"},
	{"neck", "Neck", 0x000020, "Neck"},
	{"shoulders", "Shoulders", 0x000040, "Shoulders"},
	{"arms", "Arms", 0x000080, "Arms"},
	{"back", "Back", 0x000100, "Back"},
	{"wrist", "Wrist", 0x000200 | 0x000400, "Wrist"},
	{"range", "Range", 0x000800, "Range"},
	{"hands", "Hands", 0x001000, "Hands"},
	{"primary", "Primary", 0x002000, "Primary"},
	{"secondary", "Secondary", 0x004000, "Secondary"},
	{"fingers", "Fingers", 0x008000 | 0x010000, "Fingers"},
	{"chest", "Chest", 0x020000, "Chest"},
	{"legs", "Legs", 0x040000, "Legs"},
	{"feet", "Feet", 0x080000, "Feet"},
	{"waist", "Waist", 0x100000, "Waist"},
	{"ammo", "Ammo", 0x200000, "Ammo"},
}

func upgradeSlotByKey(key string) (upgradeSlot, bool) {
	for _, s := range upgradeSlots {
		if s.Key == key {
			return s, true
		}
	}
	return upgradeSlot{}, false
}

// upgradeCurrentItem describes an item currently worn in the searched slot.
type upgradeCurrentItem struct {
	ID          int              `json:"id"`
	Name        string           `json:"name"`
	Icon        int              `json:"icon"`
	Stats       upgrade.StatLine `json:"stats"`
	FocusEffect int              `json:"focus_effect"`
	FocusName   string           `json:"focus_name"`
}

// upgradeResult is one scored candidate.
type upgradeResult struct {
	ID          int                 `json:"id"`
	Name        string              `json:"name"`
	Icon        int                 `json:"icon"`
	Slots       int                 `json:"slots"`
	NoDrop      int                 `json:"nodrop"`
	ReqLevel    int                 `json:"req_level"`
	RecLevel    int                 `json:"rec_level"`
	FocusEffect int                 `json:"focus_effect"`
	FocusName   string              `json:"focus_name"`
	Score       float64             `json:"score"`
	Deltas      []upgrade.StatDelta `json:"deltas"`
}

type upgradesResponse struct {
	Slot           string               `json:"slot"`
	SlotLabel      string               `json:"slot_label"`
	Class          int                  `json:"class"`
	Level          int                  `json:"level"`
	Weights        upgrade.Weights      `json:"weights"`
	CurrentItems   []upgradeCurrentItem `json:"current_items"`
	BaselineItemID int                  `json:"baseline_item_id"`
	Candidates     []upgradeResult      `json:"candidates"`
	Considered     int                  `json:"considered"`
	HasCurrentGear bool                 `json:"has_current_gear"`
}

// upgrades handles GET /api/characters/{id}/upgrades?slot=<key>&show_all=&limit=
// It ranks every item usable in the slot by the cap-aware upgrade score against
// the item currently worn there. Item sourcing (where it drops) is layered on
// in a later pass.
func (h *charactersHandler) upgrades(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	char, ok, err := h.store.Get(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "character not found")
		return
	}
	cfg := h.mgr.Get()
	if cfg.EQPath == "" {
		writeError(w, http.StatusBadRequest, "eq_path not configured")
		return
	}

	slot, ok := upgradeSlotByKey(r.URL.Query().Get("slot"))
	if !ok {
		writeError(w, http.StatusBadRequest, "unknown or missing slot")
		return
	}
	showAll := r.URL.Query().Get("show_all") == "1" || r.URL.Query().Get("show_all") == "true"
	limit := 75
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= 500 {
		limit = v
	}

	// Weights: an inline ?weights=<json> override (live slider preview) wins;
	// otherwise the character's saved tuning; otherwise the class default.
	weights := h.resolveWeights(char)
	if raw := r.URL.Query().Get("weights"); raw != "" {
		var override upgrade.Weights
		if err := json.Unmarshal([]byte(raw), &override); err == nil {
			weights = override
		}
	}
	ctx := upgrade.Context{Level: char.Level, Current: statLineFromBlock(h.currentTotals(char))}

	byLoc, hasGear := h.loadEquipped(cfg.EQPath, char.Name)
	current, baselineID, results, considered := h.scoreSlot(char, ctx, weights, slot, byLoc, showAll, limit)

	writeJSON(w, http.StatusOK, upgradesResponse{
		Slot:           slot.Key,
		SlotLabel:      slot.Label,
		Class:          char.Class,
		Level:          char.Level,
		Weights:        weights,
		CurrentItems:   current,
		BaselineItemID: baselineID,
		Candidates:     results,
		Considered:     considered,
		HasCurrentGear: hasGear,
	})
}

// overviewSlot is one slot's best upgrade in the all-slots overview.
type overviewSlot struct {
	Slot         string               `json:"slot"`
	SlotLabel    string               `json:"slot_label"`
	CurrentItems []upgradeCurrentItem `json:"current_items"`
	Best         *upgradeResult       `json:"best"`
	Considered   int                  `json:"considered"`
}

type overviewResponse struct {
	Class          int             `json:"class"`
	Level          int             `json:"level"`
	Weights        upgrade.Weights `json:"weights"`
	Slots          []overviewSlot  `json:"slots"`
	HasCurrentGear bool            `json:"has_current_gear"`
}

// upgradesOverview handles GET /api/characters/{id}/upgrades/overview — the
// single best upgrade per slot. It parses the Quarmy export and computes the
// character's totals once, then scores every slot, so the whole sweep is one
// request instead of ~19 round trips.
func (h *charactersHandler) upgradesOverview(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	char, ok, err := h.store.Get(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "character not found")
		return
	}
	cfg := h.mgr.Get()
	if cfg.EQPath == "" {
		writeError(w, http.StatusBadRequest, "eq_path not configured")
		return
	}

	weights := h.resolveWeights(char)
	if raw := r.URL.Query().Get("weights"); raw != "" {
		var override upgrade.Weights
		if err := json.Unmarshal([]byte(raw), &override); err == nil {
			weights = override
		}
	}
	ctx := upgrade.Context{Level: char.Level, Current: statLineFromBlock(h.currentTotals(char))}
	byLoc, hasGear := h.loadEquipped(cfg.EQPath, char.Name)

	slots := make([]overviewSlot, 0, len(upgradeSlots))
	for _, s := range upgradeSlots {
		current, _, results, considered := h.scoreSlot(char, ctx, weights, s, byLoc, false, 1)
		var best *upgradeResult
		if len(results) > 0 {
			best = &results[0]
		}
		slots = append(slots, overviewSlot{
			Slot:         s.Key,
			SlotLabel:    s.Label,
			CurrentItems: current,
			Best:         best,
			Considered:   considered,
		})
	}

	writeJSON(w, http.StatusOK, overviewResponse{
		Class:          char.Class,
		Level:          char.Level,
		Weights:        weights,
		Slots:          slots,
		HasCurrentGear: hasGear,
	})
}

// scoreSlot ranks candidates for one slot against the worn baseline (the
// lowest-scoring item in the slot — you upgrade your weakest ring first). It
// takes a pre-parsed equipped map so a multi-slot sweep parses the Quarmy
// export only once. Returns the worn items, the baseline item id, the ranked
// results (truncated to limit), and how many candidates were considered.
func (h *charactersHandler) scoreSlot(
	char character.Character, ctx upgrade.Context, weights upgrade.Weights,
	slot upgradeSlot, byLoc map[string][]zeal.InventoryEntry, showAll bool, limit int,
) (current []upgradeCurrentItem, baselineID int, results []upgradeResult, considered int) {
	current = h.equippedItemsForSlot(byLoc, slot)

	baseline := upgrade.StatLine{}
	if len(current) > 0 {
		worst := current[0]
		worstScore := upgrade.Score(ctx, weights, upgrade.StatLine{}, worst.Stats).Score
		for _, ci := range current[1:] {
			if s := upgrade.Score(ctx, weights, upgrade.StatLine{}, ci.Stats).Score; s < worstScore {
				worst, worstScore = ci, s
			}
		}
		baseline = worst.Stats
		baselineID = worst.ID
	}

	classBit := 0
	if char.Class >= 0 {
		classBit = 1 << char.Class
	}
	cands, err := h.db.UpgradeCandidates(db.CandidateFilter{
		SlotMask: slot.Mask,
		ClassBit: classBit,
		RaceBit:  enums.RaceBitForCharRace(char.Race),
		MaxLevel: char.Level,
		// Hide not-yet-available Planes of Power gear unless the PoP-era flag
		// is on (Dev panel → Flags), mirroring how spells/AAs are gated.
		ExcludePoP: !h.mgr.Get().Preferences.PoPEnabled,
	})
	if err != nil {
		return current, baselineID, []upgradeResult{}, 0
	}
	considered = len(cands)

	worn := make(map[int]bool, len(current))
	for _, ci := range current {
		worn[ci.ID] = true
	}
	results = make([]upgradeResult, 0, len(cands))
	for _, c := range cands {
		if worn[c.ID] {
			continue // don't suggest what's already equipped in this slot
		}
		res := upgrade.Score(ctx, weights, baseline, statLineFromCandidate(c))
		if !showAll && res.Score <= 0 {
			continue
		}
		results = append(results, upgradeResult{
			ID:          c.ID,
			Name:        c.Name,
			Icon:        c.Icon,
			Slots:       c.Slots,
			NoDrop:      c.NoDrop,
			ReqLevel:    c.ReqLevel,
			RecLevel:    c.RecLevel,
			FocusEffect: c.FocusEffect,
			FocusName:   c.FocusName,
			Score:       res.Score,
			Deltas:      res.Deltas,
		})
	}
	sort.SliceStable(results, func(i, j int) bool { return results[i].Score > results[j].Score })
	if len(results) > limit {
		results = results[:limit]
	}
	return current, baselineID, results, considered
}

// resolveWeights returns the character's saved upgrade weights, falling back to
// the class default preset when none are saved or the stored JSON is corrupt.
func (h *charactersHandler) resolveWeights(char character.Character) upgrade.Weights {
	if raw, ok, err := h.store.GetUpgradeWeights(char.ID); err == nil && ok {
		var wts upgrade.Weights
		if json.Unmarshal([]byte(raw), &wts) == nil {
			return wts
		}
	}
	return upgrade.DefaultWeights(char.Class)
}

// upgradeWeights handles GET /api/characters/{id}/upgrade-weights — the
// character's tuned weights, or the class default when none are saved. The
// response flags which it is so the UI can show a "reset to default" affordance.
func (h *charactersHandler) upgradeWeights(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	char, ok, err := h.store.Get(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "character not found")
		return
	}
	raw, custom, err := h.store.GetUpgradeWeights(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	weights := upgrade.DefaultWeights(char.Class)
	if custom {
		var wts upgrade.Weights
		if json.Unmarshal([]byte(raw), &wts) == nil {
			weights = wts
		} else {
			custom = false // corrupt row — present the default
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"weights":   weights,
		"is_custom": custom,
		"archetype": upgrade.ArchetypeFor(char.Class),
	})
}

// updateUpgradeWeights handles PUT /api/characters/{id}/upgrade-weights.
func (h *charactersHandler) updateUpgradeWeights(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	if _, ok, err := h.store.Get(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	} else if !ok {
		writeError(w, http.StatusNotFound, "character not found")
		return
	}
	var wts upgrade.Weights
	if err := json.NewDecoder(r.Body).Decode(&wts); err != nil {
		writeError(w, http.StatusBadRequest, "invalid weights JSON")
		return
	}
	raw, err := json.Marshal(wts)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := h.store.SetUpgradeWeights(id, string(raw)); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, wts)
}

// resetUpgradeWeights handles DELETE /api/characters/{id}/upgrade-weights —
// clears the tuning so the class default applies again.
func (h *charactersHandler) resetUpgradeWeights(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	char, ok, err := h.store.Get(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "character not found")
		return
	}
	if err := h.store.DeleteUpgradeWeights(id); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, upgrade.DefaultWeights(char.Class))
}

// currentTotals returns the character's Equipped stat layer (base + worn items
// + AA, no buffs) — the reference for how much headroom remains under each
// attribute/resist cap.
func (h *charactersHandler) currentTotals(char character.Character) statBlock {
	cfg := h.mgr.Get()
	var aa db.AABonuses
	if trained, err := h.store.ListAAs(char.ID); err == nil {
		conv := make([]db.TrainedAA, 0, len(trained))
		for _, t := range trained {
			conv = append(conv, db.TrainedAA{AAID: t.AAID, Rank: t.Rank})
		}
		aa, _ = h.db.AAStatBonuses(conv)
	}
	classIdx := char.Class + 1
	defenseSkill, _ := h.db.DefenseSkillCap(classIdx, char.Level)
	offenseSkill, _ := h.db.OffenseSkillCap(classIdx, char.Level)
	weaponSkill, _ := h.db.BestWeaponSkillCap(classIdx, char.Level)
	skills := skillCaps{defense: defenseSkill, offense: offenseSkill, weapon: weaponSkill}
	itemBlock, itemHaste := h.sumEquipment(cfg.EQPath, char.Name)
	return h.deriveBlock(char, aa, spellHasteSplit{}, skills, itemBlock, itemHaste, nil)
}

// loadEquipped parses the character's Quarmy export once into a location →
// worn-entries map. ok is false when no export exists, so callers can disable
// current-item comparison. Dual slots (Ear/Wrist/Fingers) collect under one
// canonical location, so a location may hold two entries.
func (h *charactersHandler) loadEquipped(eqPath, charName string) (byLoc map[string][]zeal.InventoryEntry, ok bool) {
	path := zeal.FindQuarmyFile(eqPath, charName)
	if path == "" {
		return nil, false
	}
	q, err := zeal.ParseQuarmy(path, charName)
	if err != nil || q == nil {
		return nil, false
	}
	byLoc = make(map[string][]zeal.InventoryEntry)
	for _, e := range q.Inventory {
		if e.ID > 0 {
			byLoc[e.Location] = append(byLoc[e.Location], e)
		}
	}
	return byLoc, true
}

// equippedItemsForSlot resolves the worn items in a logical slot from a
// pre-parsed equipped map.
func (h *charactersHandler) equippedItemsForSlot(byLoc map[string][]zeal.InventoryEntry, slot upgradeSlot) []upgradeCurrentItem {
	// Always a non-nil slice so it marshals to [] not null — the frontend
	// reads .length/.map on it directly.
	items := make([]upgradeCurrentItem, 0)
	for _, entry := range byLoc[slot.Location] {
		item, err := h.db.GetItem(entry.ID)
		if err != nil || item == nil {
			continue
		}
		items = append(items, upgradeCurrentItem{
			ID:          item.ID,
			Name:        item.Name,
			Icon:        item.Icon,
			Stats:       statLineFromItem(item),
			FocusEffect: item.FocusEffect,
			FocusName:   item.FocusName,
		})
	}
	return items
}

// statLineFromBlock copies the capped attribute/resist totals from a derived
// stat block into a StatLine. Only the fields used as cap references matter;
// HP/Mana/AC are carried for completeness but unused by the cap math.
func statLineFromBlock(b statBlock) upgrade.StatLine {
	return upgrade.StatLine{
		HP: b.HP, Mana: b.Mana, AC: b.AC,
		STR: b.STR, STA: b.STA, AGI: b.AGI, DEX: b.DEX,
		WIS: b.WIS, INT: b.INT, CHA: b.CHA,
		MR: b.MR, FR: b.FR, CR: b.CR, DR: b.DR, PR: b.PR,
	}
}

func statLineFromCandidate(c db.UpgradeCandidate) upgrade.StatLine {
	return upgrade.StatLine{
		HP: c.HP, Mana: c.Mana, AC: c.AC,
		STR: c.STR, STA: c.STA, AGI: c.AGI, DEX: c.DEX,
		WIS: c.WIS, INT: c.INT, CHA: c.CHA,
		MR: c.MR, FR: c.FR, CR: c.CR, DR: c.DR, PR: c.PR,
	}
}

func statLineFromItem(it *db.Item) upgrade.StatLine {
	return upgrade.StatLine{
		HP: it.HP, Mana: it.Mana, AC: it.AC,
		STR: it.Strength, STA: it.Stamina, AGI: it.Agility, DEX: it.Dexterity,
		WIS: it.Wisdom, INT: it.Intelligence, CHA: it.Charisma,
		MR: it.MagicResist, FR: it.FireResist, CR: it.ColdResist,
		DR: it.DiseaseResist, PR: it.PoisonResist,
	}
}
