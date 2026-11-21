package db

import (
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strings"
)

// Planes of Power armor molds and patterns (Ornate, Elemental, Timeless) are
// quest combines: the mold + a "<Class> Emblem" + an ethereal material, in the
// Mystical Furnace of Ro (plate/chain/leather) or Druzzil's Mystical Sewing Kit
// (silk), make a class-specific armor piece. The armor itself never drops —
// the mold does — so the app cross-references the two: mold → the class items
// it can become, and item → the mold it is made from.

const (
	moldFurnaceID   = 17184 // Mystical Furnace of Ro
	moldSewingKitID = 17185 // Druzzil's Mystical Sewing Kit
)

var moldNameRe = regexp.MustCompile(`^(Ornate|Elemental|Timeless) .*(Mold|Pattern)$`)

// MoldItemRef is a named item reference within a mold recipe.
type MoldItemRef struct {
	ItemID int    `json:"item_id"`
	Name   string `json:"name"`
	Icon   int    `json:"icon"`
	Count  int    `json:"count,omitempty"`
}

// MoldProduct is one class item a mold can be combined into.
type MoldProduct struct {
	ItemID     int    `json:"item_id"`
	Name       string `json:"name"`
	Icon       int    `json:"icon"`
	Classes    int    `json:"classes"`
	ClassLabel string `json:"class_label"`
	RecipeID   int    `json:"recipe_id"`
	// Container is where the combine happens; Inputs are consumed (the mold
	// itself omitted); Tools go in and come back out (Tanaan hammer/needle).
	Container MoldItemRef   `json:"container"`
	Inputs    []MoldItemRef `json:"inputs"`
	Tools     []MoldItemRef `json:"tools"`
}

// MoldProductRef is a lean MoldProduct (no recipe) for list views such as
// NPC loot tables.
type MoldProductRef struct {
	ItemID     int    `json:"item_id"`
	Name       string `json:"name"`
	Icon       int    `json:"icon"`
	Classes    int    `json:"classes"`
	ClassLabel string `json:"class_label"`
}

// annotateMoldLoot fills MoldProducts on every mold item in the drops.
func (db *DB) annotateMoldLoot(drops []LootDrop) {
	for d := range drops {
		for i := range drops[d].Items {
			ps := db.MoldProducts(drops[d].Items[i].ItemID)
			if len(ps) == 0 {
				continue
			}
			refs := make([]MoldProductRef, len(ps))
			for k, p := range ps {
				refs[k] = MoldProductRef{ItemID: p.ItemID, Name: p.Name, Icon: p.Icon,
					Classes: p.Classes, ClassLabel: p.ClassLabel}
			}
			drops[d].Items[i].MoldProducts = refs
		}
	}
}

// MoldRef identifies the mold a product is made from. NoDrop follows the
// items.nodrop convention (0 = NO DROP).
type MoldRef struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Icon   int    `json:"icon"`
	NoDrop int    `json:"nodrop"`
}

type moldIndex struct {
	products    map[int][]MoldProduct // mold id → products
	productMold map[int]MoldRef       // product id → mold
}

func (db *DB) ensureMolds() *moldIndex {
	db.moldsOnce.Do(func() {
		idx, err := db.buildMoldIndex()
		if err != nil {
			slog.Warn("db: failed to build mold index", "err", err)
			idx = &moldIndex{products: map[int][]MoldProduct{}, productMold: map[int]MoldRef{}}
		}
		db.molds = idx
	})
	return db.molds
}

// MoldProducts returns the class items a mold can be combined into, sorted by
// class label. Nil when the item is not a PoP mold.
func (db *DB) MoldProducts(moldID int) []MoldProduct {
	return db.ensureMolds().products[moldID]
}

// ProductMold returns the mold an item is combined from, if it is a mold
// product.
func (db *DB) ProductMold(productID int) (MoldRef, bool) {
	m, ok := db.ensureMolds().productMold[productID]
	return m, ok
}

// TradeableMoldProductIDs returns every mold product whose mold can be traded
// (looted off a corpse by anyone / bid on). The products themselves are all NO
// DROP, so the upgrade finder's NO DROP filter judges them by their mold.
func (db *DB) TradeableMoldProductIDs() []int {
	idx := db.ensureMolds()
	out := make([]int, 0, len(idx.productMold))
	for pid, m := range idx.productMold {
		if m.NoDrop != 0 {
			out = append(out, pid)
		}
	}
	sort.Ints(out)
	return out
}

type moldEntry struct {
	recipeID    int
	itemID      int
	name        string
	icon        int
	classes     int
	nodrop      int
	component   int
	success     int
	isContainer bool
}

