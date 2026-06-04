// Package shoproute computes an efficient shopping route for a list of items
// (spells) that are sold across multiple zones. Picking the fewest zones that
// cover the whole list is the classic Set Cover problem, which is NP-hard, so
// we use a greedy heuristic that runs instantly and lands within a small
// factor of optimal for realistic list sizes.
//
// The solver is pure: it knows nothing about the database or HTTP. Callers
// resolve each spell to the set of zones where it can be bought, hand that to
// Solve, and enrich the resulting zone keys (vendor names, coordinates, spell
// names) for display.
//
// The algorithm has two phases:
//
//	Phase 1 — anchors. A spell sold in only one zone forces that zone into any
//	solution, so we add every such zone up front and cross off every other
//	spell that happens to be co-located there for free.
//
//	Phase 2 — greedy max-density. For the remaining spells, repeatedly pick the
//	zone that covers the most still-missing spells, add it, cross those off,
//	and repeat until the list is empty.
package shoproute

import "sort"

// SpellAvail is the solver input for one spell: the spell's id and the set of
// zone short-names where it can be purchased. A spell with an empty Zones set
// is reported back as unavailable.
type SpellAvail struct {
	SpellID int
	Zones   map[string]bool
}

// StopReason explains why a zone was added to the itinerary.
type StopReason string

const (
	// ReasonAnchor means the zone is the only source of at least one spell.
	ReasonAnchor StopReason = "anchor"
	// ReasonGreedy means the zone was chosen because it covered the most
	// remaining spells at the time it was picked.
	ReasonGreedy StopReason = "greedy"
)

// Stop is one zone in the computed itinerary.
type Stop struct {
	Zone     string
	Reason   StopReason
	SpellIDs []int // spells this stop is credited with covering, sorted
}

// Plan is the solver output: an ordered itinerary plus any spells that no
// zone could cover.
type Plan struct {
	Stops     []Stop
	Uncovered []int // spell ids with no purchasable zone, sorted
}

// Solve runs the two-phase greedy set-cover heuristic over the given spells.
//
// Determinism: when two zones tie on coverage, the zone whose short-name sorts
// first wins, so the same input always yields the same route.
func Solve(spells []SpellAvail) Plan {
	// missing tracks spells not yet covered, keyed by spell id.
	missing := make(map[int]map[string]bool, len(spells))
	var uncovered []int
	for _, s := range spells {
		if len(s.Zones) == 0 {
			uncovered = append(uncovered, s.SpellID)
			continue
		}
		// Copy the zone set so we never mutate the caller's data.
		zones := make(map[string]bool, len(s.Zones))
		for z := range s.Zones {
			zones[z] = true
		}
		missing[s.SpellID] = zones
	}
	sort.Ints(uncovered)

	plan := Plan{Uncovered: uncovered}
	chosen := make(map[string]bool) // zones already added to the itinerary

	// Phase 1 — anchors. A spell with exactly one zone forces that zone.
	var anchorZones []string
	for _, zones := range missing {
		if len(zones) == 1 {
			for z := range zones {
				if !chosen[z] {
					chosen[z] = true
					anchorZones = append(anchorZones, z)
				}
			}
		}
	}
	sort.Strings(anchorZones)
	for _, z := range anchorZones {
		covered := takeCovered(missing, z)
		if len(covered) > 0 {
			plan.Stops = append(plan.Stops, Stop{
				Zone: z, Reason: ReasonAnchor, SpellIDs: covered,
			})
		}
	}

	// Phase 2 — greedy max-density over whatever remains.
	for len(missing) > 0 {
		best := bestZone(missing)
		if best == "" {
			break // defensive: every remaining spell had at least one zone
		}
		chosen[best] = true
		covered := takeCovered(missing, best)
		plan.Stops = append(plan.Stops, Stop{
			Zone: best, Reason: ReasonGreedy, SpellIDs: covered,
		})
	}

	return plan
}

// takeCovered removes every still-missing spell available in zone z and
// returns their ids, sorted. After this call those spells are no longer in
// missing, so they can't be credited to a later stop.
func takeCovered(missing map[int]map[string]bool, z string) []int {
	var covered []int
	for id, zones := range missing {
		if zones[z] {
			covered = append(covered, id)
			delete(missing, id)
		}
	}
	sort.Ints(covered)
	return covered
}

// bestZone returns the zone covering the most still-missing spells, breaking
// ties by lexicographic zone short-name for determinism.
func bestZone(missing map[int]map[string]bool) string {
	counts := make(map[string]int)
	for _, zones := range missing {
		for z := range zones {
			counts[z]++
		}
	}
	best, bestCount := "", 0
	for z, c := range counts {
		if c > bestCount || (c == bestCount && z < best) {
			best, bestCount = z, c
		}
	}
	return best
}
