package api

import (
	"encoding/json"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/jasonsoprovich/pq-companion/backend/internal/buffmod"
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

// twoHandedItemTypes are the items.itemtype values that are mechanically
// two-handed weapons (EQMacEmu common/item_data.h ItemType enum — see
// internal/db/enums/item_type.go). Verified against live quarm.db data:
// every 2H Blunt/Slashing/Piercing item has items.slots = Primary only (it
// can't be "placed" in Secondary at all), while ordinary 1H weapons — and
// Monk Hand to Hand (itemtype 45) — carry BOTH the Primary and Secondary
// bits, since they're equally valid in either hand. items.slots therefore
// can't distinguish 1H from 2H (an earlier version of this check assumed
// "both bits set" meant 2H, which is backwards — it was flagging ordinary
// dual-wieldable 1H/H2H weapons as 2H and vice versa); itemtype is the only
// reliable signal.
var twoHandedItemTypes = map[int]bool{
	1:  true, // 2H Slashing
	4:  true, // 2H Blunt
	35: true, // 2H Piercing
}

func isTwoHander(itemType int) bool {
	return twoHandedItemTypes[itemType]
}

// sumStatLine adds two equipped items' stat lines field-by-field, for the
// combined Primary+Secondary baseline a 2H weapon candidate is scored
// against (see scoreSlotCands). Damage/Delay are summed too — a rough
// stand-in for "total weapon value of both hands" rather than a precise
// dual-wield DPS model, but it's a closer baseline than ignoring the
// offhand weapon's ratio entirely. Haste is best-of, matching how worn
// haste already works for a single loadout (it doesn't stack).
func sumStatLine(a, b upgrade.StatLine) upgrade.StatLine {
	haste := a.Haste
	if b.Haste > haste {
		haste = b.Haste
	}
	return upgrade.StatLine{
		HP: a.HP + b.HP, Mana: a.Mana + b.Mana, AC: a.AC + b.AC,
		STR: a.STR + b.STR, STA: a.STA + b.STA, AGI: a.AGI + b.AGI, DEX: a.DEX + b.DEX,
		WIS: a.WIS + b.WIS, INT: a.INT + b.INT, CHA: a.CHA + b.CHA,
		MR: a.MR + b.MR, FR: a.FR + b.FR, CR: a.CR + b.CR, DR: a.DR + b.DR, PR: a.PR + b.PR,
		Damage: a.Damage + b.Damage, Delay: a.Delay + b.Delay,
		Attack: a.Attack + b.Attack, ManaRegen: a.ManaRegen + b.ManaRegen,
		Haste: haste,
		// Skill mods are best-of like haste; the weapon identity stays a's
		// (the Primary) — backstab reads Context.Primary, not this sum.
		BackstabMod: max(a.BackstabMod, b.BackstabMod),
		ItemType:    a.ItemType, RecLevel: a.RecLevel,
	}.WithMod2(sumMod2(a, b))
}

func sumMod2(a, b upgrade.StatLine) db.Mod2Stats {
	m := a.Mod2Of()
	m.Add(b.Mod2Of())
	return m
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
	// PriorityFocus is true when this candidate carries a focus the character
	// asked for (that rank or better) and doesn't already wear. Score stays the
	// plain stat score; priority items are also listed in the response's
	// separate priority array so they show even when they're a stat loss.
	PriorityFocus bool `json:"priority_focus"`
	// ReplacesSecondary is true when this candidate is a two-handed weapon
	// shown in the Primary slot — equipping it also unequips the current
	// Secondary item, so the score already nets out that item's stats.
	ReplacesSecondary bool `json:"replaces_secondary"`
	// SecondaryItemName is the offhand item ReplacesSecondary accounts for,
	// so the UI can explain the tradeoff ("also replaces: <name>").
	SecondaryItemName string `json:"secondary_item_name,omitempty"`
	// FromMold is the PoP armor mold this item is combined from. The item
	// itself never drops — the mold does, and it's what gets bid or rolled on.
	FromMold *db.MoldRef `json:"from_mold,omitempty"`
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
	// Priority is every candidate carrying a wanted focus (priorityFocusMatch),
	// sorted by stat score, with no limit and no score gate. Candidates is
	// unaffected by it.
	Priority       []upgradeResult `json:"priority"`
	Considered     int             `json:"considered"`
	HasCurrentGear bool            `json:"has_current_gear"`
	Caps           upgradeCaps     `json:"caps"`
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
	// PoP gear is hidden unless explicitly requested (independent of the app's
	// era display elsewhere, so the finder is predictable).
	excludePoP := !(r.URL.Query().Get("show_pop") == "1" || r.URL.Query().Get("show_pop") == "true")
	// Crafted (tradeskill-made) gear is hidden by default; ?hide_crafted=0 keeps it.
	excludeCrafted := r.URL.Query().Get("hide_crafted") != "0" && r.URL.Query().Get("hide_crafted") != "false"
	// NO DROP gear is shown by default; ?hide_nodrop=1 drops it.
	excludeNoDrop := r.URL.Query().Get("hide_nodrop") == "1" || r.URL.Query().Get("hide_nodrop") == "true"
	// focus_only keeps only items carrying a focus effect, across every
	// candidate rather than just the ones that made the ranked list.
	focusOnly := r.URL.Query().Get("focus_only") == "1" || r.URL.Query().Get("focus_only") == "true"
	// weapon_style narrows Primary-slot candidates to a hand preference: "dw"
	// (dual wield — 1H only, for classes that won't ever equip a 2H regardless
	// of stats) or "2h" (two-handed only). Empty/anything else means no filter.
	weaponStyle := r.URL.Query().Get("weapon_style")
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
	ctx := upgrade.Context{
		Level: char.Level, Current: statLineFromBlock(h.currentTotals(char)),
		StatCapMod: h.statCapMods(char), BackstabSkill: h.backstabSkill(char),
	}

	byLoc, hasGear := h.loadEquipped(cfg.EQPath, char.Name)
	worn := h.resolveWornItems(byLoc)
	pf := h.newPriorityFocus(char, worn)
	wornLore := h.equippedLoreSet(worn)
	wc := h.newWornCache()
	hasteByLoc := h.hasteByLocation(byLoc, worn, wc)
	filter := db.CandidateFilter{
		ExcludePoP:     excludePoP,     // finder-local "Show PoP gear" toggle (default hide)
		ExcludeCrafted: excludeCrafted, // finder-local "Hide crafted gear" toggle (default hide)
		ExcludeNoDrop:  excludeNoDrop,  // finder-local "Hide NO DROP" toggle (default show)
		FocusOnly:      focusOnly,
	}
	current, baselineID, results, priority, considered, err := h.scoreSlot(char, ctx, weights, slot, byLoc, worn, showAll, filter, weaponStyle, limit, pf, wornLore, wc, hasteByLoc)
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
		Priority:       priority,
		Considered:     considered,
		HasCurrentGear: hasGear,
		Caps:           buildUpgradeCaps(ctx, hasteByLoc),
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
	HasGear bool                `json:"has_gear"`
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
	// PriorityBest is the top-scoring wanted-focus item for the slot, if any.
	// It never replaces Best, which stays the best stat pick.
	PriorityBest *upgradeResult `json:"priority_best,omitempty"`
	Considered   int            `json:"considered"`
}

type overviewResponse struct {
	Class          int             `json:"class"`
	Level          int             `json:"level"`
	Weights        upgrade.Weights `json:"weights"`
	Slots          []overviewSlot  `json:"slots"`
	HasCurrentGear bool            `json:"has_current_gear"`
	Caps           upgradeCaps     `json:"caps"`
}

// upgradeSweep is the per-request state shared by the all-slots views
// (overview, by-zone): the character, scoring context, and the parsed loadout,
// computed once and reused across every slot.
type upgradeSweep struct {
	char           character.Character
	ctx            upgrade.Context
	weights        upgrade.Weights
	byLoc          map[string][]zeal.InventoryEntry
	worn           map[int]*db.Item
	hasGear        bool
	priority       *priorityFocus
	wornLore       map[int]bool
	wc             *wornCache
	hasteByLoc     map[string]int
	excludePoP     bool
	excludeCrafted bool
	excludeNoDrop  bool
	focusOnly      bool
	weaponStyle    string
}

// loadUpgradeSweep resolves the character and shared scoring state for an
// all-slots request. On failure it writes the error response and returns false.
func (h *charactersHandler) loadUpgradeSweep(w http.ResponseWriter, r *http.Request) (*upgradeSweep, bool) {
	id, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid id")
		return nil, false
	}
	char, ok, err := h.store.Get(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return nil, false
	}
	if !ok {
		writeError(w, http.StatusNotFound, "character not found")
		return nil, false
	}
	cfg := h.mgr.Get()
	if cfg.EQPath == "" {
		writeError(w, http.StatusBadRequest, "eq_path not configured")
		return nil, false
	}

	weights := h.resolveWeights(char)
	if raw := r.URL.Query().Get("weights"); raw != "" {
		var override upgrade.Weights
		if err := json.Unmarshal([]byte(raw), &override); err == nil {
			weights = override
		}
	}
	q := r.URL.Query()
	sw := &upgradeSweep{
		char:    char,
		weights: weights,
		ctx: upgrade.Context{
			Level: char.Level, Current: statLineFromBlock(h.currentTotals(char)),
			StatCapMod: h.statCapMods(char), BackstabSkill: h.backstabSkill(char),
		},
		excludePoP:     !(q.Get("show_pop") == "1" || q.Get("show_pop") == "true"),
		excludeCrafted: q.Get("hide_crafted") != "0" && q.Get("hide_crafted") != "false",
		excludeNoDrop:  q.Get("hide_nodrop") == "1" || q.Get("hide_nodrop") == "true",
		focusOnly:      q.Get("focus_only") == "1" || q.Get("focus_only") == "true",
		weaponStyle:    q.Get("weapon_style"),
	}
	sw.byLoc, sw.hasGear = h.loadEquipped(cfg.EQPath, char.Name)
	sw.worn = h.resolveWornItems(sw.byLoc)
	sw.priority = h.newPriorityFocus(char, sw.worn)
	sw.wornLore = h.equippedLoreSet(sw.worn)
	sw.wc = h.newWornCache()
	sw.hasteByLoc = h.hasteByLocation(sw.byLoc, sw.worn, sw.wc)
	return sw, true
}

