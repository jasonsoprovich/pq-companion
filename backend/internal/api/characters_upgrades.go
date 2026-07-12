package api

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/jasonsoprovich/pq-companion/backend/internal/character"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db/enums"
	"github.com/jasonsoprovich/pq-companion/backend/internal/era"
	"github.com/jasonsoprovich/pq-companion/backend/internal/upgrade"
	"github.com/jasonsoprovich/pq-companion/backend/internal/zeal"
)

// upgradeSlot is one worn slot the upgrade finder can target. The paired slots
// (Ear/Wrist/Fingers) are split into two targets each — Ear 1 / Ear 2, etc. —
// so the finder ranks against the specific item worn in that physical slot
// (the user knows which of their two earrings the suggestion replaces). Both
// slots in a pair share the same Mask (ORing both item bits — an earring fits
// either ear, so the candidate pool is identical) and the same Location (Zeal
// normalizes Ear1/Ear2 → "Ear", Finger1/2 → "Fingers", etc.). Index picks the
// equipped item at that Location to use as the baseline (0 for the first, 1 for
// the second). Non-paired slots use Index 0.
type upgradeSlot struct {
	Key      string
	Label    string
	Mask     int
	Location string
	Index    int
}

var upgradeSlots = []upgradeSlot{
	// No Charm slot — Project Quarm (TAKP/EQMac client) has no charm slot.
	{"ear1", "Ear 1", 0x000002 | 0x000010, "Ear", 0},
	{"ear2", "Ear 2", 0x000002 | 0x000010, "Ear", 1},
	{"head", "Head", 0x000004, "Head", 0},
	{"face", "Face", 0x000008, "Face", 0},
	{"neck", "Neck", 0x000020, "Neck", 0},
	{"shoulders", "Shoulders", 0x000040, "Shoulders", 0},
	{"arms", "Arms", 0x000080, "Arms", 0},
	{"back", "Back", 0x000100, "Back", 0},
	{"wrist1", "Wrist 1", 0x000200 | 0x000400, "Wrist", 0},
	{"wrist2", "Wrist 2", 0x000200 | 0x000400, "Wrist", 1},
	{"range", "Range", 0x000800, "Range", 0},
	{"hands", "Hands", 0x001000, "Hands", 0},
	{"primary", "Primary", 0x002000, "Primary", 0},
	{"secondary", "Secondary", 0x004000, "Secondary", 0},
	{"finger1", "Finger 1", 0x008000 | 0x010000, "Fingers", 0},
	{"finger2", "Finger 2", 0x008000 | 0x010000, "Fingers", 1},
	{"chest", "Chest", 0x020000, "Chest", 0},
	{"legs", "Legs", 0x040000, "Legs", 0},
	{"feet", "Feet", 0x080000, "Feet", 0},
	{"waist", "Waist", 0x100000, "Waist", 0},
	{"ammo", "Ammo", 0x200000, "Ammo", 0},
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
	ClickEffect int                 `json:"click_effect"`
	ClickName   string              `json:"click_name"`
	ProcEffect  int                 `json:"proc_effect"`
	ProcName    string              `json:"proc_name"`
	Score       float64             `json:"score"`
	Deltas      []upgrade.StatDelta `json:"deltas"`
	// PriorityFocus is true when this candidate carries one of the character's
	// priority focus effects that they don't already have equipped — the score
	// includes the focus bonus and the UI flags it.
	PriorityFocus bool `json:"priority_focus"`
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
	// PoP gear is hidden unless explicitly requested (independent of the global
	// pop_enabled era flag, so the finder is predictable).
	excludePoP := !(r.URL.Query().Get("show_pop") == "1" || r.URL.Query().Get("show_pop") == "true")
	// Crafted (tradeskill-made) gear is hidden by default; ?hide_crafted=0 keeps it.
	excludeCrafted := r.URL.Query().Get("hide_crafted") != "0" && r.URL.Query().Get("hide_crafted") != "false"
	// NO DROP gear is shown by default; ?hide_nodrop=1 drops it.
	excludeNoDrop := r.URL.Query().Get("hide_nodrop") == "1" || r.URL.Query().Get("hide_nodrop") == "true"
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
	worn := h.resolveWornItems(byLoc)
	prioritySet := h.priorityFocusSet(id)
	equippedFocus := h.equippedFocusSet(worn)
	wornLore := h.equippedLoreSet(worn)
	wc := h.newWornCache()
	hasteByLoc := h.hasteByLocation(byLoc, worn, wc)
	current, baselineID, results, considered, err := h.scoreSlot(char, ctx, weights, slot, byLoc, worn, showAll, excludePoP, excludeCrafted, excludeNoDrop, limit, prioritySet, equippedFocus, wornLore, wc, hasteByLoc)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load upgrade candidates")
		return
	}

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

