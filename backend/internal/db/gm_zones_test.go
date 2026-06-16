package db_test

import (
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
)

// IsGMZoneItem must flag gear sold only by Sunset Home (cshome/cshome2)
// merchants, and must NOT flag ordinary obtainable items.
func TestIsGMZoneItem(t *testing.T) {
	d := openTestDB(t)

	// Known Sunset Home-exclusive chest pieces (sold only by GM-zone merchants,
	// never dropped/sold elsewhere) — verified against quarm.db.
	gmOnly := []int{27188, 31894, 31908} // Breastplate of Oasis / of Tranquility / Chestguard of the Mediator
	for _, id := range gmOnly {
		if !d.IsGMZoneItem(id) {
			t.Errorf("item %d should be flagged as GM-zone-only", id)
		}
	}

	// A staple obtainable item must not be flagged.
	if d.IsGMZoneItem(1001) { // Cloth Cap — sold all over Norrath
		t.Error("Cloth Cap (1001) wrongly flagged as GM-zone-only")
	}
}

// The upgrade finder must not surface GM-zone-only gear. An Enchanter chest
// sweep should exclude the known Sunset Home breastplates.
func TestUpgradeCandidates_ExcludesGMZoneItems(t *testing.T) {
	d := openTestDB(t)

	const chestBit = 0x020000
	cands, err := d.UpgradeCandidates(db.CandidateFilter{
		SlotMask: chestBit,
		ClassBit: 0x2000, // enchanter
		MaxLevel: 65,
	})
	if err != nil {
		t.Fatalf("UpgradeCandidates: %v", err)
	}
	gmOnly := map[int]bool{27188: true, 31894: true, 31908: true}
	for _, c := range cands {
		if gmOnly[c.ID] {
			t.Errorf("GM-zone-only item %q (%d) was offered as an upgrade", c.Name, c.ID)
		}
	}
}
