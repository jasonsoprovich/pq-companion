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

	got, err := d.BandolierSlotItems(0 /*Primary*/, owned, "", db.BandolierSlotFilter{})
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
	got, err := d.BandolierSlotItems(0, owned, term[0], db.BandolierSlotFilter{})
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

func TestBandolierSlotItems_ClassAndLevelGuardrails(t *testing.T) {
	d := openTestDB(t)

	const primaryBit = 0x002000
	primary, err := d.UpgradeCandidates(db.CandidateFilter{SlotMask: primaryBit, MaxLevel: 60})
	if err != nil {
		t.Fatalf("UpgradeCandidates(primary): %v", err)
	}
	if len(primary) < 5 {
		t.Skipf("not enough primary items in DB: %d", len(primary))
	}
	owned := make([]int, 0, len(primary))
	byID := map[int]db.UpgradeCandidate{}
	for _, c := range primary {
		owned = append(owned, c.ID)
		byID[c.ID] = c
	}

	// Zero-value filter must match the ungated result exactly (graceful fallback).
	base, err := d.BandolierSlotItems(0, owned, "", db.BandolierSlotFilter{})
	if err != nil {
		t.Fatalf("BandolierSlotItems(base): %v", err)
	}

	// Class gate: pick a class bit that only some owned primaries allow, so the
	// filter must both keep the allowed ones and drop the rest.
	var classBit int
	for cls := 0; cls <= 14; cls++ {
		bit := 1 << cls
		allowed, blocked := 0, 0
		for _, c := range base {
			if byID[c.ID].Classes&bit != 0 {
				allowed++
			} else {
				blocked++
			}
		}
		if allowed > 0 && blocked > 0 {
			classBit = bit
			break
		}
	}
	if classBit == 0 {
		t.Skip("no class discriminates the owned primary set")
	}

	got, err := d.BandolierSlotItems(0, owned, "", db.BandolierSlotFilter{ClassBit: classBit})
	if err != nil {
		t.Fatalf("BandolierSlotItems(class): %v", err)
	}
	for _, it := range got {
		if byID[it.ID].Classes&classBit == 0 {
			t.Errorf("item %d (%q) returned but class bit %#x not in classes %#x",
				it.ID, it.Name, classBit, byID[it.ID].Classes)
		}
	}
	for _, c := range base {
		want := byID[c.ID].Classes&classBit != 0
		found := false
		for _, it := range got {
			if it.ID == c.ID {
				found = true
				break
			}
		}
		if want != found {
			t.Errorf("class filter mismatch for item %d (%q): want present=%v got=%v", c.ID, c.Name, want, found)
		}
	}

	// Level gate: no item with reqlevel above the character level may survive.
	const lvl = 5
	lvlGot, err := d.BandolierSlotItems(0, owned, "", db.BandolierSlotFilter{Level: lvl})
	if err != nil {
		t.Fatalf("BandolierSlotItems(level): %v", err)
	}
	for _, it := range lvlGot {
		if byID[it.ID].ReqLevel > lvl {
			t.Errorf("item %d (%q) reqlevel %d survived level-%d filter", it.ID, it.Name, byID[it.ID].ReqLevel, lvl)
		}
	}
}

func TestBandolierSlotItems_EmptyAndOutOfRange(t *testing.T) {
	d := openTestDB(t)

	got, err := d.BandolierSlotItems(0, nil, "", db.BandolierSlotFilter{})
	if err != nil {
		t.Fatalf("empty owned: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("empty owned returned %d items, want 0", len(got))
	}

	got, err = d.BandolierSlotItems(99, []int{1001}, "", db.BandolierSlotFilter{})
	if err != nil {
		t.Fatalf("out-of-range slot: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("out-of-range slot returned %d items, want 0", len(got))
	}
}
