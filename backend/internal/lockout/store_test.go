package lockout

import (
	"path/filepath"
	"testing"
	"time"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "user.db")
	s, err := OpenStore(path)
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestSnapshotReplaces(t *testing.T) {
	s := openTestStore(t)
	now := time.Unix(1_700_000_000, 0)

	first := []Entry{
		{Section: SectionLoot, TargetName: "King Tranix", ExpiresAt: 0},
		{Section: SectionLoot, TargetName: "Lord Nagafen", ExpiresAt: now.Add(time.Hour).Unix()},
		{Section: SectionLegacy, TargetName: "Shining Metallic Robes", ExpiresAt: 0},
	}
	if err := s.Snapshot("Osui", first, now); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	got, err := s.ListByCharacter("Osui")
	if err != nil {
		t.Fatalf("ListByCharacter: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d entries, want 3", len(got))
	}
	// Ordered by section then position. SectionLegacy < SectionLoot
	// alphabetically, so the legacy row sorts first.
	if got[0].Section != SectionLegacy || got[0].TargetName != "Shining Metallic Robes" {
		t.Errorf("first row = %+v, want the legacy entry", got[0])
	}
	for _, e := range got {
		if e.ObservedAt != now.Unix() {
			t.Errorf("observed_at = %d, want %d", e.ObservedAt, now.Unix())
		}
	}

	// A second snapshot fully replaces the first — fewer rows, no leftovers.
	second := []Entry{
		{Section: SectionLoot, TargetName: "Trakanon", ExpiresAt: 0},
	}
	later := now.Add(24 * time.Hour)
	if err := s.Snapshot("Osui", second, later); err != nil {
		t.Fatalf("Snapshot 2: %v", err)
	}
	got, err = s.ListByCharacter("Osui")
	if err != nil {
		t.Fatalf("ListByCharacter 2: %v", err)
	}
	if len(got) != 1 || got[0].TargetName != "Trakanon" {
		t.Fatalf("after replace, got %+v, want only Trakanon", got)
	}
	if got[0].ObservedAt != later.Unix() {
		t.Errorf("observed_at not refreshed: %d", got[0].ObservedAt)
	}
}

func TestSnapshotKeepsDuplicateNames(t *testing.T) {
	s := openTestStore(t)
	now := time.Unix(1_700_000_000, 0)
	// "Kaas Thox Xi Aten Ha Ra" legitimately appears twice in /sll with
	// different timers — position must keep them distinct.
	rows := []Entry{
		{Section: SectionLoot, TargetName: "Kaas Thox Xi Aten Ha Ra", ExpiresAt: now.Add(4 * 24 * time.Hour).Unix()},
		{Section: SectionLoot, TargetName: "Kaas Thox Xi Aten Ha Ra", ExpiresAt: now.Add(2 * 24 * time.Hour).Unix()},
	}
	if err := s.Snapshot("Osui", rows, now); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	got, err := s.ListByCharacter("Osui")
	if err != nil {
		t.Fatalf("ListByCharacter: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d rows, want 2 (duplicates preserved)", len(got))
	}
	if got[0].ExpiresAt == got[1].ExpiresAt {
		t.Errorf("duplicate rows collapsed: %+v", got)
	}
}

