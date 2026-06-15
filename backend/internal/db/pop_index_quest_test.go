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

// TestGetItemQuests checks the display-resolved Quests payload: zone long-name
// and related turn-in item names are filled in, and the item itself is not
// listed as its own prerequisite.
func TestGetItemQuests(t *testing.T) {
	d := openTestDB(t)
	q, err := d.GetItemQuests(29861) // Sigil Earring of Veracity
	if err != nil {
		t.Fatalf("GetItemQuests: %v", err)
	}
	if len(q.RewardedBy) == 0 {
		t.Fatal("expected at least one rewarding quest")
	}
	ref := q.RewardedBy[0]
	if ref.ZoneName == "" || ref.ZoneName == ref.ZoneShortName {
		t.Errorf("zone long-name not resolved: %+v", ref)
	}
	for _, ri := range ref.RelatedItems {
		if ri.ID == 29861 {
			t.Error("item listed as its own turn-in")
		}
		if ri.Name == "" {
			t.Errorf("related item %d name not resolved", ri.ID)
		}
	}
}
