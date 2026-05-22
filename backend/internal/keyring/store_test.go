package keyring

import (
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	s, err := OpenStore(filepath.Join(dir, "user.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func ownedItems(t *testing.T, s *Store, char string) []int {
	t.Helper()
	rows, err := s.ListByCharacter(char)
	if err != nil {
		t.Fatalf("ListByCharacter %q: %v", char, err)
	}
	ids := make([]int, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.KeyItem)
	}
	slices.Sort(ids)
	return ids
}

// Snapshot inserts new rows and preserves first_seen_at across subsequent
// snapshots that re-observe the same item.
func TestSnapshot_PreservesFirstSeenAcrossReObservation(t *testing.T) {
	s := openTest(t)

	t1 := time.Unix(1_700_000_000, 0)
	if err := s.Snapshot("Osui", []int{6378, 20600}, t1); err != nil {
		t.Fatalf("snapshot 1: %v", err)
	}

	t2 := t1.Add(2 * time.Hour)
	if err := s.Snapshot("Osui", []int{6378, 20600, 19719}, t2); err != nil {
		t.Fatalf("snapshot 2: %v", err)
	}

	rows, err := s.ListByCharacter("Osui")
	if err != nil {
		t.Fatalf("ListByCharacter: %v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(rows))
	}
	for _, r := range rows {
		switch r.KeyItem {
		case 6378, 20600:
			if r.FirstSeenAt != t1.Unix() {
				t.Errorf("key %d first_seen_at=%d, want %d (preserved from snapshot 1)", r.KeyItem, r.FirstSeenAt, t1.Unix())
			}
			if r.LastSeenAt != t2.Unix() {
				t.Errorf("key %d last_seen_at=%d, want %d (bumped by snapshot 2)", r.KeyItem, r.LastSeenAt, t2.Unix())
			}
		case 19719:
			if r.FirstSeenAt != t2.Unix() || r.LastSeenAt != t2.Unix() {
				t.Errorf("key 19719 should have first=last=%d, got first=%d last=%d", t2.Unix(), r.FirstSeenAt, r.LastSeenAt)
			}
		default:
			t.Errorf("unexpected key %d", r.KeyItem)
		}
	}
}

// Snapshot drops rows whose key_item isn't in the new set — the character
// must have used those keys, so they shouldn't keep showing as owned.
func TestSnapshot_DropsStaleEntries(t *testing.T) {
	s := openTest(t)

	t1 := time.Unix(1_700_000_000, 0)
	if err := s.Snapshot("Osui", []int{6378, 20600, 19719}, t1); err != nil {
		t.Fatalf("snapshot 1: %v", err)
	}
	t2 := t1.Add(time.Hour)
	// Character now only has 6378 on keyring.
	if err := s.Snapshot("Osui", []int{6378}, t2); err != nil {
		t.Fatalf("snapshot 2: %v", err)
	}

	if got := ownedItems(t, s, "Osui"); !slices.Equal(got, []int{6378}) {
		t.Errorf("owned = %v, want [6378]", got)
	}
}

// Per-character isolation: snapshotting Osui doesn't touch Nariana's rows.
func TestSnapshot_IsolatedAcrossCharacters(t *testing.T) {
	s := openTest(t)

	ts := time.Unix(1_700_000_000, 0)
	if err := s.Snapshot("Osui", []int{6378, 20600}, ts); err != nil {
		t.Fatalf("snapshot Osui: %v", err)
	}
	if err := s.Snapshot("Nariana", []int{19719}, ts); err != nil {
		t.Fatalf("snapshot Nariana: %v", err)
	}
	// Re-snapshot Osui with only one key. Nariana must be untouched.
	if err := s.Snapshot("Osui", []int{20600}, ts.Add(time.Minute)); err != nil {
		t.Fatalf("snapshot Osui 2: %v", err)
	}

	if got := ownedItems(t, s, "Osui"); !slices.Equal(got, []int{20600}) {
		t.Errorf("Osui owned = %v, want [20600]", got)
	}
	if got := ownedItems(t, s, "Nariana"); !slices.Equal(got, []int{19719}) {
		t.Errorf("Nariana owned = %v, want [19719]", got)
	}
}

func TestCharacters_ReturnsDistinctSortedNames(t *testing.T) {
	s := openTest(t)
	ts := time.Unix(1_700_000_000, 0)
	if err := s.Snapshot("Osui", []int{6378}, ts); err != nil {
		t.Fatalf("snapshot Osui: %v", err)
	}
	if err := s.Snapshot("nariana", []int{20600}, ts); err != nil {
		t.Fatalf("snapshot nariana: %v", err)
	}
	if err := s.Snapshot("Argyle", []int{19719}, ts); err != nil {
		t.Fatalf("snapshot Argyle: %v", err)
	}
	got, err := s.Characters()
	if err != nil {
		t.Fatalf("Characters: %v", err)
	}
	want := []string{"Argyle", "nariana", "Osui"}
	if !slices.Equal(got, want) {
		t.Errorf("Characters() = %v, want %v", got, want)
	}
}
