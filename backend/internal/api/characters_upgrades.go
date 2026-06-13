package api

import (
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

	weights := upgrade.DefaultWeights(char.Class)
	ctx := upgrade.Context{Level: char.Level, Current: statLineFromBlock(h.currentTotals(char))}

	// Items currently worn in this slot (0, 1, or 2 for dual slots).
	currentItems, hasGear := h.equippedInSlot(cfg.EQPath, char.Name, slot)

	// Baseline = the worn item we'd actually replace: the lowest-scoring one in
	// the slot (you upgrade your weakest ring first). Empty when slot is bare.
	baseline := upgrade.StatLine{}
	baselineID := 0
	if len(currentItems) > 0 {
		worst := currentItems[0]
		worstScore := upgrade.Score(ctx, weights, upgrade.StatLine{}, worst.Stats).Score
		for _, ci := range currentItems[1:] {
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
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	worn := make(map[int]bool, len(currentItems))
	for _, ci := range currentItems {
		worn[ci.ID] = true
	}

	results := make([]upgradeResult, 0, len(cands))
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

	writeJSON(w, http.StatusOK, upgradesResponse{
		Slot:           slot.Key,
		SlotLabel:      slot.Label,
		Class:          char.Class,
		Level:          char.Level,
		Weights:        weights,
		CurrentItems:   currentItems,
		BaselineItemID: baselineID,
		Candidates:     results,
		Considered:     len(cands),
		HasCurrentGear: hasGear,
	})
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

// equippedInSlot returns the items currently worn in a logical slot, parsed
// from the character's Quarmy export. hasGear is false when no export exists,
// so callers can disable current-item deltas. A dual slot (Ear/Wrist/Fingers)
// may return two items.
func (h *charactersHandler) equippedInSlot(eqPath, charName string, slot upgradeSlot) (items []upgradeCurrentItem, hasGear bool) {
	path := zeal.FindQuarmyFile(eqPath, charName)
	if path == "" {
		return nil, false
	}
	q, err := zeal.ParseQuarmy(path, charName)
	if err != nil || q == nil {
		return nil, false
	}
	for _, entry := range q.Inventory {
		if entry.Location != slot.Location || entry.ID <= 0 {
			continue
		}
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
	return items, true
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
