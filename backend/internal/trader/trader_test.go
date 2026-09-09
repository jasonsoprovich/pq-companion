package trader

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeExport writes a synthetic tab-delimited inventory export to a temp file
// and returns its path.
func writeExport(t *testing.T, lines ...string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "T-Inventory.txt")
	body := "Location\tName\tID\tCount\tSlots\n" + strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// testdataDir points at the shared game-directory fixture (Feane is the trader).
// These are real game exports under the gitignored testdata/ tree, so they are
// only present on a dev machine — CI skips the fixture-backed tests.
const testdataDir = "../../../testdata/TAKPv22"

// requireFixture skips the test when a shared game-directory fixture is absent
// (e.g. in CI, where testdata/ is gitignored). Mirrors the zeal package pattern.
func requireFixture(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(testdataDir, name)
	if _, err := os.Stat(path); err != nil {
		t.Skipf("testdata fixture not present: %v", err)
	}
	return path
}

func TestParseBZR(t *testing.T) {
	path := requireFixture(t, "BZR_Feane_pq.proj.ini")
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
		"Insignia Protector":      0,
		"Skull of Jhen`Tra":       25000,
		"Gloves of Enrapturement": 400000,
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
	path := requireFixture(t, "Feane-Inventory.txt")
	snap, err := ParseSnapshot(path, "Feane")
	if err != nil {
		t.Fatalf("ParseSnapshot: %v", err)
	}

	// Coin: General-Coin is 0 (must NOT be coerced to 1), Bank-Coin is set.
	if snap.OnPersonCopper != 0 {
		t.Errorf("OnPersonCopper = %d, want 0", snap.OnPersonCopper)
	}
	if snap.BankCopper != 25822255 {
		t.Errorf("BankCopper = %d, want 25822255", snap.BankCopper)
	}

	// Feane has 7 Trader's Satchels (General1-7); General8 is a Small Box and
	// must be excluded. There are 47 non-empty slots across the satchels.
	if len(snap.Satchel) != 47 {
		t.Errorf("satchel has %d entries, want 47", len(snap.Satchel))
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
		10022: "Amber",                  // General7 (stack of 8)
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

	// Stacked count is preserved (Amber x8).
	for _, it := range snap.Satchel {
		if it.ItemID == 10022 && it.Count != 8 {
			t.Errorf("Amber count = %d, want 8", it.Count)
		}
	}
}

func TestInferSales(t *testing.T) {
	listing, err := ParseBZR(requireFixture(t, "BZR_Feane_pq.proj.ini"), "Feane")
	if err != nil {
		t.Fatalf("ParseBZR: %v", err)
	}
	prev, err := ParseSnapshot(requireFixture(t, "Feane-Inventory.txt"), "Feane")
	if err != nil {
		t.Fatalf("ParseSnapshot: %v", err)
	}

	// Build a "next" snapshot where the Incandescent Mask (250000) sold and the
	// trader gained that much on-person coin.
	const maskID = 2350
	const maskPrice int64 = 250000
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

// A trader who banks their earnings between snapshots shows a negative
// on-person delta even though sales happened. Reconciliation must fall back to
// the bank-inclusive total delta so the session still reads as "coin matches".
func TestInferSalesReconcilesAcrossBanking(t *testing.T) {
	listing := &BZRListing{
		Character: "T",
		Items:     []PricedItem{{Name: "Widget", Price: 5000}},
	}
	prev := &Snapshot{
		Character:      "T",
		Satchel:        []SatchelItem{{Bag: 1, Slot: 1, ItemID: 42, Name: "Widget", Count: 1}},
		OnPersonCopper: 742222, // platinum carried in the pocket before parking
		BankCopper:     100000,
	}
	next := &Snapshot{
		Character:      "T",
		TakenAt:        prev.TakenAt.Add(time.Hour),
		Satchel:        []SatchelItem{},        // Widget sold
		OnPersonCopper: 0,                      // logged back in, banked everything
		BankCopper:     100000 + 742222 + 5000, // prior bank + pocket + sale
	}

	sess := InferSales(prev, next, listing)

	if sess.OnPersonDelta != -742222 {
		t.Errorf("OnPersonDelta = %d, want -742222", sess.OnPersonDelta)
	}
	if sess.TotalCoinDelta != 5000 {
		t.Errorf("TotalCoinDelta = %d, want 5000", sess.TotalCoinDelta)
	}
	if sess.CoinGained != 5000 {
		t.Errorf("CoinGained = %d, want 5000 (bank-inclusive)", sess.CoinGained)
	}
	if sess.EstimatedRevenue != 5000 {
		t.Errorf("EstimatedRevenue = %d, want 5000", sess.EstimatedRevenue)
	}
	if !sess.Reconciles {
		t.Errorf("session should reconcile against the total coin delta despite the negative on-person delta")
	}
}

// Bank Trader's Satchels are parsed and tagged Vault; items in a non-satchel
// bank bag (a Backpack) are still excluded.
func TestParseSnapshotBankSatchel(t *testing.T) {
	path := writeExport(t,
		"General1\tTrader's Satchel\t17899\t1\t10",
		"General1-Slot1\tBrick of Ore\t5001\t4\t0",
		"General1-Slot2\tEmpty\t0\t0\t0",
		"Bank1\tTrader's Satchel\t17899\t1\t10",
		"Bank1-Slot1\tRusty Dagger\t5002\t1\t0",
		"Bank2\tBackpack\t17005\t1\t8",
		"Bank2-Slot1\tPearl\t5003\t9\t0",
		"General-Coin\tCurrency\t0\t0\t0",
		"Bank-Coin\tCurrency\t0\t123456\t0",
	)
	snap, err := ParseSnapshot(path, "T")
	if err != nil {
		t.Fatalf("ParseSnapshot: %v", err)
	}

	var bar, vault []SatchelItem
	for _, it := range snap.Satchel {
		if it.OnBar() {
			bar = append(bar, it)
		} else {
			vault = append(vault, it)
		}
	}
	if len(bar) != 1 || bar[0].ItemID != 5001 || bar[0].Count != 4 {
		t.Errorf("on-bar = %+v, want one Brick of Ore x4", bar)
	}
	if len(vault) != 1 || vault[0].ItemID != 5002 {
		t.Errorf("vault = %+v, want one Rusty Dagger", vault)
	}
	for _, it := range snap.Satchel {
		if it.ItemID == 5003 {
			t.Errorf("Pearl is in a bank Backpack, not a Trader's Satchel — should be excluded")
		}
	}
	if snap.BankCopper != 123456 {
		t.Errorf("BankCopper = %d, want 123456", snap.BankCopper)
	}
}

// Pulling an unsold item off the bar into a bank satchel must not read as a
// sale: only the copy that actually left the character's satchels counts.
func TestInferSalesNetsBarToVaultMove(t *testing.T) {
	listing := &BZRListing{
		Character: "T",
		Items:     []PricedItem{{Name: "Widget", Price: 5000}},
	}
	prev := &Snapshot{
		Satchel: []SatchelItem{
			{Bag: 1, Slot: 1, ItemID: 1, Name: "Widget", Count: 2},
			{Bag: 1, Slot: 2, ItemID: 2, Name: "Gadget", Count: 1},
		},
	}
	next := &Snapshot{
		TakenAt: prev.TakenAt.Add(time.Hour),
		Satchel: []SatchelItem{
			{Bag: 1, Slot: 1, ItemID: 1, Name: "Widget", Count: 1, Vault: true},
			{Bag: 1, Slot: 2, ItemID: 2, Name: "Gadget", Count: 1},
		},
		OnPersonCopper: 5000, // one Widget sold
	}

	sess := InferSales(prev, next, listing)

	if len(sess.Sold) != 1 || sess.Sold[0].ItemID != 1 || sess.Sold[0].Qty != 1 {
		t.Fatalf("Sold = %+v, want exactly one Widget qty 1 (the bar→vault move must not count)", sess.Sold)
	}
	if len(sess.Restocked) != 0 {
		t.Errorf("Restocked = %+v, want none", sess.Restocked)
	}
	if sess.EstimatedRevenue != 5000 || !sess.Reconciles {
		t.Errorf("revenue = %d reconciles = %v, want 5000/true", sess.EstimatedRevenue, sess.Reconciles)
	}
}

// A leftover "before" export from a previous trip diffs as a pile of items
// leaving with no priced revenue and no coin — that pairing should be flagged.
func TestInferSalesFlagsStaleBeforeSnapshot(t *testing.T) {
	prev := &Snapshot{
		Satchel: []SatchelItem{
			{Bag: 1, Slot: 1, ItemID: 10, Name: "Old Relic", Count: 1},
			{Bag: 1, Slot: 2, ItemID: 11, Name: "Dusty Tome", Count: 1},
		},
	}
	next := &Snapshot{
		TakenAt: prev.TakenAt.Add(30 * time.Minute), // recent gap, so the flag is content-driven
		Satchel: []SatchelItem{},                    // everything "gone"
	}

	sess := InferSales(prev, next, nil) // no BZR listing → nothing priced

	if !sess.Suspect {
		t.Fatalf("expected Suspect=true for an unpriced, coinless mass-removal diff")
	}
	joined := strings.Join(sess.Caveats, " ")
	if !strings.Contains(joined, "stale \"before\" snapshot") {
		t.Errorf("caveats missing the stale-snapshot guidance: %q", joined)
	}

	// A normal reconciled session is not flagged.
	listing := &BZRListing{Character: "T", Items: []PricedItem{{Name: "Old Relic", Price: 4000}}}
	next2 := &Snapshot{
		TakenAt:        prev.TakenAt.Add(time.Hour),
		Satchel:        []SatchelItem{{Bag: 1, Slot: 2, ItemID: 11, Name: "Dusty Tome", Count: 1}},
		OnPersonCopper: 4000,
	}
	if InferSales(prev, next2, listing).Suspect {
		t.Errorf("a reconciled session should not be flagged Suspect")
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
