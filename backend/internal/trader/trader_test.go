package trader

import (
	"path/filepath"
	"testing"
	"time"
)

// testdataDir points at the shared game-directory fixture (Feane is the trader).
const testdataDir = "../../../testdata/TAKPv22"

func TestParseBZR(t *testing.T) {
	path := filepath.Join(testdataDir, "BZR_Feane_pq.proj.ini")
	listing, err := ParseBZR(path, "Feane")
	if err != nil {
		t.Fatalf("ParseBZR: %v", err)
	}
	if len(listing.Items) < 50 {
		t.Fatalf("expected many priced items, got %d", len(listing.Items))
	}

	// Spot-check a normal price, a zero (not-for-sale) entry, and a backtick name.
	cases := map[string]int64{
		"Bone Chips":              800,
		"Skull of Jhen`Tra":       0,
		"Gloves of Enrapturement": 600000,
	}
	for name, want := range cases {
		got, ok := listing.priceOf(name)
		if !ok {
			t.Errorf("priceOf(%q): not found", name)
			continue
		}
		if got != want {
			t.Errorf("priceOf(%q) = %d, want %d", name, got, want)
		}
	}

	// Case-insensitive lookup.
	if _, ok := listing.priceOf("bone chips"); !ok {
		t.Errorf("priceOf is not case-insensitive")
	}
}

func TestParseSnapshot(t *testing.T) {
	path := filepath.Join(testdataDir, "Feane-Inventory.txt")
	snap, err := ParseSnapshot(path, "Feane")
	if err != nil {
		t.Fatalf("ParseSnapshot: %v", err)
	}

	// Coin: General-Coin is 0 (must NOT be coerced to 1), Bank-Coin is set.
	if snap.OnPersonCopper != 0 {
		t.Errorf("OnPersonCopper = %d, want 0", snap.OnPersonCopper)
	}
	if snap.BankCopper != 14209255 {
		t.Errorf("BankCopper = %d, want 14209255", snap.BankCopper)
	}

	// Feane has 7 Trader's Satchels (General1-7); General8 is a Small Box and
	// must be excluded. There are 43 non-empty slots across the satchels.
	if len(snap.Satchel) != 43 {
		t.Errorf("satchel has %d entries, want 43", len(snap.Satchel))
	}

	got := make(map[int]string)
	for _, it := range snap.Satchel {
		got[it.ItemID] = it.Name
	}
	// Items that ARE in a Trader's Satchel.
	want := map[int]string{
		32330: "Goranga Spear",          // General1
		10595: "Scaled Wolf Hide Cloak", // General1
		2350:  "Incandescent Mask",      // General1
		2749:  "Damaged Hopper Hide",    // General7 (stack of 13)
	}
	for id, name := range want {
		if got[id] != name {
			t.Errorf("satchel item %d = %q, want %q", id, got[id], name)
		}
	}
	// Items in the Small Box (General8) must NOT be counted as satchel items.
	for _, id := range []int{2627 /* Maelin's */, 9979 /* A Worn Candle */} {
		if _, ok := got[id]; ok {
			t.Errorf("item %d is in a Small Box, not a Trader's Satchel — should be excluded", id)
		}
	}

	// Stacked count is preserved (Damaged Hopper Hide x13).
	for _, it := range snap.Satchel {
		if it.ItemID == 2749 && it.Count != 13 {
			t.Errorf("Damaged Hopper Hide count = %d, want 13", it.Count)
		}
	}
}

func TestInferSales(t *testing.T) {
	listing, err := ParseBZR(filepath.Join(testdataDir, "BZR_Feane_pq.proj.ini"), "Feane")
	if err != nil {
		t.Fatalf("ParseBZR: %v", err)
	}
	prev, err := ParseSnapshot(filepath.Join(testdataDir, "Feane-Inventory.txt"), "Feane")
	if err != nil {
		t.Fatalf("ParseSnapshot: %v", err)
	}

	// Build a "next" snapshot where the Incandescent Mask (350000) sold and the
	// trader gained that much on-person coin.
	const maskID = 2350
	const maskPrice int64 = 350000
	next := &Snapshot{
		Character:      "Feane",
		TakenAt:        prev.TakenAt.Add(time.Hour),
		OnPersonCopper: prev.OnPersonCopper + maskPrice,
		BankCopper:     prev.BankCopper,
	}
	for _, it := range prev.Satchel {
		if it.ItemID == maskID {
			continue // sold — removed from satchel
		}
		next.Satchel = append(next.Satchel, it)
	}

	sess := InferSales(prev, next, listing)

	if len(sess.Sold) != 1 {
		t.Fatalf("expected 1 sold item, got %d (%+v)", len(sess.Sold), sess.Sold)
	}
	sold := sess.Sold[0]
	if sold.ItemID != maskID || sold.Qty != 1 {
		t.Errorf("sold = %+v, want mask id=%d qty=1", sold, maskID)
	}
	if sold.UnitPrice != maskPrice || sold.LineTotal != maskPrice {
		t.Errorf("sold price = %d/%d, want %d", sold.UnitPrice, sold.LineTotal, maskPrice)
	}
	if !sold.Listed {
		t.Errorf("mask should be listed (priced) in BZR")
	}
	if sess.EstimatedRevenue != maskPrice {
		t.Errorf("EstimatedRevenue = %d, want %d", sess.EstimatedRevenue, maskPrice)
	}
	if sess.OnPersonDelta != maskPrice {
		t.Errorf("OnPersonDelta = %d, want %d", sess.OnPersonDelta, maskPrice)
	}
	if !sess.Reconciles {
		t.Errorf("session should reconcile (revenue == coin gained)")
	}
}

func TestFingerprintStableAcrossOrder(t *testing.T) {
	a := &Snapshot{
		Satchel: []SatchelItem{
			{ItemID: 1, Count: 2}, {ItemID: 2, Count: 1},
		},
		OnPersonCopper: 100,
	}
	b := &Snapshot{
		Satchel: []SatchelItem{
			{ItemID: 2, Count: 1}, {ItemID: 1, Count: 2},
		},
		OnPersonCopper: 100,
	}
	if a.Fingerprint() != b.Fingerprint() {
		t.Errorf("fingerprint should ignore satchel ordering")
	}
	b.Satchel[0].Count = 5
	if a.Fingerprint() == b.Fingerprint() {
		t.Errorf("fingerprint should change when a count changes")
	}
}