func (db *DB) buildMoldIndex() (*moldIndex, error) {
	rows, err := db.Query(`
		SELECT e.recipe_id, e.item_id, i.Name, i.icon, i.classes, i.nodrop,
		       e.componentcount, e.successcount, e.iscontainer
		FROM tradeskill_recipe_entries e
		JOIN tradeskill_recipe r ON r.id = e.recipe_id
		JOIN items i ON i.id = e.item_id
		WHERE r.tradeskill = 75
		  AND e.recipe_id IN (
		    SELECT recipe_id FROM tradeskill_recipe_entries
		    WHERE iscontainer = 1 AND item_id IN (?, ?))
		ORDER BY e.recipe_id, e.id`, moldFurnaceID, moldSewingKitID)
	if err != nil {
		return nil, fmt.Errorf("mold index: %w", err)
	}
	defer rows.Close()

	byRecipe := map[int][]moldEntry{}
	var order []int
	for rows.Next() {
		var e moldEntry
		var ic int
		if err := rows.Scan(&e.recipeID, &e.itemID, &e.name, &e.icon, &e.classes,
			&e.nodrop, &e.component, &e.success, &ic); err != nil {
			return nil, fmt.Errorf("mold index scan: %w", err)
		}
		e.isContainer = ic != 0
		if _, seen := byRecipe[e.recipeID]; !seen {
			order = append(order, e.recipeID)
		}
		byRecipe[e.recipeID] = append(byRecipe[e.recipeID], e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	idx := &moldIndex{products: map[int][]MoldProduct{}, productMold: map[int]MoldRef{}}
	for _, rid := range order {
		mold, product, ok := classifyMoldRecipe(byRecipe[rid])
		if !ok {
			continue
		}
		idx.products[mold.ID] = append(idx.products[mold.ID], product)
		idx.productMold[product.ItemID] = mold
	}
	for id := range idx.products {
		ps := idx.products[id]
		sort.Slice(ps, func(a, b int) bool { return ps[a].ClassLabel < ps[b].ClassLabel })
	}
	return idx, nil
}

// classifyMoldRecipe splits one recipe's rows by role. Input and output are
// separate rows, so a tool is an item id that appears both as a component and
// as a success output.
func classifyMoldRecipe(entries []moldEntry) (MoldRef, MoldProduct, bool) {
	inputs := map[int]bool{}
	for _, e := range entries {
		if e.component > 0 {
			inputs[e.itemID] = true
		}
	}
	var mold MoldRef
	var p MoldProduct
	var emblem string
	for _, e := range entries {
		ref := MoldItemRef{ItemID: e.itemID, Name: e.name, Icon: e.icon}
		switch {
		case e.isContainer:
			p.Container = ref
		case e.success > 0 && inputs[e.itemID]:
			// Returned tool: listed again on the component side; record once.
		case e.success > 0:
			if p.ItemID != 0 {
				return mold, p, false // more than one product: not a mold combine
			}
			p.ItemID, p.Name, p.Icon, p.Classes = e.itemID, e.name, e.icon, e.classes
			p.RecipeID = e.recipeID
		case e.component > 0 && moldNameRe.MatchString(e.name):
			mold = MoldRef{ID: e.itemID, Name: e.name, Icon: e.icon, NoDrop: e.nodrop}
		case e.component > 0:
			returned := false
			for _, o := range entries {
				if o.itemID == e.itemID && o.success > 0 {
					returned = true
					break
				}
			}
			if returned {
				p.Tools = append(p.Tools, ref)
				continue
			}
			ref.Count = e.component
			p.Inputs = append(p.Inputs, ref)
			if strings.HasSuffix(e.name, " Emblem") {
				emblem = strings.TrimSuffix(e.name, " Emblem")
			}
		}
	}
	if mold.ID == 0 || p.ItemID == 0 {
		return mold, p, false
	}
	p.ClassLabel = moldClassLabel(p.Classes, emblem)
	return mold, p, true
}

var moldClassNames = []string{
	"Warrior", "Cleric", "Paladin", "Ranger", "Shadow Knight", "Druid", "Monk",
	"Bard", "Rogue", "Shaman", "Necromancer", "Wizard", "Magician", "Enchanter",
	"Beastlord",
}

// moldClassLabel names the class a product is for: its single class bit when
// it has exactly one, else the emblem's class.
func moldClassLabel(classes int, emblem string) string {
	if classes > 0 && classes&(classes-1) == 0 {
		for i, n := range moldClassNames {
			if classes == 1<<i {
				return n
			}
		}
	}
	if emblem == "Shadowknight" {
		return "Shadow Knight"
	}
	return emblem
}
