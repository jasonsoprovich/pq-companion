package character

import "testing"

func TestHiddenCharacters_SetListRoundTrip(t *testing.T) {
	s, _ := openTestStore(t)

	if got, err := s.HiddenNames(); err != nil || len(got) != 0 {
		t.Fatalf("empty hidden set: got %v err %v", got, err)
	}

	if err := s.SetHidden("Mule", true); err != nil {
		t.Fatalf("hide Mule: %v", err)
	}
	// Hiding is idempotent — a second call must not error or duplicate.
	if err := s.SetHidden("Mule", true); err != nil {
		t.Fatalf("re-hide Mule: %v", err)
	}

	hidden, err := s.HiddenNames()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	// Membership is case-insensitive and the set is keyed lower-case.
	if _, ok := hidden["mule"]; !ok {
		t.Fatalf("HiddenNames = %v, want it to contain mule", hidden)
	}
	if len(hidden) != 1 {
		t.Fatalf("HiddenNames = %v, want exactly one entry", hidden)
	}

	// Unhiding with a different case still removes the row (COLLATE NOCASE).
	if err := s.SetHidden("MULE", false); err != nil {
		t.Fatalf("unhide MULE: %v", err)
	}
	if got, _ := s.HiddenNames(); len(got) != 0 {
		t.Fatalf("after unhide, HiddenNames = %v, want empty", got)
	}

	// Unhiding something that was never hidden is a no-op, not an error.
	if err := s.SetHidden("NeverHidden", false); err != nil {
		t.Fatalf("unhide unknown: %v", err)
	}

	// An empty name is rejected so it can't shadow the "no character" state.
	if err := s.SetHidden("", true); err == nil {
		t.Fatal("SetHidden(\"\", true) = nil, want error")
	}
	if err := s.SetHidden("   ", true); err == nil {
		t.Fatal("SetHidden(whitespace, true) = nil, want error")
	}
}

func TestHiddenCharacters_NameNeedNotBeAStoredCharacter(t *testing.T) {
	s, _ := openTestStore(t)

	// A name that only exists as an on-disk _pq.proj.ini (never Create()d) can
	// still be hidden — the hidden_characters table is keyed by name, not by a
	// characters.id foreign key.
	if err := s.SetHidden("DiskOnlyAlt", true); err != nil {
		t.Fatalf("hide disk-only alt: %v", err)
	}
	hidden, err := s.HiddenNames()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if _, ok := hidden["diskonlyalt"]; !ok {
		t.Fatalf("HiddenNames = %v, want it to contain diskonlyalt", hidden)
	}
}