// sweepCandidates runs one candidate scan for the whole sweep: the class/race/level/
// hidden/variant filter is identical across all slots — only the slot mask
// differs — so query the union of every slot mask once and let the caller
// partition in Go by slot, instead of one full-table item scan per slot.
func (h *charactersHandler) sweepCandidates(sw *upgradeSweep) ([]db.UpgradeCandidate, error) {
	combinedMask := 0
	for _, s := range upgradeSlots {
		combinedMask |= s.Mask
	}
	return h.db.UpgradeCandidates(db.CandidateFilter{
		SlotMask:       combinedMask,
		ClassBit:       charClassBit(sw.char),
		RaceBit:        enums.RaceBitForCharRace(sw.char.Race),
		MaxLevel:       upgradeMaxLevel(sw.char.Level, sw.excludePoP),
		ExcludePoP:     sw.excludePoP,
		ExcludeCrafted: sw.excludeCrafted,
		ExcludeNoDrop:  sw.excludeNoDrop,
		FocusOnly:      sw.focusOnly,
	})
}

// scoreSweepSlot scores the sweep's candidates that fit one slot.
func (h *charactersHandler) scoreSweepSlot(sw *upgradeSweep, s upgradeSlot, cands []db.UpgradeCandidate, limit int) (current []upgradeCurrentItem, results, priority []upgradeResult, considered int) {
	var slotCands []db.UpgradeCandidate
	for _, c := range cands {
		if c.Slots&s.Mask != 0 {
			slotCands = append(slotCands, c)
		}
	}
	current, _, results, priority, considered = h.scoreSlotCands(sw.char, sw.ctx, sw.weights, s, sw.byLoc, sw.worn, false, sw.weaponStyle, limit, sw.priority, sw.wornLore, sw.wc, sw.hasteByLoc, slotCands)
	return current, results, priority, considered
}

