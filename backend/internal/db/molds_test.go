package db_test

import (
	"strings"
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
)

func TestMoldIndex_Counts(t *testing.T) {
	d := openTestDB(t)

	tiers := map[string]int{}
	types := map[string]bool{}
	products := 0
	molds := map[int]bool{}
	for _, id := range d.TradeableMoldProductIDs() {
		m, ok := d.ProductMold(id)
		if !ok {
			t.Fatalf("tradeable product %d has no mold", id)
		}
		if m.NoDrop == 0 {
			t.Fatalf("product %d listed tradeable but mold %q is NO DROP", id, m.Name)
		}
	}

	// Walk every mold via the products of each mold name candidate.
	rows, err := d.Query(`SELECT id, Name FROM items
	  WHERE (Name LIKE 'Ornate %' OR Name LIKE 'Elemental %' OR Name LIKE 'Timeless %')
	    AND (Name LIKE '%Mold' OR Name LIKE '%Pattern')`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			t.Fatal(err)
		}
		ps := d.MoldProducts(id)
		if len(ps) == 0 {
			continue
		}
		molds[id] = true
		tiers[strings.Fields(name)[0]]++
		for _, kind := range []string{"Chain", "Leather", "Silk"} {
			if strings.Contains(name, " "+kind+" ") {
				types[kind] = true
			}
		}
		if strings.HasSuffix(name, "Mold") {
			types["Plate"] = true
		}
		for _, p := range ps {
			products++
			if m, ok := d.ProductMold(p.ItemID); !ok || m.ID != id {
				t.Errorf("product %q does not map back to mold %q", p.Name, name)
			}
			if p.ClassLabel == "" {
				t.Errorf("product %q has no class label", p.Name)
			}
			if p.Container.ItemID == 0 {
				t.Errorf("product %q has no container", p.Name)
			}
		}
	}
	if len(molds) != 56 || tiers["Ornate"] != 28 || tiers["Elemental"] != 24 || tiers["Timeless"] != 4 {
		t.Errorf("molds = %d %v, want 56 (28/24/4)", len(molds), tiers)
	}
	if products != 210 {
		t.Errorf("products = %d, want 210", products)
	}
	if len(types) != 4 {
		t.Errorf("armor types covered = %v, want plate/chain/leather/silk", types)
	}
}

func TestMoldIndex_ElementalChainSleeve(t *testing.T) {
	d := openTestDB(t)
	const pattern = 16364 // Elemental Chain Sleeve Pattern

	got := map[string]string{}
	for _, p := range d.MoldProducts(pattern) {
		got[p.ClassLabel] = p.Name
		if p.Container.ItemID != 17184 {
			t.Errorf("%q container = %+v, want Mystical Furnace of Ro", p.Name, p.Container)
		}
		if len(p.Tools) != 1 || p.Tools[0].ItemID != 29546 {
			t.Errorf("%q tools = %+v, want Tanaan Smithing Hammer", p.Name, p.Tools)
		}
		var rings int
		for _, in := range p.Inputs {
			if in.ItemID == 29546 || in.ItemID == pattern {
				t.Errorf("%q input list contains %q", p.Name, in.Name)
			}
			if in.ItemID == 16284 {
				rings = in.Count
			}
		}
		if rings != 2 {
			t.Errorf("%q Ethereal Metal Rings count = %d, want 2", p.Name, rings)
		}
	}
	want := map[string]string{
		"Ranger": "Askr's Thunderous Armplates",
		"Rogue":  "Bidilis' Sleeves of the Elusive",
		"Shaman": "Rosrak's Sleeves of the Primal",
	}
	if len(got) != len(want) {
		t.Fatalf("products = %v, want %v", got, want)
	}
	for class, name := range want {
		if got[class] != name {
			t.Errorf("%s → %q, want %q", class, got[class], name)
		}
	}

	m, ok := d.ProductMold(11447)
	if !ok || m.ID != pattern || m.NoDrop != 0 {
		t.Errorf("ProductMold(Askr's) = %+v %v, want NO DROP pattern %d", m, ok, pattern)
	}
}

// Mold products are all NO DROP; "Hide NO DROP" judges them by their mold, so
// the Ornate (tradeable mold) product stays while the Elemental (NO DROP mold)
// one goes — in the candidate query, ahead of any scoring.
func TestUpgradeCandidates_NoDropJudgesMoldProductsByMold(t *testing.T) {
	d := openTestDB(t)
	const (
		armsBit    = 128
		rangerBit  = 8
		ornateArms = 11446 // Planestriders Vambraces (Ornate Chain Sleeve Pattern)
		elemArms   = 11447 // Askr's Thunderous Armplates (Elemental Chain Sleeve Pattern)
	)
	cands, err := d.UpgradeCandidates(db.CandidateFilter{
		SlotMask: armsBit, ClassBit: rangerBit, MaxLevel: 65, ExcludeNoDrop: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	seen := map[int]bool{}
	for _, c := range cands {
		seen[c.ID] = true
		if c.NoDrop == 0 && c.ID != ornateArms {
			if _, ok := d.ProductMold(c.ID); !ok {
				t.Errorf("NO DROP non-mold item %q (%d) passed the filter", c.Name, c.ID)
			}
		}
	}
	if !seen[ornateArms] {
		t.Error("Ornate mold product filtered out despite tradeable mold")
	}
	if seen[elemArms] {
		t.Error("Elemental mold product kept despite NO DROP mold")
	}
}

// NPC loot tables carry the class items a dropped mold combines into.
func TestGetNPCLoot_AnnotatesMolds(t *testing.T) {
	d := openTestDB(t)
	var npcID int
	err := d.QueryRow(`SELECT n.id FROM npc_types n
	  JOIN loottable_entries lt ON lt.loottable_id = n.loottable_id
	  JOIN lootdrop_entries l ON l.lootdrop_id = lt.lootdrop_id
	  WHERE l.item_id = 16356 LIMIT 1`).Scan(&npcID)
	if err != nil {
		t.Fatalf("find an NPC dropping Ornate Silk Sleeve Pattern: %v", err)
	}
	loot, err := d.GetNPCLoot(npcID)
	if err != nil || loot == nil {
		t.Fatalf("GetNPCLoot(%d): %v", npcID, err)
	}
	found := false
	for _, dr := range append(loot.Drops, loot.ZoneWideDrops...) {
		for _, it := range dr.Items {
			if it.ItemID == 16356 {
				found = true
				if len(it.MoldProducts) != 4 {
					t.Errorf("mold products = %+v, want the 4 silk classes", it.MoldProducts)
				}
			} else if len(it.MoldProducts) > 0 {
				if _, ok := d.ProductMold(it.MoldProducts[0].ItemID); !ok {
					t.Errorf("non-mold %q annotated", it.ItemName)
				}
			}
		}
	}
	if !found {
		t.Fatal("mold not in the NPC's loot")
	}
}
