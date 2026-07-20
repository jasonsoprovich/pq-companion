package character

import "testing"

func TestUpsertFactionTally_InsertThenUpdate(t *testing.T) {
	s, charID := openTestStore(t)

	if err := s.UpsertFactionTally(FactionTallyRow{
		CharacterID: charID, FactionID: 341, FactionName: "Priests of Life",
		Better: 0, Worse: 1, EstimatedNet: -100,
	}); err != nil {
		t.Fatalf("first upsert: %v", err)
	}
	rows, err := s.ListFactionTallies(charID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rows) != 1 || rows[0].Worse != 1 || rows[0].EstimatedNet != -100 {
		t.Fatalf("rows = %+v, want one row Worse=1 EstimatedNet=-100", rows)
	}

	// Second upsert for the same (character, faction) must update in place,
	// not insert a duplicate row.
	if err := s.UpsertFactionTally(FactionTallyRow{
		CharacterID: charID, FactionID: 341, FactionName: "Priests of Life",
		Better: 0, Worse: 2, EstimatedNet: -200,
		LastBucket: "dubious", LastConsideredAt: 1700000000, LastConsiderSuspect: true,
	}); err != nil {
		t.Fatalf("second upsert: %v", err)
	}
	rows, err = s.ListFactionTallies(charID)
	if err != nil {
		t.Fatalf("list after update: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %+v, want exactly 1 (update in place, not a new row)", rows)
	}
	got := rows[0]
	if got.Worse != 2 || got.EstimatedNet != -200 || got.LastBucket != "dubious" ||
		got.LastConsideredAt != 1700000000 || !got.LastConsiderSuspect {
		t.Errorf("row = %+v, want updated fields", got)
	}
}

func TestListFactionTallies_ScopedToCharacter(t *testing.T) {
	s, charA := openTestStore(t)
	charB, err := s.Create("Otherchar", 0, 1, 60)
	if err != nil {
		t.Fatalf("create second char: %v", err)
	}

	if err := s.UpsertFactionTally(FactionTallyRow{CharacterID: charA, FactionID: 1, FactionName: "A"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertFactionTally(FactionTallyRow{CharacterID: charB.ID, FactionID: 2, FactionName: "B"}); err != nil {
		t.Fatal(err)
	}

	rowsA, err := s.ListFactionTallies(charA)
	if err != nil {
		t.Fatal(err)
	}
	if len(rowsA) != 1 || rowsA[0].FactionID != 1 {
		t.Errorf("charA tallies = %+v, want exactly faction 1", rowsA)
	}
}

func TestDeleteFactionTally_RemovesOnlyThatFaction(t *testing.T) {
	s, charID := openTestStore(t)
	if err := s.UpsertFactionTally(FactionTallyRow{CharacterID: charID, FactionID: 1, FactionName: "A"}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertFactionTally(FactionTallyRow{CharacterID: charID, FactionID: 2, FactionName: "B"}); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteFactionTally(charID, 1); err != nil {
		t.Fatalf("delete: %v", err)
	}
	rows, err := s.ListFactionTallies(charID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].FactionID != 2 {
		t.Errorf("rows = %+v, want only faction 2 left", rows)
	}
}

func TestClearFactionTallies_RemovesAllForCharacter(t *testing.T) {
	s, charID := openTestStore(t)
	if err := s.UpsertFactionTally(FactionTallyRow{CharacterID: charID, FactionID: 1, FactionName: "A", Worse: 3}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpsertFactionTally(FactionTallyRow{CharacterID: charID, FactionID: 2, FactionName: "B", Better: 5}); err != nil {
		t.Fatal(err)
	}
	if err := s.ClearFactionTallies(charID); err != nil {
		t.Fatalf("clear: %v", err)
	}
	rows, err := s.ListFactionTallies(charID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Errorf("rows = %+v, want empty after ClearFactionTallies", rows)
	}
}