// upgradesOverview handles GET /api/characters/{id}/upgrades/overview — the
// single best upgrade per slot. It parses the Quarmy export and computes the
// character's totals once, then scores every slot, so the whole sweep is one
// request instead of ~19 round trips.
func (h *charactersHandler) upgradesOverview(w http.ResponseWriter, r *http.Request) {
	sw, ok := h.loadUpgradeSweep(w, r)
	if !ok {
		return
	}
	allCands, err := h.sweepCandidates(sw)
	if err != nil {
		// A locked/corrupt quarm.db must surface as an error, not an empty
		// overview that reads as "everything is best-in-slot".
		writeError(w, http.StatusInternalServerError, "failed to load upgrade candidates")
		return
	}

	slots := make([]overviewSlot, 0, len(upgradeSlots))
	for _, s := range upgradeSlots {
		current, results, priority, considered := h.scoreSweepSlot(sw, s, allCands, 1)
		var best, priorityBest *upgradeResult
		if len(results) > 0 {
			best = &results[0]
		}
		if len(priority) > 0 {
			priorityBest = &priority[0]
		}
		slots = append(slots, overviewSlot{
			Slot:         s.Key,
			SlotLabel:    s.Label,
			CurrentItems: current,
			Best:         best,
			PriorityBest: priorityBest,
			Considered:   considered,
		})
	}

	writeJSON(w, http.StatusOK, overviewResponse{
		Class:          sw.char.Class,
		Level:          sw.char.Level,
		Weights:        sw.weights,
		Slots:          slots,
		HasCurrentGear: sw.hasGear,
		Caps:           buildUpgradeCaps(sw.ctx, sw.hasteByLoc),
	})
}

// zoneUpgradeResult is a scored candidate plus where it drops in the zone.
type zoneUpgradeResult struct {
	upgradeResult
	Sources []db.ZoneItemSource `json:"sources"`
	// ViaMold is true when Sources are the drops of the item's mold (see
	// FromMold) rather than of the item itself.
	ViaMold bool `json:"via_mold,omitempty"`
}

// zoneUpgradeSlot is one slot's upgrades available from a zone.
type zoneUpgradeSlot struct {
	Slot         string               `json:"slot"`
	SlotLabel    string               `json:"slot_label"`
	CurrentItems []upgradeCurrentItem `json:"current_items"`
	Candidates   []zoneUpgradeResult  `json:"candidates"`
	// Priority lists the zone's wanted-focus items for the slot (see
	// upgradesResponse.Priority).
	Priority   []zoneUpgradeResult `json:"priority"`
	Considered int                 `json:"considered"`
}

type zoneUpgradesResponse struct {
	Zone           string            `json:"zone"`
	ZoneName       string            `json:"zone_name"`
	Class          int               `json:"class"`
	Level          int               `json:"level"`
	Weights        upgrade.Weights   `json:"weights"`
	Slots          []zoneUpgradeSlot `json:"slots"`
	ZoneItems      int               `json:"zone_items"`
	HasCurrentGear bool              `json:"has_current_gear"`
}

// zoneCandidates keeps the candidates that drop in the zone. A PoP mold
// product never drops itself; when its mold drops here, the mold's NPCs are
// written into sources as the product's and the product is flagged in viaMold.
func (h *charactersHandler) zoneCandidates(all []db.UpgradeCandidate, sources map[int][]db.ZoneItemSource) ([]db.UpgradeCandidate, map[int]bool) {
	viaMold := map[int]bool{}
	out := make([]db.UpgradeCandidate, 0)
	for _, c := range all {
		if len(sources[c.ID]) == 0 {
			m, ok := h.db.ProductMold(c.ID)
			if !ok || len(sources[m.ID]) == 0 {
				continue
			}
			sources[c.ID] = sources[m.ID]
			viaMold[c.ID] = true
		}
		out = append(out, c)
	}
	return out, viaMold
}

