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

// TestMergeBackfillFactionTally_InsertsWhenNoExistingRow checks a backfill
// merge against a faction never seen before is a plain insert.
func TestMergeBackfillFactionTally_InsertsWhenNoExistingRow(t *testing.T) {
	s, charID := openTestStore(t)
	changed, err := s.MergeBackfillFactionTally(FactionTallyRow{
		CharacterID: charID, FactionID: 341, FactionName: "Priests of Life",
		Better: 0, Worse: 3, EstimatedNet: -300,
	})
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
	if len(rows) != 1 || rows[0].Worse != 3 || rows[0].EstimatedNet != -300 {
		t.Fatalf("rows = %+v, want one row Worse=3 EstimatedNet=-300", rows)
	}
}

// TestMergeBackfillFactionTally_KeepsHigherExistingCount is the core
// idempotency guarantee: a backfill replay that saw FEWER total events than
// what's already persisted (e.g. the log was rotated since the live
// tracker counted more of it) must never regress the stored counters.
func TestMergeBackfillFactionTally_KeepsHigherExistingCount(t *testing.T) {
	s, charID := openTestStore(t)
	if err := s.UpsertFactionTally(FactionTallyRow{
		CharacterID: charID, FactionID: 341, FactionName: "Priests of Life",
		Better: 2, Worse: 5, EstimatedNet: -400, Unresolved: 1,
	}); err != nil {
		t.Fatal(err)
	}

	changed, err := s.MergeBackfillFactionTally(FactionTallyRow{
		CharacterID: charID, FactionID: 341, FactionName: "Priests of Life",
		Better: 0, Worse: 3, EstimatedNet: -300, // 3 total < 7 total already stored
	})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if changed {
		t.Error("expected changed=false — backfill saw fewer events than already stored")
	}
	rows, _ := s.ListFactionTallies(charID)
	if len(rows) != 1 || rows[0].Better != 2 || rows[0].Worse != 5 || rows[0].EstimatedNet != -400 {
		t.Errorf("rows = %+v, want existing counters untouched", rows)
	}
}

// TestMergeBackfillFactionTally_ReplacesWhenBackfillSawMore is the
// complementary case: a full-log backfill that saw MORE than the currently
// persisted counters (e.g. the character was never live-tracked before)
// replaces them wholesale.
func TestMergeBackfillFactionTally_ReplacesWhenBackfillSawMore(t *testing.T) {
	s, charID := openTestStore(t)
	if err := s.UpsertFactionTally(FactionTallyRow{
		CharacterID: charID, FactionID: 341, FactionName: "Priests of Life",
		Better: 1, Worse: 1, EstimatedNet: -10,
	}); err != nil {
		t.Fatal(err)
	}

	changed, err := s.MergeBackfillFactionTally(FactionTallyRow{
		CharacterID: charID, FactionID: 341, FactionName: "Priests of Life",
		Better: 4, Worse: 6, EstimatedNet: -500, Unresolved: 2, // 10 total > 2 total stored
	})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if !changed {
		t.Error("expected changed=true — backfill saw more events than stored")
	}
	rows, _ := s.ListFactionTallies(charID)
	if len(rows) != 1 || rows[0].Better != 4 || rows[0].Worse != 6 || rows[0].EstimatedNet != -500 || rows[0].Unresolved != 2 {
		t.Errorf("rows = %+v, want replaced with backfill's larger counters", rows)
	}
}

// TestMergeBackfillFactionTally_RepeatedRunIsNoop verifies re-running a
// backfill against the same (unchanged) log a second time is a true no-op —
// the safety guarantee the Settings → Log Backfill panel promises for every
// tracker ("re-running is always safe").
func TestMergeBackfillFactionTally_RepeatedRunIsNoop(t *testing.T) {
	s, charID := openTestStore(t)
	row := FactionTallyRow{
		CharacterID: charID, FactionID: 341, FactionName: "Priests of Life",
		Better: 2, Worse: 5, EstimatedNet: -400,
	}
	if _, err := s.MergeBackfillFactionTally(row); err != nil {
		t.Fatalf("first merge: %v", err)
	}
	changed, err := s.MergeBackfillFactionTally(row)
	if err != nil {
		t.Fatalf("second merge: %v", err)
	}
	if changed {
		t.Error("expected changed=false on an identical second run")
	}
}

// TestMergeBackfillFactionTally_NewerConsiderReadingWins checks the
// last_bucket/last_considered_at fields merge independently of the
// better/worse counter comparison — whichever /con reading is
// chronologically newer always wins.
func TestMergeBackfillFactionTally_NewerConsiderReadingWins(t *testing.T) {
	s, charID := openTestStore(t)
	if err := s.UpsertFactionTally(FactionTallyRow{
		CharacterID: charID, FactionID: 262, FactionName: "Guards of Qeynos",
		Better: 5, Worse: 5, // higher event count than the backfill below
		LastBucket: "kindly", LastConsideredAt: 1000,
	}); err != nil {
		t.Fatal(err)
	}

	changed, err := s.MergeBackfillFactionTally(FactionTallyRow{
		CharacterID: charID, FactionID: 262, FactionName: "Guards of Qeynos",
		Better: 1, Worse: 1, // fewer events — counters must NOT replace
		LastBucket: "dubious", LastConsideredAt: 2000, // but this reading is newer
	})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	if !changed {
		t.Error("expected changed=true — the /con reading advanced even though counters didn't")
	}
	rows, _ := s.ListFactionTallies(charID)
	got := rows[0]
	if got.Better != 5 || got.Worse != 5 {
		t.Errorf("counters = %+v, want untouched (backfill saw fewer events)", got)
	}
	if got.LastBucket != "dubious" || got.LastConsideredAt != 2000 {
		t.Errorf("bucket = %+v, want the newer dubious/2000 reading", got)
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
