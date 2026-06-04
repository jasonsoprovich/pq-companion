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
	plan := shoproute.Solve(input)

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
