package db_test

import (
	"strings"
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
)

// TestGetZoneDropsSortFilter covers the Zones → Drops tab's server-side sort
// and filter options. Chardok is large enough (~9k rows) that the tab can only
// page through it correctly if ordering happens in SQL.
func TestGetZoneDropsSortFilter(t *testing.T) {
	d := openTestDB(t)

	all, err := d.GetZoneDrops("chardok", db.ZoneDropsOptions{}, 500, 0)
	if err != nil {
		t.Fatalf("GetZoneDrops: %v", err)
	}
	if all.Total == 0 || len(all.Items) == 0 {
		t.Fatal("expected chardok drops")
	}

	cases := []struct {
		name  string
		opts  db.ZoneDropsOptions
		check func(t *testing.T, items []db.ZoneDropItem)
	}{
		{
			name: "chance desc",
			opts: db.ZoneDropsOptions{Sort: "chance", Desc: true},
			check: func(t *testing.T, items []db.ZoneDropItem) {
				for i := 1; i < len(items); i++ {
					if items[i].Chance > items[i-1].Chance {
						t.Fatalf("row %d chance %.2f > previous %.2f", i, items[i].Chance, items[i-1].Chance)
					}
				}
			},
		},
		{
			name: "chance asc",
			opts: db.ZoneDropsOptions{Sort: "chance"},
			check: func(t *testing.T, items []db.ZoneDropItem) {
				for i := 1; i < len(items); i++ {
					if items[i].Chance < items[i-1].Chance {
						t.Fatalf("row %d chance %.2f < previous %.2f", i, items[i].Chance, items[i-1].Chance)
					}
				}
			},
		},
		{
			name: "npc asc",
			opts: db.ZoneDropsOptions{Sort: "npc"},
			check: func(t *testing.T, items []db.ZoneDropItem) {
				for i := 1; i < len(items); i++ {
					if items[i].NPCName < items[i-1].NPCName {
						t.Fatalf("row %d npc %q < previous %q", i, items[i].NPCName, items[i-1].NPCName)
					}
				}
			},
		},
		{
			name: "min chance",
			opts: db.ZoneDropsOptions{MinChance: 25},
			check: func(t *testing.T, items []db.ZoneDropItem) {
				for _, it := range items {
					if it.Chance < 25 {
						t.Fatalf("%s from %s has chance %.2f below min", it.ItemName, it.NPCName, it.Chance)
					}
				}
			},
		},
		{
			name: "query matches npc with spaces",
			opts: db.ZoneDropsOptions{Query: "a sarnak"},
			check: func(t *testing.T, items []db.ZoneDropItem) {
				for _, it := range items {
					if !strings.Contains(strings.ToLower(it.ItemName), "a sarnak") &&
						!strings.Contains(strings.ToLower(it.NPCName), "a_sarnak") {
						t.Fatalf("row %s / %s does not match query", it.ItemName, it.NPCName)
					}
				}
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			res, err := d.GetZoneDrops("chardok", c.opts, 500, 0)
			if err != nil {
				t.Fatalf("GetZoneDrops: %v", err)
			}
			if len(res.Items) == 0 {
				t.Fatal("expected rows")
			}
			if len(res.Items) > res.Total {
				t.Fatalf("items %d exceed total %d", len(res.Items), res.Total)
			}
			c.check(t, res.Items)
		})
	}

	// Filters must narrow Total, not just the page.
	filtered, err := d.GetZoneDrops("chardok", db.ZoneDropsOptions{MinChance: 25}, 1, 0)
	if err != nil {
		t.Fatalf("GetZoneDrops: %v", err)
	}
	if filtered.Total >= all.Total {
		t.Errorf("min_chance total %d not below unfiltered %d", filtered.Total, all.Total)
	}
}

// TestGetZoneDropsMinChanceUsesEffectiveRate guards against MinChance being
// applied to the raw lootdrop_entries.chance column instead of the effective
// rate. Ssraeshza Temple has many loottables with probability
// under 100, so the two diverge there (unlike Chardok above).
func TestGetZoneDropsMinChanceUsesEffectiveRate(t *testing.T) {
	d := openTestDB(t)

	res, err := d.GetZoneDrops("ssratemple", db.ZoneDropsOptions{MinChance: 25}, 5000, 0)
	if err != nil {
		t.Fatalf("GetZoneDrops: %v", err)
	}
	if len(res.Items) == 0 {
		t.Fatal("expected rows")
	}
	for _, it := range res.Items {
		if it.Chance < 25 {
			t.Fatalf("%s from %s has chance %.2f below min", it.ItemName, it.NPCName, it.Chance)
		}
	}
}
