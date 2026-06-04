package db_test

import (
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/shoproute"
)

// TestGetSpellVendorOptions exercises the batch vendor/zone resolution against
// the real quarm.db and feeds the result through the route solver, asserting
// the produced itinerary is a valid set cover: every spell is covered exactly
// once, and each stop genuinely sells the spells credited to it. Spells 48
// (Cancel Magic), 709 and 710 (bard tunes) are all stocked by the same general
// caster-merchant set, so they should all be buyable somewhere.
func TestGetSpellVendorOptions(t *testing.T) {
	d := openTestDB(t)

	ids := []int{48, 709, 710}

	opts, err := d.GetSpellVendorOptions(ids)
	if err != nil {
		t.Fatalf("GetSpellVendorOptions: %v", err)
	}
	if len(opts) == 0 {
		t.Fatal("expected vendor options, got none")
	}

	// Every option must carry a resolvable zone and a vendor.
	zonesPerSpell := map[int]map[string]bool{}
	for _, o := range opts {
		if o.ZoneShort == "" || o.VendorID == 0 {
			t.Errorf("option missing zone/vendor: %+v", o)
		}
		if zonesPerSpell[o.SpellID] == nil {
			zonesPerSpell[o.SpellID] = map[string]bool{}
		}
		zonesPerSpell[o.SpellID][o.ZoneShort] = true
	}

	input := make([]shoproute.SpellAvail, 0, len(ids))
	for _, id := range ids {
		input = append(input, shoproute.SpellAvail{SpellID: id, Zones: zonesPerSpell[id]})
	}
	plan := shoproute.Solve(input, nil)

	if len(plan.Uncovered) != 0 {
		t.Errorf("unexpected uncovered spells: %v", plan.Uncovered)
	}
	// Validity: each spell covered exactly once, by a zone that truly sells it.
	covered := map[int]int{}
	for _, stop := range plan.Stops {
		for _, id := range stop.SpellIDs {
			covered[id]++
			if !zonesPerSpell[id][stop.Zone] {
				t.Errorf("stop %q credited with spell %d it doesn't sell", stop.Zone, id)
			}
		}
	}
	for _, id := range ids {
		if covered[id] != 1 {
			t.Errorf("spell %d covered %d times, want 1", id, covered[id])
		}
	}
}

// TestShoppingRouteHonorsStartZone is the regression test for the reported bug:
// Strengthen (spell 40) is sold in ~14 zones, so with no distance signal the
// solver used to always pick the alphabetically-first zone (Ak'Anon), ignoring
// where the player starts. With Plane of Knowledge excluded (it's pruned from
// the graph and dropped as a source) and a start next to Shadow Haven, the
// nearest source — Shadow Haven — must win.
func TestShoppingRouteHonorsStartZone(t *testing.T) {
	d := openTestDB(t)

	const strengthen = 40
	opts, err := d.GetSpellVendorOptions([]int{strengthen})
	if err != nil {
		t.Fatalf("GetSpellVendorOptions: %v", err)
	}

	// Build the source-zone set with Plane of Knowledge excluded, mirroring the
	// handler's default (include_pok = false).
	zones := map[string]bool{}
	for _, o := range opts {
		if o.ZoneShort == "poknowledge" {
			continue
		}
		zones[o.ZoneShort] = true
	}
	if !zones["shadowhaven"] {
		t.Fatalf("test premise broken: Strengthen not sold in shadowhaven; zones=%v", zones)
	}
	input := []shoproute.SpellAvail{{SpellID: strengthen, Zones: zones}}

	adj, err := d.GetZoneAdjacency()
	if err != nil {
		t.Fatalf("GetZoneAdjacency: %v", err)
	}
	// Prune PoK from the graph the way the handler does, so distances don't
	// shortcut through the book hub.
	for z, ns := range adj {
		if z == "poknowledge" {
			delete(adj, z)
			continue
		}
		kept := ns[:0]
		for _, n := range ns {
			if n != "poknowledge" {
				kept = append(kept, n)
			}
		}
		adj[z] = kept
	}

	// Starting at the Nexus (one hop from Shadow Haven), the nearest source wins.
	dist := shoproute.Distances("nexus", adj)
	plan := shoproute.Solve(input, dist)
	if len(plan.Stops) != 1 {
		t.Fatalf("expected one stop, got %d: %+v", len(plan.Stops), plan.Stops)
	}
	if got := plan.Stops[0].Zone; got != "shadowhaven" {
		t.Errorf("start=nexus, PoK off: routed to %q, want shadowhaven", got)
	}

	// Sanity check the old behaviour: with no distance signal the alphabetical
	// tiebreak still picks the earliest zone (akanon), proving distance is what
	// changes the result.
	if got := shoproute.Solve(input, nil).Stops[0].Zone; got != "akanon" {
		t.Errorf("no-dist control: routed to %q, want akanon", got)
	}
}

