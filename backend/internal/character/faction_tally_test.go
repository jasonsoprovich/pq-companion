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

// TestMergeBackfillConsiderReading_InsertsWhenNoExistingRow checks a backfill
// /con reading for a faction never seen before creates a zero-count row
// seeded with just the bucket/timestamp.
func TestMergeBackfillConsiderReading_InsertsWhenNoExistingRow(t *testing.T) {
	s, charID := openTestStore(t)
	changed, err := s.MergeBackfillConsiderReading(charID, 341, "Priests of Life", "dubious", 1000)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if !changed {
		t.Error("expected changed=true for a brand-new row")
	}
	rows, err := s.ListFactionTallies(charID)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].LastBucket != "dubious" || rows[0].LastConsideredAt != 1000 {
		t.Fatalf("rows = %+v, want one row bucket=dubious at=1000", rows)
	}
	if rows[0].Better != 0 || rows[0].Worse != 0 || rows[0].EstimatedNet != 0 {
		t.Errorf("rows = %+v, want zero counters — backfill never touches them", rows)
	}
}

// TestMergeBackfillConsiderReading_NewerReadingReplaces checks a backfilled
// reading newer than what's stored advances the baseline, without touching
// the existing better/worse/estimated_net counters at all.
func TestMergeBackfillConsiderReading_NewerReadingReplaces(t *testing.T) {
	s, charID := openTestStore(t)
	if err := s.UpsertFactionTally(FactionTallyRow{
		CharacterID: charID, FactionID: 341, FactionName: "Priests of Life",
		Better: 2, Worse: 5, EstimatedNet: -400,
		LastBucket: "kindly", LastConsideredAt: 1000, LastConsiderSuspect: true,
	}); err != nil {
		t.Fatal(err)
	}

	changed, err := s.MergeBackfillConsiderReading(charID, 341, "Priests of Life", "dubious", 2000)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if !changed {
		t.Error("expected changed=true — the backfilled reading is newer")
	}
	rows, _ := s.ListFactionTallies(charID)
	got := rows[0]
	if got.LastBucket != "dubious" || got.LastConsideredAt != 2000 {
		t.Errorf("bucket = %+v, want the newer dubious/2000 reading", got)
	}
	if got.LastConsiderSuspect {
		t.Error("expected last_consider_suspect cleared — backfill can never confirm illusion state")
	}
	if got.Better != 2 || got.Worse != 5 || got.EstimatedNet != -400 {
		t.Errorf("counters = %+v, want untouched — backfill never touches better/worse/estimated_net", got)
	}
}

// TestMergeBackfillConsiderReading_OlderReadingIsNoop checks a backfilled
// reading OLDER than (or equal to) what's already stored never regresses
// it — the safety guarantee that makes re-running backfill always safe.
func TestMergeBackfillConsiderReading_OlderReadingIsNoop(t *testing.T) {
	s, charID := openTestStore(t)
	if err := s.UpsertFactionTally(FactionTallyRow{
		CharacterID: charID, FactionID: 341, FactionName: "Priests of Life",
		LastBucket: "dubious", LastConsideredAt: 2000,
	}); err != nil {
		t.Fatal(err)
	}

	changed, err := s.MergeBackfillConsiderReading(charID, 341, "Priests of Life", "kindly", 1000)
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if changed {
		t.Error("expected changed=false — the backfilled reading is older than what's stored")
	}
	rows, _ := s.ListFactionTallies(charID)
	if rows[0].LastBucket != "dubious" || rows[0].LastConsideredAt != 2000 {
		t.Errorf("rows = %+v, want the existing newer reading untouched", rows)
	}

	// Re-running with the exact same reading (the "re-run backfill against
	// an unchanged log" case) is likewise a no-op.
	changed, err = s.MergeBackfillConsiderReading(charID, 341, "Priests of Life", "dubious", 2000)
	if err != nil {
		t.Fatalf("second merge: %v", err)
	}
	if changed {
		t.Error("expected changed=false on an identical re-run")
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
