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

// distUnreachable is the distance assigned to a zone the start can't reach over
// the connectivity graph — larger than any real hop count, so reachable zones
// always sort ahead of unreachable ones.
const distUnreachable = int(^uint(0) >> 1) // max int

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
// dist gives hop-distance from the player's start zone to each zone (see
// Distances). When a spell is sold in several zones that tie on coverage, the
// nearest one is preferred, so the route reflects where the player actually is.
// Pass a nil dist when there's no start zone — then ties fall through to the
// short-name.
//
// Determinism: coverage is the primary key; among equal-coverage zones the
// nearer wins; the lexicographically-first short-name breaks any remaining tie,
// so the same input always yields the same route.
func Solve(spells []SpellAvail, dist map[string]int) Plan {
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
		best := bestZone(missing, dist)
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

// bestZone returns the zone covering the most still-missing spells. Ties on
// coverage go to the zone nearest the start (per dist), then to the
// lexicographically-first short-name for determinism.
func bestZone(missing map[int]map[string]bool, dist map[string]int) string {
	counts := make(map[string]int)
	for _, zones := range missing {
		for z := range zones {
			counts[z]++
		}
	}
	best, bestCount, bestDist := "", 0, 0
	for z, c := range counts {
		d := zoneDist(dist, z)
		if best == "" || better(c, d, z, bestCount, bestDist, best) {
			best, bestCount, bestDist = z, c, d
		}
	}
	return best
}

// better reports whether candidate (c,d,z) beats the current best (bc,bd,bz):
// more coverage wins; equal coverage goes to the closer zone; equal distance
// goes to the lexicographically smaller short-name.
func better(c, d int, z string, bc, bd int, bz string) bool {
	if c != bc {
		return c > bc
	}
	if d != bd {
		return d < bd
	}
	return z < bz
}

// zoneDist looks up a zone's hop-distance from the start. With no distance map
// every zone scores 0, so the comparison falls through to the short-name and
// behaviour matches the old alphabetical tiebreak.
func zoneDist(dist map[string]int, z string) int {
	if dist == nil {
		return 0
	}
	if d, ok := dist[z]; ok {
		return d
	}
	return distUnreachable
}
