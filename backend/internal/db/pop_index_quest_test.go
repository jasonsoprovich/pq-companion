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

// TestSearchQuests checks the DB-explorer quest search matches on a related
// item name and resolves zone/item display fields.
func TestSearchQuests(t *testing.T) {
	d := openTestDB(t)
	res := d.SearchQuests("Sigil Earring of Veracity", 50, 0)
	if res.Total == 0 {
		t.Fatal("expected a quest matching the Sigil Earring item name")
	}
	found := false
	for _, q := range res.Results {
		if q.NPC == "Lcea Katta" && q.ZoneName != "" && q.ZoneName != q.ZoneShortName {
			found = true
		}
	}
	if !found {
		t.Errorf("expected Lcea Katta with a resolved zone name in results")
	}

	// Empty query returns the full set, paged.
	all := d.SearchQuests("", 10, 0)
	if all.Total < 100 || len(all.Results) != 10 {
		t.Errorf("empty-query paging off: total=%d page=%d", all.Total, len(all.Results))
	}
}

// TestGetItemQuests checks the display-resolved walkthrough: the Sigil Earring
// of Veracity is rewarded by Lcea Katta, whose dialogue must resolve with a
// zone long-name, readable text, and a branch that grants the item.
func TestGetItemQuests(t *testing.T) {
	d := openTestDB(t)
	q, err := d.GetItemQuests(29861) // Sigil Earring of Veracity
	if err != nil {
		t.Fatalf("GetItemQuests: %v", err)
	}
	if len(q.Walkthrough) == 0 {
		t.Fatal("expected a walkthrough for the Sigil Earring")
	}
	w := q.Walkthrough[0]
	if w.ZoneName == "" || w.ZoneName == w.ZoneShortName {
		t.Errorf("zone long-name not resolved: %+v", w)
	}
	if len(w.Dialogue) < 2 {
		t.Fatalf("expected multiple dialogue branches, got %d", len(w.Dialogue))
	}
	grantsTarget, hasText := false, false
	for _, b := range w.Dialogue {
		if b.Text != "" {
			hasText = true
		}
		for _, g := range b.Grants {
			if g.ID == 29861 {
				grantsTarget = true
			}
			if g.Name == "" {
				t.Errorf("unresolved grant item name for %d", g.ID)
			}
		}
	}
	if !grantsTarget {
		t.Error("expected a dialogue branch granting the Sigil Earring")
	}
	if !hasText {
		t.Error("expected at least one dialogue branch with NPC text")
	}
}
