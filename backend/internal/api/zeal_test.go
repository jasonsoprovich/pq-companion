package api

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
	"github.com/jasonsoprovich/pq-companion/backend/internal/zeal"
)

// TestEnrichEntries verifies the inventory-icon join end-to-end by parsing
// a real Zeal export from testdata/ and running the same enrichment that
// the API handlers use. Confirms that every item with a non-zero items.icon
// in the DB gets that icon populated on the corresponding InventoryEntry.
func TestEnrichEntries(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(file), "..", "..", "..")

	invPath := filepath.Join(repoRoot, "testdata", "Osui-Inventory.txt")
	if _, err := os.Stat(invPath); os.IsNotExist(err) {
		t.Skipf("testdata fixture %s not present", invPath)
	}
	dbPath := filepath.Join(repoRoot, "backend", "data", "quarm.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Skipf("quarm.db not present at %s", dbPath)
	}

	d, err := db.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	defer d.Close()

	inv, err := zeal.ParseInventory(invPath, "Osui")
	if err != nil {
		t.Fatalf("parse inventory: %v", err)
	}
	if len(inv.Entries) == 0 {
		t.Fatal("expected entries from Osui-Inventory.txt")
	}

	h := &zealHandler{db: d}
	h.enrichEntries(inv.Entries)

	var realItems, withIcon int
	for _, e := range inv.Entries {
		if e.ID > 0 {
			realItems++
			if e.Icon > 0 {
				withIcon++
			}
		}
	}
	if realItems == 0 {
		t.Fatal("expected at least one entry with id > 0")
	}
	// Every real item in Osui's inventory should have a matching icon in the DB.
	// Tolerate a small handful of misses in case the items table is ever pruned
	// (currently 0 misses for this fixture).
	if withIcon < realItems-3 {
		t.Errorf("only %d/%d real items got icons; expected ~all", withIcon, realItems)
	}
	t.Logf("enriched %d/%d real items with icons (out of %d total entries)", withIcon, realItems, len(inv.Entries))
}
