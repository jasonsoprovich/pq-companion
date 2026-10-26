package api

import (
	"sort"
	"strconv"
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/zeal"
)

func TestMockEquipment(t *testing.T) {
	q := &zeal.QuarmyData{
		Inventory: []zeal.InventoryEntry{
			{Location: "Head", ID: 100},
			{Location: "Ear", ID: 201},
			{Location: "Ear", ID: 202},
			{Location: "General1", ID: 900}, // bag item: never worn
		},
		AAs: []zeal.AAEntry{{ID: 21, Rank: 2}, {ID: 55, Rank: 1}},
	}

	tests := []struct {
		name    string
		o       mockOverrides
		wantInv []string // "Location:ID", sorted
		wantAAs map[int]int
	}{
		{
			name:    "no overrides keeps actual worn gear and AAs",
			wantInv: []string{"Ear:201", "Ear:202", "Head:100"},
			wantAAs: map[int]int{21: 2, 55: 1},
		},
		{
			name:    "override second ear only",
			o:       mockOverrides{Gear: map[string]int{"ear2": 300}},
			wantInv: []string{"Ear:201", "Ear:300", "Head:100"},
			wantAAs: map[int]int{21: 2, 55: 1},
		},
		{
			name:    "zero empties a slot, new slot filled",
			o:       mockOverrides{Gear: map[string]int{"head": 0, "neck": 400}},
			wantInv: []string{"Ear:201", "Ear:202", "Neck:400"},
			wantAAs: map[int]int{21: 2, 55: 1},
		},
		{
			name:    "AA rank raised, another untrained, new one trained",
			o:       mockOverrides{AAs: map[int]int{21: 3, 55: 0, 113: 1}},
			wantInv: []string{"Ear:201", "Ear:202", "Head:100"},
			wantAAs: map[int]int{21: 3, 113: 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inv, aas := mockEquipment(q, tt.o)
			var got []string
			for _, e := range inv {
				got = append(got, e.Location+":"+strconv.Itoa(e.ID))
			}
			sort.Strings(got)
			if len(got) != len(tt.wantInv) {
				t.Fatalf("inventory = %v, want %v", got, tt.wantInv)
			}
			for i := range got {
				if got[i] != tt.wantInv[i] {
					t.Fatalf("inventory = %v, want %v", got, tt.wantInv)
				}
			}
			gotAAs := map[int]int{}
			for _, a := range aas {
				gotAAs[a.ID] = a.Rank
			}
			if len(gotAAs) != len(tt.wantAAs) {
				t.Fatalf("aas = %v, want %v", gotAAs, tt.wantAAs)
			}
			for id, r := range tt.wantAAs {
				if gotAAs[id] != r {
					t.Fatalf("aas = %v, want %v", gotAAs, tt.wantAAs)
				}
			}
		})
	}

	// No export at all: overrides alone define the loadout.
	inv, aas := mockEquipment(nil, mockOverrides{Gear: map[string]int{"head": 5}, AAs: map[int]int{21: 1}})
	if len(inv) != 1 || inv[0].ID != 5 || len(aas) != 1 {
		t.Errorf("nil quarmy: inv=%v aas=%v", inv, aas)
	}
}
