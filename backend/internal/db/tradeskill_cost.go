package db

import "fmt"

// itemAcquisition is the cheapest known way to obtain one unit of an item:
// bought from a vendor, or crafted from (recursively) obtainable components,
// whichever is cheaper. known=false means no vendor sells it and it can't be
// fully crafted from vendor-obtainable parts — i.e. something must be
// farmed/dropped.
type itemAcquisition struct {
	copper int
	known  bool
}

type recipeComp struct {
	itemID int
	count  int
}

// costResolver answers "cheapest copper to obtain item X" across the whole
// (enabled, non-quest) recipe DAG, memoized, with cycle guarding. It is built
// once per plan request from bulk snapshots of vendor prices and the recipe
// graph, so recursion happens in memory rather than as per-item queries.
//
// This is what lets the leveling planner cost a recipe whose components aren't
// directly vendor-sold but can be sub-crafted from vendor materials (e.g.
// blacksmithing sheet metal), and to distinguish a genuine "must craft it"
// sub-combine from a component you'd just buy.
type costResolver struct {
	vendorPrice map[int]int          // item -> base copper (merchant-sold only)
	producedBy  map[int][]int        // item -> every enabled recipe id that yields it
	components  map[int][]recipeComp // recipe id -> its consumed components
	yield       map[int]int          // recipe id -> primary product successcount
	trivial     map[int]int          // recipe id -> trivial
	memo        map[int]itemAcquisition
	inProgress  map[int]bool
}

func (db *DB) newCostResolver() (*costResolver, error) {
	r := &costResolver{
		vendorPrice: map[int]int{},
		producedBy:  map[int][]int{},
		components:  map[int][]recipeComp{},
		yield:       map[int]int{},
		trivial:     map[int]int{},
		memo:        map[int]itemAcquisition{},
		inProgress:  map[int]bool{},
	}

	vrows, err := db.Query(`
		SELECT i.id, i.price FROM items i
		WHERE EXISTS (SELECT 1 FROM merchantlist m WHERE m.item = i.id)`)
	if err != nil {
		return nil, fmt.Errorf("cost resolver vendor prices: %w", err)
	}
	for vrows.Next() {
		var id, price int
		if err := vrows.Scan(&id, &price); err != nil {
			vrows.Close()
			return nil, fmt.Errorf("scan vendor price: %w", err)
		}
		r.vendorPrice[id] = price
	}
	vrows.Close()
	if err := vrows.Err(); err != nil {
		return nil, err
	}

	// The recipe graph: components consumed and products yielded. Quest recipes
	// are excluded — a one-off quest combine isn't a repeatable way to craft a
	// sub-component.
	erows, err := db.Query(`
		SELECT tre.recipe_id, r.trivial, tre.item_id, tre.componentcount, tre.successcount, tre.iscontainer
		FROM tradeskill_recipe_entries tre
		JOIN tradeskill_recipe r ON r.id = tre.recipe_id
		WHERE r.enabled = 1 AND r.quest = 0
		ORDER BY tre.recipe_id, tre.id`)
	if err != nil {
		return nil, fmt.Errorf("cost resolver recipe graph: %w", err)
	}
	defer erows.Close()
	for erows.Next() {
		var recipeID, triv, itemID, cc, sc, isCon int
		if err := erows.Scan(&recipeID, &triv, &itemID, &cc, &sc, &isCon); err != nil {
			return nil, fmt.Errorf("scan recipe graph: %w", err)
		}
		r.trivial[recipeID] = triv
		switch {
		case isCon != 0:
			// Containers are durable vessels, not consumed materials.
		case sc > 0:
			if _, ok := r.yield[recipeID]; !ok {
				r.yield[recipeID] = sc // primary product's per-combine output
			}
			r.producedBy[itemID] = append(r.producedBy[itemID], recipeID)
		case cc > 0:
			r.components[recipeID] = append(r.components[recipeID], recipeComp{itemID: itemID, count: cc})
		}
	}
	return r, erows.Err()
}

// craftableSubcombine reports the producing recipe for an item that must be
// CRAFTED to obtain (not vendor-sold) — a genuine sub-combine dependency — or
// (0,false) if the item is vendor-sold or not craftable. When several recipes
// make it, the lowest-trivial one is returned (the easiest to make, and the
// most era-appropriate). Self-production is ignored.
func (r *costResolver) craftableSubcombine(itemID, consumingRecipe int) (int, bool) {
	if _, vendorSold := r.vendorPrice[itemID]; vendorSold {
		return 0, false // you'd just buy it
	}
	bestID, bestTriv := 0, 0
	for _, pid := range r.producedBy[itemID] {
		if pid == consumingRecipe {
			continue
		}
		if bestID == 0 || r.trivial[pid] < bestTriv {
			bestID, bestTriv = pid, r.trivial[pid]
		}
	}
	return bestID, bestID != 0
}

// cost returns the cheapest acquisition for one unit of itemID — the min of its
// vendor price and crafting it via any producing recipe — memoized.
func (r *costResolver) cost(itemID int) itemAcquisition {
	if v, ok := r.memo[itemID]; ok {
		return v
	}
	if r.inProgress[itemID] {
		return itemAcquisition{} // cycle: unobtainable via this path (don't memo)
	}

	best := itemAcquisition{}
	if p, ok := r.vendorPrice[itemID]; ok {
		best = itemAcquisition{copper: p, known: true}
	}

	if producers, ok := r.producedBy[itemID]; ok {
		r.inProgress[itemID] = true
		for _, recipeID := range producers {
			craftCost, craftKnown := 0, true
			for _, comp := range r.components[recipeID] {
				cc := r.cost(comp.itemID)
				if !cc.known {
					craftKnown = false
					break
				}
				craftCost += comp.count * cc.copper
			}
			if craftKnown {
				y := r.yield[recipeID]
				if y < 1 {
					y = 1
				}
				perUnit := craftCost / y
				if !best.known || perUnit < best.copper {
					best = itemAcquisition{copper: perUnit, known: true}
				}
			}
		}
		r.inProgress[itemID] = false
	}

	r.memo[itemID] = best
	return best
}
