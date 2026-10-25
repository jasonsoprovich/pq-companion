package api

import (
	"net/http"
	"sort"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/jasonsoprovich/pq-companion/backend/internal/buffmod"
	"github.com/jasonsoprovich/pq-companion/backend/internal/character"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/db/enums"
	"github.com/jasonsoprovich/pq-companion/backend/internal/era"
	"github.com/jasonsoprovich/pq-companion/backend/internal/zeal"
)

// Classes (0-based) that don't cast spells, so "missing spell focus" hints would
// only be noise: Warrior, Monk, Rogue, Bard.
var nonCasterClasses = map[int]bool{0: true, 6: true, 7: true, 8: true}

// maxFocusUpgradeItems caps one category's item list. High enough that lower
// ranks of a worn focus (the "pivot" view) aren't cut off.
const maxFocusUpgradeItems = 200

// focusItem is one item a character could wear for a focus category.
type focusItem struct {
	ItemID       int            `json:"item_id"`
	Name         string         `json:"name"`
	Icon         int            `json:"icon"`
	Slots        int            `json:"slots"`
	ReqLevel     int            `json:"req_level"`
	NoDrop       bool           `json:"no_drop"`
	FocusSpellID int            `json:"focus_spell_id"`
	FocusName    string         `json:"focus_name"`
	Percent      int            `json:"percent"`
	Limits       buffmod.Limits `json:"limits"`
	// CoversTop is true when the focus still applies to the character's highest
	// spells. Focus level caps are hard limits (a "Max Level 60" focus does
	// nothing on a level-61+ spell), so a lower-percent focus that covers the
	// top spells can beat a higher one that doesn't.
	CoversTop bool `json:"covers_top"`
	// Equipped is true when the character is wearing this item now.
	Equipped bool `json:"equipped"`
	// IsUpgrade is true when the item's focus beats the best one currently worn
	// in this category (never set for categories where "higher" isn't "better").
	IsUpgrade bool `json:"is_upgrade"`
}

// missingFocusCategory is a category the character has no worn focus for but
// could get from at least one item usable by their class and level.
type missingFocusCategory struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	Blurb     string `json:"blurb"`
	ItemCount int    `json:"item_count"`
}

// focusTopLevel is the highest spell level the character can be casting: their
// level, capped at the server's level cap.
func focusTopLevel(char character.Character) int {
	if char.Level > era.PoPMaxLevel {
		return era.PoPMaxLevel
	}
	return char.Level
}

// focusCoversTop reports whether a focus with these limits applies to spells at
// the character's top level (no max-level limit, or one at or above it).
func focusCoversTop(l buffmod.Limits, topLevel int) bool {
	return l.MaxLevel == 0 || l.MaxLevel >= topLevel
}

// focusCandidates returns every item the character's class/race/level can use
// that carries a categorised focus, grouped by category.
func (h *charactersHandler) focusCandidates(char character.Character) (map[string][]focusItem, error) {
	cands, err := h.db.UpgradeCandidates(db.CandidateFilter{
		SlotMask:  0x7FFFFFFF, // every slot — a focus is on the item, not the slot
		ClassBit:  charClassBit(char),
		RaceBit:   enums.RaceBitForCharRace(char.Race),
		MaxLevel:  char.Level,
		FocusOnly: true,
	})
	if err != nil {
		return nil, err
	}
	spells := map[int][]buffmod.Modifier{}
	out := map[string][]focusItem{}
	top := focusTopLevel(char)
	for _, c := range cands {
		mods, ok := spells[c.FocusEffect]
		if !ok {
			if sp, err := h.db.GetSpell(c.FocusEffect); err == nil && sp != nil {
				mods = buffmod.FocusModifiers(sp)
			}
			spells[c.FocusEffect] = mods
		}
		// An item's category is its first categorised focus slot.
		for _, m := range mods {
			if m.Category == "" {
				continue
			}
			out[m.Category] = append(out[m.Category], focusItem{
				ItemID: c.ID, Name: c.Name, Icon: c.Icon, Slots: c.Slots, ReqLevel: c.ReqLevel,
				NoDrop: c.NoDrop == 0, FocusSpellID: c.FocusEffect, FocusName: c.FocusName,
				Percent: m.Percent, Limits: m.Limits, CoversTop: focusCoversTop(m.Limits, top),
			})
			break
		}
	}
	return out, nil
}

// equippedFocusCategory is a category the character already wears a focus for,
// with the strongest worn percent — the entry point for planning gear pivots.
type equippedFocusCategory struct {
	missingFocusCategory
	SPA     int `json:"spa"`
	Percent int `json:"percent"`
}