// TestTeleportDestinationsAreReachable checks that the Druid/Wizard port
// destinations load from the real DB and that, once linked onto the Nexus hub,
// a zone that's far by zone-lines (South Ro) collapses to a single hop from the
// Nexus — the "easy to catch a port" model the route relies on.
func TestTeleportDestinationsAreReachable(t *testing.T) {
	d := openTestDB(t)

	dests, err := d.GetTeleportDestinations()
	if err != nil {
		t.Fatalf("GetTeleportDestinations: %v", err)
	}
	if len(dests) < 20 {
		t.Fatalf("expected the full Druid/Wizard port list, got %d: %v", len(dests), dests)
	}
	set := map[string]bool{}
	for _, z := range dests {
		set[z] = true
	}
	// Spot-check known ports are present and evac-only junk is absent.
	for _, want := range []string{"sro", "commons", "gfaydark", "nexus"} {
		if !set[want] {
			t.Errorf("expected %q among teleport destinations", want)
		}
	}

	adj, err := d.GetZoneAdjacency()
	if err != nil {
		t.Fatalf("GetZoneAdjacency: %v", err)
	}
	// Before linking, South Ro is several zone-lines from the Nexus.
	if base := shoproute.Distances("nexus", adj)["sro"]; base <= 1 {
		t.Fatalf("test premise broken: sro already %d hop(s) from nexus before linking", base)
	}
	// After linking the teleport hub, it's one hop.
	linked := shoproute.LinkHub(adj, "nexus", dests)
	if got := shoproute.Distances("nexus", linked)["sro"]; got != 1 {
		t.Errorf("sro should be 1 hop from nexus after teleport linking, got %d", got)
	}
}

// TestGetZoneAdjacency confirms the zone graph loads and is symmetric (every
// edge present in both directions), with no self-loops.
func TestGetZoneAdjacency(t *testing.T) {
	d := openTestDB(t)
	adj, err := d.GetZoneAdjacency()
	if err != nil {
		t.Fatalf("GetZoneAdjacency: %v", err)
	}
	if len(adj) < 50 {
		t.Fatalf("expected a substantial zone graph, got %d nodes", len(adj))
	}
	// Plane of Knowledge is a hub — it should connect to many zones.
	if len(adj["poknowledge"]) < 5 {
		t.Errorf("poknowledge has %d neighbors, expected many (hub)", len(adj["poknowledge"]))
	}
	for node, neighbors := range adj {
		for _, n := range neighbors {
			if n == node {
				t.Errorf("self-loop at %q", node)
			}
			// Symmetry: n must list node back.
			found := false
			for _, back := range adj[n] {
				if back == node {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("edge %q->%q not mirrored", node, n)
			}
		}
	}
}

// TestGetSpellVendorOptionsEmpty confirms a non-vendor spell id yields no
// options (it would surface as "unavailable" in the route).
func TestGetSpellVendorOptionsEmpty(t *testing.T) {
	d := openTestDB(t)
	opts, err := d.GetSpellVendorOptions([]int{99999999})
	if err != nil {
		t.Fatalf("GetSpellVendorOptions: %v", err)
	}
	if len(opts) != 0 {
		t.Errorf("expected no options for bogus spell, got %d", len(opts))
	}
}
