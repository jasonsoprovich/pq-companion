package db_test

import (
	"strings"
	"testing"
)

// The charm finder's zone list must not pull in NPCs whose id merely collides
// with a spawngroup id in the zone. User report: Western Wastes listed Plane
// of Tactics' "a War Boar" and "A Young War Boar".
func TestCharmCandidatesByZone_NoSpawngroupIDCollisions(t *testing.T) {
	d := openTestDB(t)
	cs, err := d.CharmCandidatesByZone("westwastes")
	if err != nil {
		t.Fatalf("CharmCandidatesByZone: %v", err)
	}
	seen := map[string]bool{}
	for _, c := range cs {
		seen[c.Name] = true
		if strings.Contains(strings.ToLower(c.Name), "war_boar") {
			t.Errorf("Plane of Tactics NPC %q (id %d) listed in Western Wastes", c.Name, c.ID)
		}
	}
	if !seen["a_velium_hound"] {
		t.Error("a_velium_hound (a real Western Wastes spawn) missing")
	}
}