// upgradesZone handles GET /api/characters/{id}/upgrades/zone?zone=<short>&limit=
// — every upgrade per slot that drops in one zone, ranked by score, each with
// the NPCs that drop it. Built for walking into a raid or hunting zone and
// seeing what's worth killing, bidding, or rolling on. Takes the overview's
// filter params except show_pop; limit caps results per slot (default 25).
func (h *charactersHandler) upgradesZone(w http.ResponseWriter, r *http.Request) {
	zoneShort := strings.TrimSpace(r.URL.Query().Get("zone"))
	if zoneShort == "" {
		writeError(w, http.StatusBadRequest, "missing zone")
		return
	}
	zone, err := h.db.GetZoneByShortName(zoneShort)
	if err != nil || zone == nil {
		writeError(w, http.StatusNotFound, "zone not found")
		return
	}
	sw, ok := h.loadUpgradeSweep(w, r)
	if !ok {
		return
	}
	// The zone itself decides what's obtainable here, so the PoP-gear toggle
	// doesn't apply (the UI hides it in this view): never PoP-gate zone drops.
	sw.excludePoP = false
	limit := 25
	if v, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && v > 0 && v <= 200 {
		limit = v
	}

	sources, err := h.db.ZoneItemSources(zone.ShortName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load zone drops")
		return
	}
	allCands, err := h.sweepCandidates(sw)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load upgrade candidates")
		return
	}
	zoneCands, viaMold := h.zoneCandidates(allCands, sources)

	slots := make([]zoneUpgradeSlot, 0, len(upgradeSlots))
	for _, s := range upgradeSlots {
		current, results, priority, considered := h.scoreSweepSlot(sw, s, zoneCands, limit)
		withSources := func(rs []upgradeResult) []zoneUpgradeResult {
			out := make([]zoneUpgradeResult, 0, len(rs))
			for _, res := range rs {
				out = append(out, zoneUpgradeResult{
					upgradeResult: res, Sources: sources[res.ID], ViaMold: viaMold[res.ID],
				})
			}
			return out
		}
		slots = append(slots, zoneUpgradeSlot{
			Slot:         s.Key,
			SlotLabel:    s.Label,
			CurrentItems: current,
			Candidates:   withSources(results),
			Priority:     withSources(priority),
			Considered:   considered,
		})
	}

	writeJSON(w, http.StatusOK, zoneUpgradesResponse{
		Zone:           zone.ShortName,
		ZoneName:       zone.LongName,
		Class:          sw.char.Class,
		Level:          sw.char.Level,
		Weights:        sw.weights,
		Slots:          slots,
		ZoneItems:      len(zoneCands),
		HasCurrentGear: sw.hasGear,
	})
}

type itemUpgradeScore struct {
	Score float64 `json:"score"`
	Slot  string  `json:"slot_label"`
}

// itemPriorityFocus flags an item carrying a wanted focus, with its signed
// stat score (it can be a stat loss) and the slot that score is for.
type itemPriorityFocus struct {
	Score     float64 `json:"score"`
	Slot      string  `json:"slot_label"`
	FocusName string  `json:"focus_name"`
}

