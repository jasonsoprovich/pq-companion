package db_test

import "testing"

// TestGetNPCMerchant covers the three shapes callers have to handle: a real
// vendor, a non-merchant NPC, and an id that doesn't exist. The NPC page keys
// its "Sells" section off a nil return, so nil-vs-empty matters.
func TestGetNPCMerchant(t *testing.T) {
	d := openTestDB(t)

	// Discover a vendor that actually has inventory rather than hardcoding an
	// id: quarm.db is regenerated from upstream dumps, and an NPC's merchant_id
	// can point at a merchantlist that no longer has rows.
	var id int
	if err := d.QueryRow(`
		SELECT n.id FROM npc_types n
		JOIN merchantlist ml ON ml.merchantid = n.merchant_id
		JOIN items i ON i.id = ml.item
		WHERE n.merchant_id > 0
		LIMIT 1`).Scan(&id); err != nil {
		t.Skipf("no stocked vendor in DB: %v", err)
	}

	m, err := d.GetNPCMerchant(id)
	if err != nil {
		t.Fatalf("get merchant for %d: %v", id, err)
	}
	if m == nil {
		t.Fatalf("npc %d has merchantlist rows but GetNPCMerchant returned nil", id)
	}
	if m.MerchantID == 0 {
		t.Errorf("MerchantID: got 0, want non-zero")
	}
	if len(m.Items) == 0 {
		t.Fatal("Items: got none, want at least one")
	}
	for _, it := range m.Items {
		if it.ItemID == 0 {
			t.Errorf("item in slot %d has zero ItemID", it.Slot)
		}
		if it.Name == "" {
			t.Errorf("item %d has empty Name", it.ItemID)
		}
		if it.BasePrice < 0 {
			t.Errorf("item %d BasePrice: got %d, want >= 0", it.ItemID, it.BasePrice)
		}
	}
}

func TestGetNPCMerchant_NotAMerchant(t *testing.T) {
	d := openTestDB(t)

	// Find an NPC with merchant_id = 0 and confirm we report nil rather than an
	// empty inventory, so the UI hides the section entirely.
	var id int
	if err := d.QueryRow(
		`SELECT id FROM npc_types WHERE merchant_id = 0 LIMIT 1`,
	).Scan(&id); err != nil {
		t.Skipf("no non-merchant NPC available: %v", err)
	}

	m, err := d.GetNPCMerchant(id)
	if err != nil {
		t.Fatalf("get merchant for %d: %v", id, err)
	}
	if m != nil {
		t.Errorf("npc %d: got merchant %+v, want nil", id, m)
	}
}

func TestGetNPCMerchant_UnknownNPC(t *testing.T) {
	d := openTestDB(t)

	m, err := d.GetNPCMerchant(-1)
	if err != nil {
		t.Fatalf("get merchant for unknown npc: %v", err)
	}
	if m != nil {
		t.Errorf("unknown npc: got merchant %+v, want nil", m)
	}
}