// equippedSlotEntry is one physical slot's worn item, for the item-compare
// "vs equipped" view. ItemIDs has 0 or 1 entries — empty when that slot is bare.
type equippedSlotEntry struct {
	Slot      string `json:"slot"`
	SlotLabel string `json:"slot_label"`
	ItemIDs   []int  `json:"item_ids"`
}

type equippedInSlotsResponse struct {
	HasGear bool                 `json:"has_gear"`
	Slots   []equippedSlotEntry `json:"slots"`
}

// equippedInSlots handles GET /api/characters/{id}/equipped-in-slots?slots=<mask>.
// Given an item's slots bitmask (item.slots), it returns what the character
// currently has worn in each matching physical slot — the baseline the item
// compare view diffs a candidate item against. Item stats aren't included
// here; the frontend fetches full item data via GET /items/{id} so the diff
// runs through the same renderer as a plain item-vs-item compare.
func (h *charactersHandler) equippedInSlots(w http.ResponseWriter, r *http.Request) {
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
	mask, err := strconv.Atoi(r.URL.Query().Get("slots"))
	if err != nil || mask <= 0 {
		writeError(w, http.StatusBadRequest, "invalid or missing slots mask")
		return
	}

	byLoc, hasGear := h.loadEquipped(cfg.EQPath, char.Name)
	worn := h.resolveWornItems(byLoc)
	wc := h.newWornCache()

	slots := make([]equippedSlotEntry, 0)
	for _, s := range upgradeSlots {
		if s.Mask&mask == 0 {
			continue
		}
		allWorn := h.equippedItemsForSlot(byLoc, s, wc, worn)
		ids := make([]int, 0, 1)
		if s.Index < len(allWorn) {
			ids = append(ids, allWorn[s.Index].ID)
		}
		slots = append(slots, equippedSlotEntry{Slot: s.Key, SlotLabel: s.Label, ItemIDs: ids})
	}

	writeJSON(w, http.StatusOK, equippedInSlotsResponse{HasGear: hasGear, Slots: slots})
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
	worn := h.resolveWornItems(byLoc)
	prioritySet := h.priorityFocusSet(id)
	equippedFocus := h.equippedFocusSet(worn)
	wornLore := h.equippedLoreSet(worn)
	wc := h.newWornCache()
	hasteByLoc := h.hasteByLocation(byLoc, worn, wc)
	excludePoP := !(r.URL.Query().Get("show_pop") == "1" || r.URL.Query().Get("show_pop") == "true")
	excludeCrafted := r.URL.Query().Get("hide_crafted") != "0" && r.URL.Query().Get("hide_crafted") != "false"
	excludeNoDrop := r.URL.Query().Get("hide_nodrop") == "1" || r.URL.Query().Get("hide_nodrop") == "true"

	// One candidate scan for the whole sweep: the class/race/level/hidden/variant
	// filter is identical across all 19 slots — only the slot mask differs — so
	// query the union of every slot mask once and partition in Go by slot. This
	// replaces 19 full-table item scans with a single one.
	combinedMask := 0
	for _, s := range upgradeSlots {
		combinedMask |= s.Mask
	}
	allCands, err := h.db.UpgradeCandidates(db.CandidateFilter{
		SlotMask:       combinedMask,
		ClassBit:       charClassBit(char),
		RaceBit:        enums.RaceBitForCharRace(char.Race),
		MaxLevel:       upgradeMaxLevel(char.Level, excludePoP),
		ExcludePoP:     excludePoP,
		ExcludeCrafted: excludeCrafted,
		ExcludeNoDrop:  excludeNoDrop,
	})
	if err != nil {
		// A locked/corrupt quarm.db must surface as an error, not an empty
		// overview that reads as "everything is best-in-slot".
		writeError(w, http.StatusInternalServerError, "failed to load upgrade candidates")
		return
	}

	slots := make([]overviewSlot, 0, len(upgradeSlots))
	for _, s := range upgradeSlots {
		var slotCands []db.UpgradeCandidate
		for _, c := range allCands {
			if c.Slots&s.Mask != 0 {
				slotCands = append(slotCands, c)
			}
		}
		current, _, results, considered := h.scoreSlotCands(char, ctx, weights, s, byLoc, worn, false, 1, prioritySet, equippedFocus, wornLore, wc, hasteByLoc, slotCands)
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

// upgradeMaxLevel is the reqlevel ceiling for candidate items. When PoP gear is
// included, raise it to the PoP cap (65) so level-65 Planes gear isn't filtered
// out for a level-60 character — the player is browsing PoP-era upgrades and
// will hit that cap when the expansion goes live. Otherwise stay at the
// character's actual level.
func upgradeMaxLevel(charLevel int, excludePoP bool) int {
	if !excludePoP && era.PoPMaxLevel > charLevel {
		return era.PoPMaxLevel
	}
	return charLevel
}

// charClassBit returns the items.classes bit for a character's class (0 for an
// unset/negative class, meaning "don't class-filter").
func charClassBit(char character.Character) int {
	if char.Class >= 0 {
		return 1 << char.Class
	}
	return 0
}

// scoreSlot ranks candidates for one slot against the worn baseline. It fetches
// the slot's candidate set and delegates the scoring to scoreSlotCands. Used by
// the single-slot endpoint; the overview fetches all slots' candidates in one
// query and calls scoreSlotCands directly.
func (h *charactersHandler) scoreSlot(
	char character.Character, ctx upgrade.Context, weights upgrade.Weights,
	slot upgradeSlot, byLoc map[string][]zeal.InventoryEntry, worn map[int]*db.Item, showAll, excludePoP, excludeCrafted, excludeNoDrop bool, limit int,
	prioritySet, equippedFocus, wornLore map[int]bool, wc *wornCache, hasteByLoc map[string]int,
) (current []upgradeCurrentItem, baselineID int, results []upgradeResult, considered int, err error) {
	cands, err := h.db.UpgradeCandidates(db.CandidateFilter{
		SlotMask:       slot.Mask,
		ClassBit:       charClassBit(char),
		RaceBit:        enums.RaceBitForCharRace(char.Race),
		MaxLevel:       upgradeMaxLevel(char.Level, excludePoP),
		ExcludePoP:     excludePoP,     // finder-local "Show PoP gear" toggle (default hide)
		ExcludeCrafted: excludeCrafted, // finder-local "Hide crafted gear" toggle (default hide)
		ExcludeNoDrop:  excludeNoDrop,  // finder-local "Hide NO DROP" toggle (default show)
	})
	if err != nil {
		// Propagate: a locked/corrupt quarm.db must not be reported to the user
		// as "no upgrades" (indistinguishable from best-in-slot).
		return nil, 0, nil, 0, err
	}
	current, baselineID, results, considered = h.scoreSlotCands(char, ctx, weights, slot, byLoc, worn, showAll, limit, prioritySet, equippedFocus, wornLore, wc, hasteByLoc, cands)
	return current, baselineID, results, considered, nil
}

// scoreSlotCands scores a pre-fetched candidate set for one slot against the
// item worn in that physical slot (slot.Index picks Ear 1 vs Ear 2, etc.). It
// takes a pre-parsed equipped map so a multi-slot sweep parses
// the Quarmy export only once. Returns the worn items, the baseline item id, the
// ranked results (truncated to limit), and how many candidates were considered.
func (h *charactersHandler) scoreSlotCands(
	char character.Character, ctx upgrade.Context, weights upgrade.Weights,
	slot upgradeSlot, byLoc map[string][]zeal.InventoryEntry, worn map[int]*db.Item, showAll bool, limit int,
	prioritySet, equippedFocus, wornLore map[int]bool, wc *wornCache, hasteByLoc map[string]int,
	cands []db.UpgradeCandidate,
) (current []upgradeCurrentItem, baselineID int, results []upgradeResult, considered int) {
	focusBonus := weights.FocusBonus
	if focusBonus <= 0 {
		focusBonus = upgrade.DefaultFocusBonus
	}

	// Worn haste is best-of-type: a candidate only gains haste if it beats the
	// best the character already wears in OTHER slots (toward the level cap).
	otherHaste := 0
	for loc, hv := range hasteByLoc {
		if loc != slot.Location && hv > otherHaste {
			otherHaste = hv
		}
	}
	ctx.OtherHaste = otherHaste

	// allWorn is every item at this Location (both earrings, both rings, …);
	// current is just the one in THIS physical slot (slot.Index), which is what
	// we display and rank against. An empty second slot → empty baseline → the
	// candidate ranks against nothing, which is correct for a bare slot.
	allWorn := h.equippedItemsForSlot(byLoc, slot, wc, worn)
	current = make([]upgradeCurrentItem, 0, 1)
	if slot.Index < len(allWorn) {
		current = append(current, allWorn[slot.Index])
	}

	baseline := upgrade.StatLine{}
	if len(current) > 0 {
		baseline = current[0].Stats
		baselineID = current[0].ID
	}

	considered = len(cands)

	// Exclude every item worn at this Location (both paired slots), so the other
	// earring/ring you already wear is never suggested as an upgrade.
	wornHere := make(map[int]bool, len(allWorn))
	for _, ci := range allWorn {
		wornHere[ci.ID] = true
	}
	results = make([]upgradeResult, 0, len(cands))
	for _, c := range cands {
		if wornHere[c.ID] {
			continue // don't suggest what's already equipped in this slot
		}
		if wornLore[c.ID] {
			continue // a LORE item worn elsewhere can't be acquired a second time
		}
		res := upgrade.Score(ctx, weights, baseline, h.candStatLine(c, wc))
		score := res.Score
		// Priority focus the character wants but doesn't already have equipped
		// gets a bonus so a focus item floats up over modest stat upgrades.
		priority := c.FocusEffect > 0 && prioritySet[c.FocusEffect] && !equippedFocus[c.FocusEffect]
		if priority {
			score += focusBonus
		}
		if !showAll && score <= 0 {
			continue
		}
		results = append(results, upgradeResult{
			ID:            c.ID,
			Name:          c.Name,
			Icon:          c.Icon,
			Slots:         c.Slots,
			NoDrop:        c.NoDrop,
			ReqLevel:      c.ReqLevel,
			RecLevel:      c.RecLevel,
			FocusEffect:   c.FocusEffect,
			FocusName:     c.FocusName,
			ClickEffect:   c.ClickEffect,
			ClickName:     c.ClickName,
			ProcEffect:    c.ProcEffect,
			ProcName:      c.ProcName,
			Score:         score,
			Deltas:        res.Deltas,
			PriorityFocus: priority,
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
	def := upgrade.DefaultWeights(char.Class)
	if raw, ok, err := h.store.GetUpgradeWeights(char.ID); err == nil && ok {
		var wts upgrade.Weights
		if json.Unmarshal([]byte(raw), &wts) == nil {
			// Backfill weights added after this profile was saved, so an older
			// saved set picks up DPS/focus scoring instead of silently using 0
			// (JSON can't distinguish a missing key from an intentional 0).
			if !strings.Contains(raw, `"dps"`) {
				wts.DPS = def.DPS
			}
			if !strings.Contains(raw, `"focus_bonus"`) {
				wts.FocusBonus = def.FocusBonus
			}
			if !strings.Contains(raw, `"atk"`) {
				wts.ATK = def.ATK
			}
			if !strings.Contains(raw, `"haste"`) {
				wts.Haste = def.Haste
			}
			if !strings.Contains(raw, `"mana_regen"`) {
				wts.ManaRegen = def.ManaRegen
			}
			return wts
		}
	}
	return def
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
	_, custom, err := h.store.GetUpgradeWeights(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// resolveWeights returns the effective set with any newly-added weights
	// (DPS, focus bonus) backfilled, so the editor shows real values.
	writeJSON(w, http.StatusOK, map[string]any{
		"weights":   h.resolveWeights(char),
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

// priorityFocusSet returns the character's priority focus-effect spell ids.
func (h *charactersHandler) priorityFocusSet(charID int) map[int]bool {
	set := map[int]bool{}
	if ids, err := h.store.GetPriorityFocus(charID); err == nil {
		for _, id := range ids {
			set[id] = true
		}
	}
	return set
}

// resolveWornItems resolves every distinct worn item id in the equipped map to
// its full Item once. GetItem is a wide multi-subquery read, and the focus and
// haste passes each used to call it over the whole loadout; sharing one resolved
// map collapses those repeated sweeps into a single lookup pass.
func (h *charactersHandler) resolveWornItems(byLoc map[string][]zeal.InventoryEntry) map[int]*db.Item {
	out := make(map[int]*db.Item)
	for _, entries := range byLoc {
		for _, e := range entries {
			if _, seen := out[e.ID]; seen {
				continue
			}
			if item, err := h.db.GetItem(e.ID); err == nil && item != nil {
				out[e.ID] = item
			}
		}
	}
	return out
}

// equippedFocusSet returns the focus effects the character already has equipped
// (any slot). A priority focus already in this set earns no bonus — only the
// best of a focus type matters, so getting another copy is not an upgrade.
func (h *charactersHandler) equippedFocusSet(worn map[int]*db.Item) map[int]bool {
	set := map[int]bool{}
	for _, item := range worn {
		if item.FocusEffect > 0 {
			set[item.FocusEffect] = true
		}
	}
	return set
}

// equippedLoreSet returns the IDs of LORE items the character already wears in
// any slot. A LORE item is unique — you can only possess one — so once it's
// equipped it must never be offered as an upgrade for a different slot (you
// can't acquire a second). EQ encodes LORE as a lore string beginning with '*'.
// (Same-slot duplicates are already filtered by the per-slot worn check, so this
// only matters for multi-slot items like a charm/torch that also fit elsewhere.)
func (h *charactersHandler) equippedLoreSet(worn map[int]*db.Item) map[int]bool {
	set := map[int]bool{}
	for id, item := range worn {
		if strings.HasPrefix(item.Lore, "*") {
			set[id] = true
		}
	}
	return set
}

// focusOptions handles GET /api/characters/{id}/focus-options — the distinct
// focus effects available on the character's class gear, for the priority picker.
func (h *charactersHandler) focusOptions(w http.ResponseWriter, r *http.Request) {
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
	classBit := 0
	if char.Class >= 0 {
		classBit = 1 << char.Class
	}
	opts, err := h.db.FocusOptions(classBit, char.Level)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, opts)
}

// priorityFocus handles GET /api/characters/{id}/priority-focus.
func (h *charactersHandler) priorityFocus(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	ids, err := h.store.GetPriorityFocus(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string][]int{"spell_ids": ids})
}

// updatePriorityFocus handles PUT /api/characters/{id}/priority-focus.
func (h *charactersHandler) updatePriorityFocus(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return
	}
	var req struct {
		SpellIDs []int `json:"spell_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	if err := h.store.SetPriorityFocus(id, req.SpellIDs); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string][]int{"spell_ids": req.SpellIDs})
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
// pre-parsed equipped map, looking each up in the already-resolved worn map
// (resolveWornItems) rather than re-issuing a wide GetItem (each of which nests
// a GetSpell) per worn piece — ~25-40 redundant queries per overview request.
func (h *charactersHandler) equippedItemsForSlot(byLoc map[string][]zeal.InventoryEntry, slot upgradeSlot, wc *wornCache, worn map[int]*db.Item) []upgradeCurrentItem {
	// Always a non-nil slice so it marshals to [] not null — the frontend
	// reads .length/.map on it directly.
	items := make([]upgradeCurrentItem, 0)
	for _, entry := range byLoc[slot.Location] {
		item := worn[entry.ID]
		if item == nil {
			continue
		}
		items = append(items, upgradeCurrentItem{
			ID:          item.ID,
			Name:        item.Name,
			Icon:        item.Icon,
			Stats:       h.itemStatLine(item, wc),
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
		// Attack and mana regen are the headroom references for the soft-capped
		// ATK (250) and Flowing Thought (15) scoring terms. Only worn FT counts
		// against the item FT cap — b.ManaRegen (AA/buff flat regen) is uncapped
		// and never item-sourced, so folding it in here would falsely report the
		// 15 cap as reached and hide real FT upgrades.
		Attack: b.Attack, ManaRegen: b.FT,
	}
}

// wornCache memoizes worn-effect spell contributions within one request. A
// worn effect can grant ATK, melee haste, and assorted stats; parsing the spell
// once per distinct (worneffect, wornlevel) keeps the cost bounded even when a
// slot has thousands of candidates.
type wornCache struct {
	h *charactersHandler
	m map[[2]int]upgrade.StatLine
}

func (h *charactersHandler) newWornCache() *wornCache {
	return &wornCache{h: h, m: map[[2]int]upgrade.StatLine{}}
}

// contribution returns the StatLine a worn effect adds (zero when there's no
// worn effect). Reuses parseWornEffect so the finder matches the char-stats
// engine's notion of what a worn effect grants.
func (wc *wornCache) contribution(wornEffect, wornLevel int) upgrade.StatLine {
	if wornEffect <= 0 {
		return upgrade.StatLine{}
	}
	key := [2]int{wornEffect, wornLevel}
	if v, ok := wc.m[key]; ok {
		return v
	}
	var sl upgrade.StatLine
	if sp, err := wc.h.db.GetSpell(wornEffect); err == nil && sp != nil {
		var blk statBlock
		haste := parseWornEffect(sp, wornLevel, &blk)
		sl = upgrade.StatLine{
			HP: blk.HP, Mana: blk.Mana, AC: blk.AC,
			STR: blk.STR, STA: blk.STA, AGI: blk.AGI, DEX: blk.DEX,
			WIS: blk.WIS, INT: blk.INT, CHA: blk.CHA,
			MR: blk.MR, FR: blk.FR, CR: blk.CR, DR: blk.DR, PR: blk.PR,
			// A worn effect's mana regen is always Flowing Thought (SPA 15 →
			// blk.FT); parseWornEffect never sets blk.ManaRegen, so FT is the
			// item's whole contribution to the cap-scored mana-regen axis.
			Attack: blk.Attack, ManaRegen: blk.FT, Haste: haste,
		}
	}
	wc.m[key] = sl
	return sl
}

// addWorn merges a worn-effect contribution into a base (flat-column) stat line.
// Most fields add; Haste is best-of (worn haste doesn't stack), and the item's
// flat Damage/Delay are preserved (worn effects carry neither).
func addWorn(base, worn upgrade.StatLine) upgrade.StatLine {
	base.HP += worn.HP
	base.Mana += worn.Mana
	base.AC += worn.AC
	base.STR += worn.STR
	base.STA += worn.STA
	base.AGI += worn.AGI
	base.DEX += worn.DEX
	base.WIS += worn.WIS
	base.INT += worn.INT
	base.CHA += worn.CHA
	base.MR += worn.MR
	base.FR += worn.FR
	base.CR += worn.CR
	base.DR += worn.DR
	base.PR += worn.PR
	base.Attack += worn.Attack
	base.ManaRegen += worn.ManaRegen
	if worn.Haste > base.Haste {
		base.Haste = worn.Haste
	}
	return base
}

func (h *charactersHandler) candStatLine(c db.UpgradeCandidate, wc *wornCache) upgrade.StatLine {
	return addWorn(statLineFromCandidate(c), wc.contribution(c.WornEffect, c.WornLevel))
}

func (h *charactersHandler) itemStatLine(it *db.Item, wc *wornCache) upgrade.StatLine {
	return addWorn(statLineFromItem(it), wc.contribution(it.WornEffect, it.WornLevel))
}

// hasteByLocation returns the best worn melee-haste percent equipped in each
// worn location, so scoreSlot can derive the "other slots" haste a candidate
// must beat (worn haste is best-of-type).
func (h *charactersHandler) hasteByLocation(byLoc map[string][]zeal.InventoryEntry, worn map[int]*db.Item, wc *wornCache) map[string]int {
	out := make(map[string]int, len(byLoc))
	for loc, entries := range byLoc {
		best := 0
		for _, e := range entries {
			item := worn[e.ID]
			if item == nil {
				continue
			}
			if hv := wc.contribution(item.WornEffect, item.WornLevel).Haste; hv > best {
				best = hv
			}
		}
		out[loc] = best
	}
	return out
}

func statLineFromCandidate(c db.UpgradeCandidate) upgrade.StatLine {
	return upgrade.StatLine{
		HP: c.HP, Mana: c.Mana, AC: c.AC,
		STR: c.STR, STA: c.STA, AGI: c.AGI, DEX: c.DEX,
		WIS: c.WIS, INT: c.INT, CHA: c.CHA,
		MR: c.MR, FR: c.FR, CR: c.CR, DR: c.DR, PR: c.PR,
		Damage: c.Damage, Delay: c.Delay,
	}
}

func statLineFromItem(it *db.Item) upgrade.StatLine {
	return upgrade.StatLine{
		HP: it.HP, Mana: it.Mana, AC: it.AC,
		STR: it.Strength, STA: it.Stamina, AGI: it.Agility, DEX: it.Dexterity,
		WIS: it.Wisdom, INT: it.Intelligence, CHA: it.Charisma,
		MR: it.MagicResist, FR: it.FireResist, CR: it.ColdResist,
		DR: it.DiseaseResist, PR: it.PoisonResist,
		Damage: it.Damage, Delay: it.Delay,
	}
}
