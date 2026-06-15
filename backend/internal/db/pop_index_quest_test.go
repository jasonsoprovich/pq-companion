package db_test

import (
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
)

// TestIsPoPGated_QuestSources verifies the quest-script source data corrects
// the two classes of leak the DB-only derivation couldn't handle:
//
//   - Jade Hoop of Speed (32106): no drop/vendor row, but a quest reward in
//     Plane of Knowledge (PoP). Must now be gated (was leaking through).
//   - Sigil Earring of Veracity (29861): no drop/vendor row, but a quest
//     reward in Katta Castellum (Luclin, current era). Must NOT be gated.
func TestIsPoPGated_QuestSources(t *testing.T) {
	d := openTestDB(t)

	cases := []struct {
		id       int
		name     string
		wantGate bool
	}{
		{32106, "Jade Hoop of Speed (PoK quest reward)", true},
		{29861, "Sigil Earring of Veracity (Katta/Luclin quest reward)", false},
		{15929, "Headsman's Hoop (pojustice drop)", true},
		// Drops only from unspawned Plane of Time NPCs — invisible to the
		// spawn2 join, caught via the id-derived home zone.
		{9444, "Mask of Conceptual Energy (unspawned PoP NPC drop)", true},
	}
	for _, c := range cases {
		if got := d.IsPoPGated(c.id); got != c.wantGate {
			t.Errorf("IsPoPGated(%d) [%s] = %v, want %v", c.id, c.name, got, c.wantGate)
		}
	}
}

// TestQuestsForItem checks the item→quest index backing the Quests tab.
func TestQuestsForItem(t *testing.T) {
	rewardedBy, _ := db.QuestsForItem(29861)
	if len(rewardedBy) == 0 {
		t.Fatal("expected Sigil Earring of Veracity to be rewarded by a quest")
	}
	found := false
	for _, q := range rewardedBy {
		if q.Zone == "katta" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected a katta quest reward for 29861, got %+v", rewardedBy)
	}

	// A turn-in lookup: the Signet Earring (29860) is handed in to Lcea Katta.
	_, usedIn := db.QuestsForItem(29860)
	if len(usedIn) == 0 {
		t.Errorf("expected Signet Earring (29860) to be used as a quest turn-in")
	}
}

// TestGetItemQuests checks the display-resolved Quests payload: the chain is
// reconstructed prerequisite-first with zone long-names and item names filled
// in. The Sigil Earring of Veracity is a 3-step Lcea Katta chain (Jewel Box →
// Signet Earring → Sigil Earring), so its final step must grant the item and
// an earlier step must produce a turn-in the final step requires.
func TestGetItemQuests(t *testing.T) {
	d := openTestDB(t)
	q, err := d.GetItemQuests(29861) // Sigil Earring of Veracity
	if err != nil {
		t.Fatalf("GetItemQuests: %v", err)
	}
	if len(q.Chain) < 2 {
		t.Fatalf("expected a multi-step chain for the Sigil Earring, got %d steps", len(q.Chain))
	}
	last := q.Chain[len(q.Chain)-1]
	if last.ZoneName == "" || last.ZoneName == last.ZoneShortName {
		t.Errorf("zone long-name not resolved: %+v", last)
	}
	grantsTarget := false
	for _, g := range last.Grants {
		if g.ID == 29861 {
			grantsTarget = true
		}
	}
	if !grantsTarget {
		t.Errorf("final chain step should grant the target item; got grants %+v", last.Grants)
	}
	// Every referenced item name must be resolved.
	for _, s := range q.Chain {
		for _, ri := range append(append([]db.ItemRef{}, s.Requires...), s.Grants...) {
			if ri.Name == "" {
				t.Errorf("unresolved item name for %d in chain", ri.ID)
			}
		}
	}
}