func TestUpsertEntryInsertsThenUpdates(t *testing.T) {
	s := openTestStore(t)
	now := time.Unix(1_700_000_000, 0)

	// First kill: no existing row, so it's inserted.
	exp1 := now.Add(6*24*time.Hour + 18*time.Hour)
	if err := s.UpsertEntry("Osui", SectionLoot, "Diabo Xi Xin Thall", exp1, now); err != nil {
		t.Fatalf("UpsertEntry (insert): %v", err)
	}
	got, err := s.ListByCharacter("Osui")
	if err != nil {
		t.Fatalf("ListByCharacter: %v", err)
	}
	if len(got) != 1 || got[0].TargetName != "Diabo Xi Xin Thall" || got[0].ExpiresAt != exp1.Unix() {
		t.Fatalf("after insert, got %+v", got)
	}

	// A second, unrelated kill appends rather than overwriting.
	if err := s.UpsertEntry("Osui", SectionLoot, "Trakanon", now.Add(24*time.Hour), now); err != nil {
		t.Fatalf("UpsertEntry (second target): %v", err)
	}
	got, err = s.ListByCharacter("Osui")
	if err != nil {
		t.Fatalf("ListByCharacter: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d rows, want 2", len(got))
	}

	// Re-killing Diabo updates the existing row in place rather than
	// duplicating it (simulates the duplicated in-game log line).
	later := now.Add(time.Hour)
	exp2 := later.Add(6 * 24 * time.Hour)
	if err := s.UpsertEntry("Osui", SectionLoot, "Diabo Xi Xin Thall", exp2, later); err != nil {
		t.Fatalf("UpsertEntry (update): %v", err)
	}
	got, err = s.ListByCharacter("Osui")
	if err != nil {
		t.Fatalf("ListByCharacter: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("got %d rows after re-kill, want 2 (updated in place)", len(got))
	}
	for _, e := range got {
		if e.TargetName == "Diabo Xi Xin Thall" {
			if e.ExpiresAt != exp2.Unix() {
				t.Errorf("Diabo ExpiresAt = %d, want %d", e.ExpiresAt, exp2.Unix())
			}
			if e.ObservedAt != later.Unix() {
				t.Errorf("Diabo ObservedAt = %d, want %d", e.ObservedAt, later.Unix())
			}
		}
	}
}

// TestUpsertTimeEntriesMergesAcrossSyncs verifies that syncing one phase's
// entries and then another's accumulates rather than replaces — the key
// difference from Snapshot's delete-and-reinsert model.
func TestUpsertTimeEntriesMergesAcrossSyncs(t *testing.T) {
	s := openTestStore(t)
	now := time.Unix(1_700_000_000, 0)

	phase1 := []Entry{
		{Section: SectionTime, TargetName: "Terlok of Earth", ExpiresAt: 0},
		{Section: SectionTime, TargetName: "Neimon of Air", ExpiresAt: now.Add(2 * 24 * time.Hour).Unix()},
	}
	if err := s.UpsertTimeEntries("Osui", phase1, now); err != nil {
		t.Fatalf("UpsertTimeEntries (phase1): %v", err)
	}

	later := now.Add(time.Hour)
	phase2 := []Entry{
		{Section: SectionTime, TargetName: "Windshapen Warlord of Air", ExpiresAt: 0},
	}
	if err := s.UpsertTimeEntries("Osui", phase2, later); err != nil {
		t.Fatalf("UpsertTimeEntries (phase2): %v", err)
	}

	got, err := s.ListByCharacter("Osui")
	if err != nil {
		t.Fatalf("ListByCharacter: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d entries, want 3 (both phases present)", len(got))
	}
	names := map[string]int64{}
	for _, e := range got {
		if e.Section != SectionTime {
			t.Errorf("entry %+v should be section time", e)
		}
		names[e.TargetName] = e.ExpiresAt
	}
	if exp, ok := names["Neimon of Air"]; !ok || exp != now.Add(2*24*time.Hour).Unix() {
		t.Errorf("Neimon of Air = %v, want the phase1 expiry preserved", exp)
	}
	if _, ok := names["Windshapen Warlord of Air"]; !ok {
		t.Error("Windshapen Warlord of Air missing after phase2 sync")
	}

	// Re-syncing phase 1 with Terlok now defeated should update in place, not
	// duplicate.
	phase1Again := []Entry{
		{Section: SectionTime, TargetName: "Terlok of Earth", ExpiresAt: later.Add(3 * time.Hour).Unix()},
	}
	if err := s.UpsertTimeEntries("Osui", phase1Again, later); err != nil {
		t.Fatalf("UpsertTimeEntries (phase1 re-sync): %v", err)
	}
	got, err = s.ListByCharacter("Osui")
	if err != nil {
		t.Fatalf("ListByCharacter after re-sync: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d entries after re-sync, want 3 (updated in place, not duplicated)", len(got))
	}
}

// TestSnapshotNeverTouchesTimeSection verifies Snapshot (the `/sll` path)
// excludes SectionTime from its delete, so a loot resync can't erase Time
// rows UpsertTimeEntries wrote.
func TestSnapshotNeverTouchesTimeSection(t *testing.T) {
	s := openTestStore(t)
	now := time.Unix(1_700_000_000, 0)

	if err := s.UpsertTimeEntries("Osui", []Entry{
		{Section: SectionTime, TargetName: "Terlok of Earth", ExpiresAt: 0},
	}, now); err != nil {
		t.Fatalf("UpsertTimeEntries: %v", err)
	}
	if err := s.Snapshot("Osui", []Entry{
		{Section: SectionLoot, TargetName: "King Tranix", ExpiresAt: 0},
	}, now); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}

	got, err := s.ListByCharacter("Osui")
	if err != nil {
		t.Fatalf("ListByCharacter: %v", err)
	}
	var sawTime, sawLoot bool
	for _, e := range got {
		if e.Section == SectionTime && e.TargetName == "Terlok of Earth" {
			sawTime = true
		}
		if e.Section == SectionLoot && e.TargetName == "King Tranix" {
			sawLoot = true
		}
	}
	if !sawTime {
		t.Error("Snapshot (an /sll resync) should not remove Time rows")
	}
	if !sawLoot {
		t.Error("Snapshot should still write its own loot row")
	}
}

func TestCharactersAndDelete(t *testing.T) {
	s := openTestStore(t)
	now := time.Unix(1_700_000_000, 0)
	mustSnap := func(char string) {
		if err := s.Snapshot(char, []Entry{{Section: SectionLoot, TargetName: "X"}}, now); err != nil {
			t.Fatalf("Snapshot %s: %v", char, err)
		}
	}
	mustSnap("Osui")
	mustSnap("Nariana")

	chars, err := s.Characters()
	if err != nil {
		t.Fatalf("Characters: %v", err)
	}
	if len(chars) != 2 || chars[0] != "Nariana" || chars[1] != "Osui" {
		t.Fatalf("Characters = %v, want [Nariana Osui]", chars)
	}

	if err := s.Delete("Osui"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	chars, _ = s.Characters()
	if len(chars) != 1 || chars[0] != "Nariana" {
		t.Fatalf("after delete, Characters = %v, want [Nariana]", chars)
	}
}
