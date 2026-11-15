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
			if n.Look.Race <= 0 {
				t.Fatalf("%s: no race", n.Name)
			}
			if n.Look.Size > 0 && n.Look.BaseSize <= 0 {
				t.Fatalf("%s: size %v but no base size", n.Name, n.Look.Size)
			}
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

// TestSpawnPointLooks checks how a few known NPCs are dressed.
func TestSpawnPointLooks(t *testing.T) {
	d := openTestDB(t)
	pts, err := d.GetZoneSpawnPoints("qeynos")
	if err != nil {
		t.Fatal(err)
	}
	found := 0
	for _, p := range pts {
		for _, n := range p.NPCs {
			switch n.Name {
			case "Guard_Mezzt":
				found++
				// Qeynos citizen model, texture 1 everywhere, helm 1.
				if n.Look.Race != 71 || n.Look.Mats != [7]int{1, 1, 1, 1, 1, 1, 1} {
					t.Errorf("%s: %+v", n.Name, n.Look)
				}
			case "Marlin_Bizmite":
				found++
				if n.Look.Primary != "IT38" {
					t.Errorf("%s: primary %q", n.Name, n.Look.Primary)
				}
			}
		}
	}
	if found < 2 {
		t.Skipf("test fixtures lack the Qeynos NPCs (found %d)", found)
	}
}
