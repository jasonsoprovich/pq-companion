package db_test

import "testing"

func TestGetZoneSpawnPoints(t *testing.T) {
	d := openTestDB(t)
	pts, err := d.GetZoneSpawnPoints("crushbone")
	if err != nil {
		t.Fatal(err)
	}
	if len(pts) == 0 {
		t.Fatal("no spawn points")
	}
	seen := map[int]bool{}
	for _, p := range pts {
		if seen[p.SpawnID] {
			t.Fatalf("spawn point %d listed twice", p.SpawnID)
		}
		seen[p.SpawnID] = true
		if len(p.NPCs) == 0 {
			t.Fatalf("spawn point %d has no NPCs", p.SpawnID)
		}
		total := 0.0
		for _, n := range p.NPCs {
			if n.AggroRadius <= 0 {
				t.Fatalf("%s: aggro radius %d", n.Name, n.AggroRadius)
			}
			if n.Name == "_" || n.Name == "" {
				t.Fatalf("placeholder NPC %+v", n)
			}
			total += n.Pct
		}
		if total < 99.9 || total > 100.1 {
			t.Fatalf("spawn point %d shares sum to %.2f", p.SpawnID, total)
		}
	}
}
