package mystats

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "modernc.org/sqlite"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenStore(filepath.Join(t.TempDir(), "user.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestStoreInsertListDelete(t *testing.T) {
	s := openTestStore(t)
	snap, _ := Parse(strings.Split(sampleSK, "\n"))

	id, inserted, err := s.Insert("Kess", 1_700_000_000, sampleSK, snap)
	if err != nil || !inserted || id == 0 {
		t.Fatalf("first insert: id=%d inserted=%v err=%v", id, inserted, err)
	}

	// Identical raw block for the same character is a no-op.
	_, inserted, err = s.Insert("kess", 1_700_000_500, sampleSK, snap)
	if err != nil || inserted {
		t.Fatalf("duplicate insert should be a no-op: inserted=%v err=%v", inserted, err)
	}

	// A changed block stores a second row.
	changed := strings.Replace(sampleSK, "Avoidance: 558 (with AAs)", "Avoidance: 601 (with AAs)", 1)
	snap2, _ := Parse(strings.Split(changed, "\n"))
	id2, inserted, err := s.Insert("Kess", 1_700_100_000, changed, snap2)
	if err != nil || !inserted {
		t.Fatalf("changed insert: inserted=%v err=%v", inserted, err)
	}

	list, err := s.List("Kess")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("List returned %d, want 2", len(list))
	}
	if list[0].ID != id2 || list[1].ID != id {
		t.Errorf("List not newest-first: got ids %d,%d", list[0].ID, list[1].ID)
	}
	if list[0].Snapshot.Avoidance != 601 || list[1].Snapshot.Avoidance != 558 {
		t.Errorf("snapshot JSON round-trip wrong: %d / %d", list[0].Snapshot.Avoidance, list[1].Snapshot.Avoidance)
	}
	if list[1].Raw != sampleSK {
		t.Errorf("raw text not preserved")
	}

	// Delete is character-scoped.
	if err := s.Delete("Kess", id); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := s.Delete("SomeoneElse", id2); err != nil {
		t.Fatalf("cross-character Delete should be a silent no-op: %v", err)
	}
	list, _ = s.List("Kess")
	if len(list) != 1 || list[0].ID != id2 {
		t.Errorf("after delete: %+v", list)
	}

	// Empty character name never inserts.
	if _, inserted, _ := s.Insert("", 1, "x", Snapshot{}); inserted {
		t.Errorf("empty character insert should be a no-op")
	}
}

func TestConsumerBuildsSnapshotFromLineStream(t *testing.T) {
	s := openTestStore(t)
	c := NewConsumer(s, func() string { return "Kess" })
	var updates []Update
	c.SetOnUpdate(func(u Update) { updates = append(updates, u) })

	ts := time.Date(2026, 9, 8, 13, 51, 6, 0, time.UTC)
	// Some unrelated chatter, then the block, then a line that closes it.
	c.HandleLine(ts.Add(-3*time.Second), "Kess begins to cast a spell.")
	for _, l := range strings.Split(sampleSK, "\n") {
		c.HandleLine(ts, l)
	}
	c.HandleLine(ts.Add(5*time.Second), "You no longer have a target.")

	if len(updates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(updates))
	}
	list, _ := s.List("Kess")
	if len(list) != 1 {
		t.Fatalf("expected 1 stored snapshot, got %d", len(list))
	}
	got := list[0].Snapshot
	if got.ACDisplay != 1783 || len(got.Melee) != 1 || got.Melee[0].DPSHasteAvg != 56.00 {
		t.Errorf("consumer-built snapshot wrong: %+v", got)
	}
	if list[0].CapturedAt != ts.Unix() {
		t.Errorf("captured_at = %d, want block-start %d", list[0].CapturedAt, ts.Unix())
	}
}

func TestConsumerBackToBackBlocks(t *testing.T) {
	s := openTestStore(t)
	c := NewConsumer(s, func() string { return "Kess" })

	ts := time.Now()
	feed := func(text string, at time.Time) {
		for _, l := range strings.Split(text, "\n") {
			c.HandleLine(at, l)
		}
	}
	feed(sampleSK, ts)
	// A second /mystats immediately, with one field changed, opens a new block
	// and must finalize the first even with no separator line between them.
	changed := strings.Replace(sampleSK, "To Hit: 457", "To Hit: 470", 1)
	feed(changed, ts.Add(30*time.Second))
	c.Flush()

	list, _ := s.List("Kess")
	if len(list) != 2 {
		t.Fatalf("expected 2 snapshots from back-to-back blocks, got %d", len(list))
	}
}

func TestConsumerNoActiveCharacterDrops(t *testing.T) {
	s := openTestStore(t)
	c := NewConsumer(s, func() string { return "" })
	for _, l := range strings.Split(sampleSK, "\n") {
		c.HandleLine(time.Now(), l)
	}
	c.Flush()
	if list, _ := s.List("Kess"); len(list) != 0 {
		t.Errorf("snapshot stored with no active character: %+v", list)
	}
}