// upgradesItems handles GET /api/characters/{id}/upgrades/items?ids=1,2,3 —
// the Gear Upgrade Finder score for specific items (an NPC's loot table, the
// Item Info overlay), keyed by item id. scores holds only actual upgrades
// (score > 0); priority holds items carrying a wanted focus whatever their
// score. An item fitting several slots reports its best one in each. Like the zone
// view, PoP and crafted gear are never filtered — the caller named the items.
func (h *charactersHandler) upgradesItems(w http.ResponseWriter, r *http.Request) {
	var ids []int
	for _, part := range strings.Split(r.URL.Query().Get("ids"), ",") {
		if v, err := strconv.Atoi(strings.TrimSpace(part)); err == nil && v > 0 {
			ids = append(ids, v)
		}
	}
	if len(ids) == 0 {
		writeJSON(w, http.StatusOK, map[string]any{
			"scores": map[int]itemUpgradeScore{}, "priority": map[int]itemPriorityFocus{},
		})
		return
	}
	if len(ids) > 500 {
		ids = ids[:500]
	}
	sw, ok := h.loadUpgradeSweep(w, r)
	if !ok {
		return
	}
	sw.excludePoP = false
	sw.excludeCrafted = false
	combinedMask := 0
	for _, s := range upgradeSlots {
		combinedMask |= s.Mask
	}
	cands, err := h.db.UpgradeCandidates(db.CandidateFilter{
		SlotMask: combinedMask,
		ClassBit: charClassBit(sw.char),
		RaceBit:  enums.RaceBitForCharRace(sw.char.Race),
		MaxLevel: upgradeMaxLevel(sw.char.Level, false),
		ItemIDs:  ids,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load upgrade candidates")
		return
	}
	scores := make(map[int]itemUpgradeScore)
	priority := make(map[int]itemPriorityFocus)
	if len(cands) > 0 {
		for _, s := range upgradeSlots {
			_, results, prio, _ := h.scoreSweepSlot(sw, s, cands, len(cands))
			for _, res := range results {
				if res.Score > 0 && res.Score > scores[res.ID].Score {
					scores[res.ID] = itemUpgradeScore{Score: res.Score, Slot: s.Label}
				}
			}
			mergeItemPriority(priority, prio, s.Label)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"scores": scores, "priority": priority})
}

// mergeItemPriority folds one slot's wanted-focus results into the per-item
// priority map, keeping each item's best slot. Scores are signed: a wanted
// focus on a stat downgrade is still reported.
func mergeItemPriority(into map[int]itemPriorityFocus, prio []upgradeResult, slotLabel string) {
	for _, res := range prio {
		if cur, ok := into[res.ID]; !ok || res.Score > cur.Score {
			into[res.ID] = itemPriorityFocus{Score: res.Score, Slot: slotLabel, FocusName: res.FocusName}
		}
	}
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
	slot upgradeSlot, byLoc map[string][]zeal.InventoryEntry, worn map[int]*db.Item, showAll bool, filter db.CandidateFilter, weaponStyle string, limit int,
	pf *priorityFocus, wornLore map[int]bool, wc *wornCache, hasteByLoc map[string]int,
) (current []upgradeCurrentItem, baselineID int, results, priority []upgradeResult, considered int, err error) {
	// filter carries the toggles (PoP / crafted / NO DROP / focus only); the
	// character's slot, class, race and level are filled in here.
	filter.SlotMask = slot.Mask
	filter.ClassBit = charClassBit(char)
	filter.RaceBit = enums.RaceBitForCharRace(char.Race)
	filter.MaxLevel = upgradeMaxLevel(char.Level, filter.ExcludePoP)
	cands, err := h.db.UpgradeCandidates(filter)
	if err != nil {
		// Propagate: a locked/corrupt quarm.db must not be reported to the user
		// as "no upgrades" (indistinguishable from best-in-slot).
		return nil, 0, nil, nil, 0, err
	}
	current, baselineID, results, priority, considered = h.scoreSlotCands(char, ctx, weights, slot, byLoc, worn, showAll, weaponStyle, limit, pf, wornLore, wc, hasteByLoc, cands)
	return current, baselineID, results, priority, considered, nil
}

// scoreSlotCands scores a pre-fetched candidate set for one slot against the
// item worn in that physical slot (slot.Index picks Ear 1 vs Ear 2, etc.). It
// takes a pre-parsed equipped map so a multi-slot sweep parses
// the Quarmy export only once. Returns the worn items, the baseline item id, the
// ranked results (truncated to limit), the wanted-focus items (pf; never
// truncated or score-gated, and never taking a ranked row), and how many
// candidates were considered.
func (h *charactersHandler) scoreSlotCands(
	char character.Character, ctx upgrade.Context, weights upgrade.Weights,
	slot upgradeSlot, byLoc map[string][]zeal.InventoryEntry, worn map[int]*db.Item, showAll bool, weaponStyle string, limit int,
	pf *priorityFocus, wornLore map[int]bool, wc *wornCache, hasteByLoc map[string]int,
	cands []db.UpgradeCandidate,
) (current []upgradeCurrentItem, baselineID int, results, priority []upgradeResult, considered int) {
	// Worn haste is best-of-type: a candidate only gains haste if it beats the
	// best the character already wears in OTHER slots (toward the level cap).
	otherHaste := 0
	for loc, hv := range hasteByLoc {
		if loc != slot.Location && hv > otherHaste {
			otherHaste = hv
		}
	}
	ctx.OtherHaste = otherHaste

	// Backstab: the actual worn Primary (for every slot, never a summed
	// baseline) and the best Backstab skill mod outside this slot. A 2H
	// Primary candidate also displaces the offhand, so it gets the best mod
	// outside both hands (otherBSModNoOffhand, applied per candidate below).
	var otherBSModNoOffhand int
	if ctx.BackstabSkill > 0 {
		secLoc := ""
		if secSlot, ok := upgradeSlotByKey("secondary"); ok {
			secLoc = secSlot.Location
		}
		ctx.OtherBackstabMod, otherBSModNoOffhand = 0, 0
		for loc, m := range h.backstabModByLocation(byLoc, worn) {
			if loc == slot.Location {
				continue
			}
			ctx.OtherBackstabMod = max(ctx.OtherBackstabMod, m)
			if loc != secLoc {
				otherBSModNoOffhand = max(otherBSModNoOffhand, m)
			}
		}
		ctx.Primary = upgrade.StatLine{}
		if priSlot, ok := upgradeSlotByKey("primary"); ok {
			priWorn := h.equippedItemsForSlot(byLoc, priSlot, wc, worn)
			if priSlot.Index < len(priWorn) {
				ctx.Primary = priWorn[priSlot.Index].Stats
			}
		}
		ctx.SlotIsPrimary = slot.Key == "primary"
	}

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

	// A 2H weapon candidate in the Primary slot also displaces the equipped
	// Secondary item (a 2H blocks dual wield/shield), so it must be scored
	// against a baseline of BOTH currently-worn items — otherwise the finder
	// credits the 2H for the offhand's stats it's actually taking away. Only
	// resolved when scoring Primary; Secondary candidates that are 2H are
	// skipped below (a 2H can't be equipped "into" just the offhand).
	var offhand *upgradeCurrentItem
	if slot.Key == "primary" {
		if secSlot, ok := upgradeSlotByKey("secondary"); ok {
			secWorn := h.equippedItemsForSlot(byLoc, secSlot, wc, worn)
			if secSlot.Index < len(secWorn) {
				oh := secWorn[secSlot.Index]
				offhand = &oh
			}
		}
	}

	considered = len(cands)

	// Exclude every item worn at this Location (both paired slots), so the other
	// earring/ring you already wear is never suggested as an upgrade. Canonicalize:
	// a worn item's id is whatever the live server assigned it, which can be a
	// non-canonical duplicate row (see db.CanonicalItemID), while candidates are
	// always the canonical row for their name — comparing raw ids would miss the
	// match and re-suggest an item already worn under a sibling id.
	wornHere := make(map[int]bool, len(allWorn))
	for _, ci := range allWorn {
		wornHere[h.db.CanonicalItemID(ci.ID)] = true
	}
	results = make([]upgradeResult, 0, len(cands))
	priority = make([]upgradeResult, 0)
	for _, c := range cands {
		if wornHere[c.ID] {
			continue // don't suggest what's already equipped in this slot
		}
		if wornLore[c.ID] {
			continue // a LORE item worn elsewhere can't be acquired a second time
		}
		twoHander := isTwoHander(c.ItemType)
		if slot.Key == "secondary" && twoHander {
			continue // occupies both hands; only ever offered under Primary
		}
		if slot.Key == "primary" {
			// Hand-style preference: some classes (rogue/monk dual wielders,
			// warrior/SK 2H users) will never equip the other style regardless
			// of stats, so let them exclude it rather than relying on the score
			// alone. Irrelevant elsewhere — Secondary already excludes 2H above,
			// and Range/other slots have no hand-style concept.
			if weaponStyle == "dw" && twoHander {
				continue
			}
			if weaponStyle == "2h" && !twoHander {
				continue
			}
		}
		slotCur := baseline
		replacesSecondary := false
		secondaryName := ""
		if twoHander && offhand != nil {
			slotCur = sumStatLine(baseline, offhand.Stats)
			replacesSecondary = true
			secondaryName = offhand.Name
		}
		cctx := ctx
		if replacesSecondary {
			cctx.OtherBackstabMod = otherBSModNoOffhand
		}
		res := upgrade.Score(cctx, weights, slotCur, h.candStatLine(c, wc))
		score := res.Score
		// A wanted focus (that rank or better, not already worn) is listed in
		// priority whatever its stat score; the score itself stays honest.
		wanted := pf.match(c.FocusEffect)
		if !wanted && !showAll && score <= 0 {
			continue
		}
		r := upgradeResult{
			ID:                c.ID,
			Name:              c.Name,
			Icon:              c.Icon,
			Slots:             c.Slots,
			NoDrop:            c.NoDrop,
			ReqLevel:          c.ReqLevel,
			RecLevel:          c.RecLevel,
			FocusEffect:       c.FocusEffect,
			FocusName:         c.FocusName,
			ClickEffect:       c.ClickEffect,
			ClickName:         c.ClickName,
			ProcEffect:        c.ProcEffect,
			ProcName:          c.ProcName,
			Score:             score,
			Deltas:            res.Deltas,
			PriorityFocus:     wanted,
			ReplacesSecondary: replacesSecondary,
			SecondaryItemName: secondaryName,
		}
		if h.db != nil {
			if m, ok := h.db.ProductMold(c.ID); ok {
				r.FromMold = &m
			}
		}
		if wanted {
			priority = append(priority, r)
		}
		if showAll || score > 0 {
			results = append(results, r)
		}
	}
	byScore := func(rs []upgradeResult) {
		sort.SliceStable(rs, func(i, j int) bool { return rs[i].Score > rs[j].Score })
	}
	byScore(results)
	byScore(priority)
	if len(results) > limit {
		results = results[:limit]
	}
	return current, baselineID, results, priority, considered
}

// resolveWeights returns the character's saved upgrade weights, falling back to
// the class default preset when none are saved or the stored JSON is corrupt.
func (h *charactersHandler) resolveWeights(char character.Character) upgrade.Weights {
	def := upgrade.DefaultWeights(char.Class)
	if raw, ok, err := h.store.GetUpgradeWeights(char.ID); err == nil && ok {
		var wts upgrade.Weights
		if json.Unmarshal([]byte(raw), &wts) == nil {
			// Backfill weights added after this profile was saved, so an older
			// saved set picks up DPS/ATK/haste scoring instead of silently using 0
			// (JSON can't distinguish a missing key from an intentional 0).
			if !strings.Contains(raw, `"dps"`) {
				wts.DPS = def.DPS
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
			backfillNewWeights(raw, &wts, def)
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

// priorityFocus is the per-request state for matching candidates against the
// character's wanted foci: the picked spells, the focus spells their worn items
// carry, and a memoized spell → modifier lookup. A nil *priorityFocus matches
// nothing.
type priorityFocus struct {
	selected []int
	worn     []int
	top      int
	lookup   focusLookup
}

// newPriorityFocus loads the character's picked foci. Worn foci come from the
// equipped items only (AA foci stack on top, so they don't make an item focus
// redundant).
func (h *charactersHandler) newPriorityFocus(char character.Character, worn map[int]*db.Item) *priorityFocus {
	pf := &priorityFocus{top: focusTopLevel(char), lookup: h.newFocusLookup()}
	if ids, err := h.store.GetPriorityFocus(char.ID); err == nil {
		pf.selected = ids
	}
	seen := map[int]bool{}
	for _, item := range worn {
		if item.FocusEffect > 0 && !seen[item.FocusEffect] {
			seen[item.FocusEffect] = true
			pf.worn = append(pf.worn, item.FocusEffect)
		}
	}
	return pf
}

// match reports whether a candidate's focus spell is a wanted one the
// character doesn't already have (see priorityFocusMatch).
func (pf *priorityFocus) match(focusSpell int) bool {
	if pf == nil || len(pf.selected) == 0 {
		return false
	}
	return priorityFocusMatch(focusSpell, pf.selected, pf.worn, pf.lookup, pf.top)
}

// newFocusLookup returns a memoized focus spell → primary categorised modifier
// resolver. "Primary" is the first categorised effect, the same rule
// focusCandidates uses to file an item under a category.
func (h *charactersHandler) newFocusLookup() focusLookup {
	type entry struct {
		m  buffmod.Modifier
		ok bool
	}
	cache := map[int]entry{}
	return func(spellID int) (buffmod.Modifier, bool) {
		if e, hit := cache[spellID]; hit {
			return e.m, e.ok
		}
		var e entry
		if sp, err := h.db.GetSpell(spellID); err == nil && sp != nil {
			for _, m := range buffmod.FocusModifiers(sp) {
				if m.Category != "" {
					e = entry{m: m, ok: true}
					break
				}
			}
		}
		cache[spellID] = e
		return e.m, e.ok
	}
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

// equippedLoreSet returns the canonical IDs of LORE items the character
// already wears in any slot. A LORE item is unique — you can only possess
// one — so once it's equipped it must never be offered as an upgrade for a
// different slot (you can't acquire a second). EQ encodes LORE as a lore
// string beginning with '*'. Keyed by db.CanonicalItemID rather than the raw
// worn id: candidates are always a name group's canonical row, so a worn item
// resolved to a non-canonical sibling id would otherwise fail to match and
// get re-suggested. (Same-slot duplicates are already filtered by the
// per-slot worn check, so this only matters for multi-slot items like a
// charm/torch that also fit elsewhere.)
func (h *charactersHandler) equippedLoreSet(worn map[int]*db.Item) map[int]bool {
	set := map[int]bool{}
	for id, item := range worn {
		if strings.HasPrefix(item.Lore, "*") {
			set[h.db.CanonicalItemID(id)] = true
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
	// Match the finder's level ceiling: with Show PoP gear on, a pre-65
	// character browses level-65 Planes gear, so its foci belong in the picker.
	showPoP := r.URL.Query().Get("show_pop") == "1" || r.URL.Query().Get("show_pop") == "true"
	opts, err := h.db.FocusOptions(charClassBit(char), upgradeMaxLevel(char.Level, !showPoP))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Tag each option with its focus type so the picker can group ranks
	// together, and flag the ones the character already wears.
	wornFocus := map[int]bool{}
	cfg := h.mgr.Get()
	if cfg.EQPath != "" {
		if byLoc, ok := h.loadEquipped(cfg.EQPath, char.Name); ok {
			for _, item := range h.resolveWornItems(byLoc) {
				if item.FocusEffect > 0 {
					wornFocus[item.FocusEffect] = true
				}
			}
		}
	}
	labels := map[string]string{}
	for _, c := range buffmod.Categories {
		labels[c.ID] = c.Label
	}
	lookup := h.newFocusLookup()
	out := make([]focusOptionView, 0, len(opts))
	for _, o := range opts {
		v := focusOptionView{FocusOption: o, Worn: wornFocus[o.SpellID]}
		if m, ok := lookup(o.SpellID); ok {
			v.Category, v.CategoryLabel, v.Percent = m.Category, labels[m.Category], m.Percent
		}
		out = append(out, v)
	}
	writeJSON(w, http.StatusOK, out)
}

// focusOptionView is a priority-picker option plus its focus type (empty for
// a one-off focus outside the known categories) and whether it's worn.
type focusOptionView struct {
	db.FocusOption
	Category      string `json:"category"`
	CategoryLabel string `json:"category_label"`
	Percent       int    `json:"percent"`
	Worn          bool   `json:"worn"`
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

// statCapMods returns the per-attribute cap raises from the character's trained
// SE_RaiseStatCap AAs (Planar Power, Innate Enlightenment), as a StatLine whose
// attribute fields hold the extra cap.
func (h *charactersHandler) statCapMods(char character.Character) upgrade.StatLine {
	trained, err := h.store.ListAAs(char.ID)
	if err != nil {
		return upgrade.StatLine{}
	}
	conv := make([]db.TrainedAA, 0, len(trained))
	for _, t := range trained {
		conv = append(conv, db.TrainedAA{AAID: t.AAID, Rank: t.Rank})
	}
	aa, _ := h.db.AAStatBonuses(conv)
	c := aa.StatCap
	return upgrade.StatLine{STR: c.STR, STA: c.STA, AGI: c.AGI, DEX: c.DEX, WIS: c.WIS, INT: c.INT, CHA: c.CHA}
}

// loadEquipped parses the character's Quarmy export once into a location →
// worn-entries map. ok is false when no export exists, so callers can disable
// current-item comparison. Dual slots (Ear/Wrist/Fingers) collect under one
// canonical location, so a location may hold two entries.
//
// Only worn slots (equipSlots) are kept. The export also lists bags, bank and
// shared bank, and every caller treats this map as "what's equipped": worn
// haste, worn foci, and worn LORE items would otherwise count items sitting in
// a bag or the bank.
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
		if e.ID > 0 && equipSlots[e.Location] {
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

// statLineFromBlock builds the cap-headroom reference from a derived stat block.
// The attributes and Flowing Thought come from the RAW (uncapped) totals:
// upgrade.Score clamps both sides of a swap to the cap itself, and feeding it
// already-capped totals made an over-cap stat look like a real loss when the
// worn item was removed (raw WIS 300, cap 280, worn +20 → "−20 effective",
// though the character stays capped at 280). Resists have no raw total; their
// 500 cap is out of reach in practice.
//
// Attack is the gear's worn ATK only: the 250 soft cap applies to worn ATK, so
// AA (and buff) attack mustn't use up that headroom.
func statLineFromBlock(b statBlock) upgrade.StatLine {
	return upgrade.StatLine{
		HP: b.HP, Mana: b.Mana, AC: b.AC,
		STR: b.STRRaw, STA: b.STARaw, AGI: b.AGIRaw, DEX: b.DEXRaw,
		WIS: b.WISRaw, INT: b.INTRaw, CHA: b.CHARaw,
		MR: b.MR, FR: b.FR, CR: b.CR, DR: b.DR, PR: b.PR,
		// Only worn FT counts against the item FT cap. b.ManaRegen (AA/buff flat
		// regen) is uncapped and never item-sourced, so folding it in would
		// falsely report the 15 cap as reached and hide real FT upgrades.
		Attack: b.Breakdown.Attack.Item, ManaRegen: b.FTRaw,
	}.WithMod2(b.Mod2Raw) // raw: Score clamps both sides to each cap
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
		BackstabMod: backstabMod(c.SkillModType, c.SkillModValue),
		ItemType:    c.ItemType, RecLevel: c.RecLevel,
	}.WithMod2(c.Mod2Stats)
}

func statLineFromItem(it *db.Item) upgrade.StatLine {
	return upgrade.StatLine{
		HP: it.HP, Mana: it.Mana, AC: it.AC,
		STR: it.Strength, STA: it.Stamina, AGI: it.Agility, DEX: it.Dexterity,
		WIS: it.Wisdom, INT: it.Intelligence, CHA: it.Charisma,
		MR: it.MagicResist, FR: it.FireResist, CR: it.ColdResist,
		DR: it.DiseaseResist, PR: it.PoisonResist,
		Damage: it.Damage, Delay: it.Delay,
		BackstabMod: backstabMod(it.SkillModType, it.SkillModValue),
		ItemType:    it.ItemType, RecLevel: it.RecLevel,
	}.WithMod2(it.Mod2Stats)
}

// backstabMod returns an item's Backstab skill mod percent, 0 for any other
// skill or a negative mod (the server only applies positive skill mods).
func backstabMod(skillType, value int) int {
	if skillType != upgrade.SkillBackstab || value <= 0 {
		return 0
	}
	return value
}

// backstabModByLocation returns the best Backstab skill mod worn at each
// location (skill mods are best-of, like worn haste).
func (h *charactersHandler) backstabModByLocation(byLoc map[string][]zeal.InventoryEntry, worn map[int]*db.Item) map[string]int {
	out := make(map[string]int, len(byLoc))
	for loc, entries := range byLoc {
		for _, e := range entries {
			if item := worn[e.ID]; item != nil {
				out[loc] = max(out[loc], backstabMod(item.SkillModType, item.SkillModValue))
			}
		}
	}
	return out
}

// backstabSkill is the character's Backstab skill for scoring: the class
// cap at its level (225 for a rogue at 60–65, the skill nearly every
// endgame rogue has maxed). 0 for classes without Backstab, which skips the
// backstab term entirely.
func (h *charactersHandler) backstabSkill(char character.Character) int {
	if h.db == nil || char.Level <= 0 {
		return 0
	}
	c, err := h.db.SkillCap(upgrade.SkillBackstab, char.Class+1, char.Level)
	if err != nil {
		return 0
	}
	return c
}

// backfillNewWeights fills weights added with the PoP item stats and
// backstab from the class default when an older saved profile lacks the key
// (JSON can't tell a missing key from an intentional 0).
func backfillNewWeights(raw string, w *upgrade.Weights, def upgrade.Weights) {
	fields := []struct {
		key string
		dst *float64
		src float64
	}{
		{"accuracy", &w.Accuracy, def.Accuracy},
		{"avoidance", &w.Avoidance, def.Avoidance},
		{"strikethrough", &w.Strikethrough, def.Strikethrough},
		{"stun_resist", &w.StunResist, def.StunResist},
		{"combat_effects", &w.CombatEffects, def.CombatEffects},
		{"spell_shield", &w.SpellShield, def.SpellShield},
		{"shielding", &w.Shielding, def.Shielding},
		{"damage_shield", &w.DamageShield, def.DamageShield},
		{"backstab", &w.Backstab, def.Backstab},
	}
	for _, f := range fields {
		if !strings.Contains(raw, `"`+f.key+`"`) {
			*f.dst = f.src
		}
	}
}
