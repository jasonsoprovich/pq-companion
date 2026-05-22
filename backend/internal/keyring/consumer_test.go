package keyring

import (
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func openConsumerTest(t *testing.T, activeChar string) (*Consumer, *Store) {
	t.Helper()
	dir := t.TempDir()
	s, err := OpenStore(filepath.Join(dir, "user.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	master := []MasterEntry{
		{KeyItem: 6378, KeyName: "Bone Crafted Key", ZoneID: 75, ZoneName: "Paineel"},
		{KeyItem: 6379, KeyName: "Hole Key", ZoneID: 75, ZoneName: "Paineel"},
		{KeyItem: 20600, KeyName: "Key to Charasis", ZoneID: 93, ZoneName: "The Overthere"},
		{KeyItem: 19719, KeyName: "Ring of the Shissar", ZoneID: 162, ZoneName: "Ssraeshza Temple"},
		{KeyItem: 20918, KeyName: "Sky: Island 8 (Veeshan)", ZoneID: 71, ZoneName: "Plane of Sky"},
	}
	c := NewConsumer(s, master, func() string { return activeChar })
	return c, s
}

// A non-matching line ends a /keys burst and commits the snapshot.
func TestConsumer_NonMatchingLineFlushesBurst(t *testing.T) {
	c, s := openConsumerTest(t, "Osui")
	base := time.Unix(1_700_000_000, 0)

	// Simulate a /keys burst — all matching lines arrive in the same second.
	c.HandleLine(base, "Bone Crafted Key")
	c.HandleLine(base, "Hole Key")
	c.HandleLine(base, "Ring of the Shissar")
	// Any non-matching line ends the burst and commits.
	c.HandleLine(base.Add(time.Second), "Cymessa begins to cast a spell.")

	got, err := s.ListByCharacter("Osui")
	if err != nil {
		t.Fatalf("ListByCharacter: %v", err)
	}
	ids := make([]int, 0, len(got))
	for _, r := range got {
		ids = append(ids, r.KeyItem)
	}
	slices.Sort(ids)
	if !slices.Equal(ids, []int{6378, 6379, 19719}) {
		t.Errorf("owned = %v, want [6378 6379 19719]", ids)
	}
}

// Empty active character: snapshot must be suppressed rather than committed
// under a blank name (which would be invisible in the UI and confusing).
func TestConsumer_SkipsSnapshotWhenNoActiveCharacter(t *testing.T) {
	c, s := openConsumerTest(t, "")
	base := time.Unix(1_700_000_000, 0)

	c.HandleLine(base, "Bone Crafted Key")
	c.HandleLine(base, "Hole Key")
	c.HandleLine(base.Add(time.Second), "some chat")

	got, err := s.ListByCharacter("")
	if err != nil {
		t.Fatalf("ListByCharacter: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected no entries committed under empty character, got %d", len(got))
	}
}

// A subsequent /keys must fully replace the prior snapshot — the user used
// a key in between and it should no longer appear as owned.
func TestConsumer_SecondBurstReplacesFirst(t *testing.T) {
	c, s := openConsumerTest(t, "Osui")
	base := time.Unix(1_700_000_000, 0)

	c.HandleLine(base, "Bone Crafted Key")
	c.HandleLine(base, "Hole Key")
	c.HandleLine(base.Add(time.Second), "non-match flush")

	// Second /keys an hour later — only Bone Crafted Key remains. Hole Key
	// was used up.
	later := base.Add(time.Hour)
	c.HandleLine(later, "Bone Crafted Key")
	c.HandleLine(later.Add(time.Second), "non-match flush")

	rows, _ := s.ListByCharacter("Osui")
	ids := make([]int, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.KeyItem)
	}
	if !slices.Equal(ids, []int{6378}) {
		t.Errorf("owned after second burst = %v, want [6378]", ids)
	}
	// Bone Crafted Key was in both bursts — first_seen_at must be from the
	// first burst, last_seen_at from the second.
	if rows[0].FirstSeenAt != base.Unix() {
		t.Errorf("first_seen_at = %d, want %d (preserved across snapshots)", rows[0].FirstSeenAt, base.Unix())
	}
	if rows[0].LastSeenAt != later.Unix() {
		t.Errorf("last_seen_at = %d, want %d (bumped by second burst)", rows[0].LastSeenAt, later.Unix())
	}
}

// Idle-timer flush: after FlushIdle elapses with no new line, the buffer
// commits without needing a non-matching line to arrive.
func TestConsumer_IdleTimerFlushesBurst(t *testing.T) {
	c, s := openConsumerTest(t, "Osui")
	base := time.Now()

	c.HandleLine(base, "Bone Crafted Key")
	c.HandleLine(base, "Hole Key")

	// Wait past FlushIdle and give the AfterFunc goroutine a moment to run.
	time.Sleep(FlushIdle + 200*time.Millisecond)

	got, _ := s.ListByCharacter("Osui")
	if len(got) != 2 {
		t.Errorf("expected idle timer to commit 2 entries, got %d", len(got))
	}
}
