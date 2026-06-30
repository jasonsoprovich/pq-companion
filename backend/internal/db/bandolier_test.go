package db_test

import (
	"strings"
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/db"
)

func TestBandolierSlotItems_OwnershipAndSlotGate(t *testing.T) {
	d := openTestDB(t)

	const primaryBit = 0x002000
	const headBit = 0x000004

	// Real primary-slot items to act as "owned" weapons.
	primary, err := d.UpgradeCandidates(db.CandidateFilter{SlotMask: primaryBit, MaxLevel: 60})
	if err != nil {
		t.Fatalf("UpgradeCandidates(primary): %v", err)
	}
	if len(primary) < 5 {
		t.Skipf("not enough primary items in DB: %d", len(primary))
	}
	// A head item the character also "owns" but which must NOT show up for the
	// Primary slot (slot gate).
	head, err := d.UpgradeCandidates(db.CandidateFilter{SlotMask: headBit, MaxLevel: 60})
	if err != nil {
		t.Fatalf("UpgradeCandidates(head): %v", err)
	}
	if len(head) == 0 {
		t.Skip("no head items in DB")
	}

	owned := []int{head[0].ID}
	wantPrimary := map[int]bool{}
	for i := 0; i < 5; i++ {
		owned = append(owned, primary[i].ID)
		// head[0] could in theory also be flagged primary; only count pure ones.
		if primary[i].Slots&primaryBit != 0 {
			wantPrimary[primary[i].ID] = true
		}
	}

	got, err := d.BandolierSlotItems(0 /*Primary*/, owned, "")
	if err != nil {
		t.Fatalf("BandolierSlotItems: %v", err)
	}

	gotIDs := map[int]bool{}
	for _, it := range got {
		gotIDs[it.ID] = true
		// Slot gate: everything returned must fit the primary slot.
		if it.Slots&primaryBit == 0 {
			t.Errorf("item %d (%q) returned for Primary but slots=%#x lacks the bit", it.ID, it.Name, it.Slots)
		}
		// Ownership gate: nothing outside the owned list may appear.
		ownedSet := map[int]bool{}
		for _, id := range owned {
			ownedSet[id] = true
		}
		if !ownedSet[it.ID] {
			t.Errorf("item %d returned but is not in the owned set", it.ID)
		}
	}

	// The head-only item must be excluded by the slot gate.
	if head[0].Slots&primaryBit == 0 && gotIDs[head[0].ID] {
		t.Errorf("head item %d leaked into Primary results", head[0].ID)
	}
	// Every owned primary weapon should be present.
	for id := range wantPrimary {
		if !gotIDs[id] {
			t.Errorf("owned primary item %d missing from results", id)
		}
	}
}

func TestBandolierSlotItems_Search(t *testing.T) {
	d := openTestDB(t)

	const primaryBit = 0x002000
	primary, err := d.UpgradeCandidates(db.CandidateFilter{SlotMask: primaryBit, MaxLevel: 60})
	if err != nil {
		t.Fatalf("UpgradeCandidates(primary): %v", err)
	}
	if len(primary) == 0 {
		t.Skip("no primary items in DB")
	}
	owned := make([]int, 0, len(primary))
	for _, c := range primary {
		owned = append(owned, c.ID)
	}

	// Use the first word of a real item's name as the search term.
	term := strings.Fields(primary[0].Name)
	if len(term) == 0 {
		t.Skip("primary item has no name")
	}
	got, err := d.BandolierSlotItems(0, owned, term[0])
	if err != nil {
		t.Fatalf("BandolierSlotItems(search): %v", err)
	}
	if len(got) == 0 {
		t.Fatalf("search %q returned nothing", term[0])
	}
	for _, it := range got {
		if !strings.Contains(strings.ToLower(it.Name), strings.ToLower(term[0])) {
			t.Errorf("item %q does not match search %q", it.Name, term[0])
		}
	}
}

func TestBandolierSlotItems_EmptyAndOutOfRange(t *testing.T) {
	d := openTestDB(t)

	got, err := d.BandolierSlotItems(0, nil, "")
	if err != nil {
		t.Fatalf("empty owned: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("empty owned returned %d items, want 0", len(got))
	}

	got, err = d.BandolierSlotItems(99, []int{1001}, "")
	if err != nil {
		t.Fatalf("out-of-range slot: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("out-of-range slot returned %d items, want 0", len(got))
	}
}
