package character

import "testing"

func TestUpdateLastZone(t *testing.T) {
	s, _ := openTestStore(t)

	// Fresh character: no zone recorded yet.
	c, ok, err := s.GetByName("Testchar")
	if err != nil || !ok {
		t.Fatalf("get char: ok=%v err=%v", ok, err)
	}
	if c.LastZone != "" || c.LastZoneAt != 0 {
		t.Fatalf("new char has zone %q@%d, want empty", c.LastZone, c.LastZoneAt)
	}

	if err := s.UpdateLastZone("testchar", "The Overthere", 1_700_000_000); err != nil {
		t.Fatalf("update last zone: %v", err)
	}
	c, _, _ = s.GetByName("Testchar")
	if c.LastZone != "The Overthere" || c.LastZoneAt != 1_700_000_000 {
		t.Fatalf("after update: %q@%d", c.LastZone, c.LastZoneAt)
	}

	// A later sighting in a different zone overwrites both fields (the camp
	// zone is wherever the character was last seen).
	if err := s.UpdateLastZone("TESTCHAR", "Firiona Vie", 1_700_000_500); err != nil {
		t.Fatalf("second update: %v", err)
	}
	c, _, _ = s.GetByName("Testchar")
	if c.LastZone != "Firiona Vie" || c.LastZoneAt != 1_700_000_500 {
		t.Fatalf("after second update: %q@%d", c.LastZone, c.LastZoneAt)
	}

	// List() must carry the fields too.
	list, err := s.List()
	if err != nil || len(list) != 1 {
		t.Fatalf("list: n=%d err=%v", len(list), err)
	}
	if list[0].LastZone != "Firiona Vie" {
		t.Errorf("List()[0].LastZone = %q", list[0].LastZone)
	}

	// Empty name/zone and unknown names are silent no-ops, not errors.
	if err := s.UpdateLastZone("", "Nektulos Forest", 1); err != nil {
		t.Errorf("empty name should be a no-op: %v", err)
	}
	if err := s.UpdateLastZone("Nobody", "", 1); err != nil {
		t.Errorf("empty zone should be a no-op: %v", err)
	}
	if err := s.UpdateLastZone("Nobody", "Nektulos Forest", 1); err != nil {
		t.Errorf("unknown character should be a no-op: %v", err)
	}
	c, _, _ = s.GetByName("Testchar")
	if c.LastZone != "Firiona Vie" {
		t.Errorf("no-op calls changed the row: %q", c.LastZone)
	}
}
