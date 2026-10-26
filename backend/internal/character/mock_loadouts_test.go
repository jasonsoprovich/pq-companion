package character

import "testing"

func TestMockLoadouts_CRUD(t *testing.T) {
	s, charID := openTestStore(t)

	if got, err := s.ListMockLoadouts(charID); err != nil || len(got) != 0 {
		t.Fatalf("empty list: got %v err %v", got, err)
	}

	gear := map[string]int{"head": 1001, "ear2": 0}
	aas := map[int]int{21: 3, 113: 0}
	created, err := s.CreateMockLoadout(charID, "Raid buffs", gear, aas)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.Gear["head"] != 1001 || created.AAs[21] != 3 {
		t.Fatalf("created round-trip = %+v", created)
	}
	// An explicit 0 (empty slot / untrained AA) must survive the round-trip —
	// it's an override, not "keep actual".
	if v, ok := created.Gear["ear2"]; !ok || v != 0 {
		t.Errorf("ear2 empty-slot override lost: %+v", created.Gear)
	}
	if v, ok := created.AAs[113]; !ok || v != 0 {
		t.Errorf("AA 113 untrained override lost: %+v", created.AAs)
	}

	if _, err := s.CreateMockLoadout(charID, "Raid buffs", nil, nil); err != ErrMockLoadoutNameTaken {
		t.Errorf("duplicate name err = %v, want ErrMockLoadoutNameTaken", err)
	}

	updated, ok, err := s.UpdateMockLoadout(charID, created.ID, "Group", map[string]int{"neck": 7}, nil)
	if err != nil || !ok {
		t.Fatalf("update: ok=%v err=%v", ok, err)
	}
	if updated.Name != "Group" || updated.Gear["neck"] != 7 || len(updated.Gear) != 1 || len(updated.AAs) != 0 {
		t.Errorf("updated = %+v", updated)
	}

	// Another character can't see or edit it.
	other, err := s.Create("Otherchar", 0, 1, 60)
	if err != nil {
		t.Fatalf("create other: %v", err)
	}
	if _, ok, _ := s.GetMockLoadout(other.ID, created.ID); ok {
		t.Error("loadout visible to another character")
	}
	if _, ok, _ := s.UpdateMockLoadout(other.ID, created.ID, "x", nil, nil); ok {
		t.Error("loadout editable by another character")
	}

	if err := s.DeleteMockLoadout(charID, created.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if got, _ := s.ListMockLoadouts(charID); len(got) != 0 {
		t.Errorf("after delete: %v", got)
	}
}