// focusCategorySummary splits the focus categories into the ones the character
// could use but has no worn contributor for (empty for non-casting classes) and
// the ones they already wear, both in display order.
func (h *charactersHandler) focusCategorySummary(char character.Character, contributors []buffmod.Modifier) ([]missingFocusCategory, []equippedFocusCategory) {
	missing := []missingFocusCategory{}
	equipped := []equippedFocusCategory{}
	// Strongest worn magnitude per category (aggro reduction is negative).
	have := map[string]int{}
	for _, m := range contributors {
		if m.Category == "" {
			continue
		}
		if cur, ok := have[m.Category]; !ok || abs(m.Percent) > abs(cur) {
			have[m.Category] = m.Percent
		}
	}
	cands, err := h.focusCandidates(char)
	if err != nil {
		return missing, equipped
	}
	for _, def := range buffmod.Categories {
		cat := missingFocusCategory{
			ID: def.ID, Label: def.Label, Blurb: def.Blurb, ItemCount: len(cands[def.ID]),
		}
		if pct, ok := have[def.ID]; ok {
			equipped = append(equipped, equippedFocusCategory{
				missingFocusCategory: cat, SPA: def.SPA, Percent: pct,
			})
			continue
		}
		if nonCasterClasses[char.Class] || def.NoMissingHint || cat.ItemCount == 0 {
			continue
		}
		missing = append(missing, cat)
	}
	return missing, equipped
}

// missingFocusCategories lists the categories the character could use but has
// no worn contributor for, in display order. Empty for non-casting classes.
func (h *charactersHandler) missingFocusCategories(char character.Character, contributors []buffmod.Modifier) []missingFocusCategory {
	missing, _ := h.focusCategorySummary(char, contributors)
	return missing
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

type focusUpgradesResponse struct {
	Category string `json:"category"`
	Label    string `json:"label"`
	Blurb    string `json:"blurb"`
	SPA      int    `json:"spa"`
	// Rolls is true when the percent is a per-cast random 1..N% (not fixed).
	Rolls bool `json:"rolls"`
	// Current lists what the character wears in this category now.
	Current []currentFocus `json:"current"`
	// CurrentPercent is the best worn percent among foci that still cover the
	// character's top spells (0 when none do) — only the best applicable focus
	// in a category applies.
	CurrentPercent int         `json:"current_percent"`
	Items          []focusItem `json:"items"`
}

// currentFocus is a worn contributor plus whether it covers the top spells.
type currentFocus struct {
	buffmod.Modifier
	CoversTop bool `json:"covers_top"`
}

// focusUpgrades handles GET /api/characters/{id}/focus-upgrades?category=<id>:
// what the character wears for a focus category, plus every item their class and
// level can use for it, best first.
func (h *charactersHandler) focusUpgrades(w http.ResponseWriter, r *http.Request) {
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
	catID := r.URL.Query().Get("category")
	var def *buffmod.CategoryDef
	for i := range buffmod.Categories {
		if buffmod.Categories[i].ID == catID {
			def = &buffmod.Categories[i]
		}
	}
	if def == nil {
		writeError(w, http.StatusBadRequest, "unknown category")
		return
	}

	// What's worn now (best effort — no export just means nothing is worn).
	top := focusTopLevel(char)
	current := []currentFocus{}
	cfg := h.mgr.Get()
	if cfg.EQPath != "" && zeal.FindQuarmyFile(cfg.EQPath, char.Name) != "" {
		if res, err := buffmod.Compute(cfg.EQPath, char.Name, h.db); err == nil {
			for _, m := range res.Contributors {
				if m.Category == catID {
					current = append(current, currentFocus{Modifier: m, CoversTop: focusCoversTop(m.Limits, top)})
				}
			}
		}
	}
	// best is the best percent among worn foci that still cover the top spells.
	best, anyCovering := 0, false
	wornItems := map[int]bool{}
	for _, m := range current {
		if m.CoversTop {
			anyCovering = true
			if m.Percent > best {
				best = m.Percent
			}
		}
		if m.SourceItemID > 0 {
			wornItems[m.SourceItemID] = true
		}
	}

	cands, err := h.focusCandidates(char)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	items := cands[catID]
	// For hate foci a bigger number isn't plainly better, so never flag upgrades.
	comparable := catID != "aggro_reduction" && catID != "bash_hate"
	for i := range items {
		items[i].Equipped = wornItems[items[i].ItemID]
		// An upgrade must still apply to the character's top spells, and either
		// nothing worn does, or it rolls/gives more than the best worn one.
		items[i].IsUpgrade = comparable && items[i].CoversTop &&
			(!anyCovering || items[i].Percent > best)
	}
	sort.SliceStable(items, func(a, b int) bool {
		if items[a].CoversTop != items[b].CoversTop {
			return items[a].CoversTop
		}
		if items[a].Percent != items[b].Percent {
			return items[a].Percent > items[b].Percent
		}
		if items[a].ReqLevel != items[b].ReqLevel {
			return items[a].ReqLevel < items[b].ReqLevel
		}
		return items[a].Name < items[b].Name
	})
	if len(items) > maxFocusUpgradeItems {
		items = items[:maxFocusUpgradeItems]
	}
	if items == nil {
		items = []focusItem{}
	}
	writeJSON(w, http.StatusOK, focusUpgradesResponse{
		Category: def.ID, Label: def.Label, Blurb: def.Blurb, SPA: def.SPA, Rolls: def.Rolls,
		Current: current, CurrentPercent: best, Items: items,
	})
}
