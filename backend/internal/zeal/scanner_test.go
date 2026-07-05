package zeal

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScanAllInventories_BagSlotDetection(t *testing.T) {
	dir := t.TempDir()

	// Inventory with lucid shards in a regular backpack and inside a combine container
	// (Shadowed Scepter Frame). Both should be detected.
	content := "Location\tName\tID\tCount\tSlots\n" +
		"Head\tSome Helmet\t1001\t1\t0\n" +
		"General1\tBackpack\t17005\t1\t8\n" +
		"General1-Slot1\tA Lucid Shard\t22185\t1\t0\n" +
		"General1-Slot2\tA Lucid Shard\t22186\t1\t0\n" +
		"General1-Slot3\tEmpty\t0\t0\t0\n" +
		"General2\tShadowed Scepter Frame\t17323\t1\t10\n" +
		"General2-Slot1\tA Lucid Shard\t22187\t1\t0\n" +
		"General2-Slot2\tA Lucid Shard\t22188\t1\t0\n" +
		"SharedBank1\tSmall Box\t17006\t1\t8\n" +
		"SharedBank1-Slot1\tSome Item\t9999\t1\t0\n"

	path := filepath.Join(dir, "TestChar-Inventory.txt")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	chars, sharedBank, err := ScanAllInventories(dir)
	if err != nil {
		t.Fatalf("ScanAllInventories: %v", err)
	}

	if len(chars) != 1 {
		t.Fatalf("expected 1 character, got %d", len(chars))
	}

	// SharedBank entries must be split off.
	if len(sharedBank) != 2 {
		t.Errorf("expected 2 SharedBank entries, got %d", len(sharedBank))
	}

	// Build ID lookup from character entries.
	haveIDs := make(map[int]bool)
	for _, e := range chars[0].Entries {
		haveIDs[e.ID] = true
	}

	// Shards in a regular backpack must be detected.
	for _, id := range []int{22185, 22186} {
		if !haveIDs[id] {
			t.Errorf("lucid shard ID %d in regular backpack: not detected", id)
		}
	}

	// Shards inside the Shadowed Scepter Frame (combine container) must also be detected.
	for _, id := range []int{22187, 22188} {
		if !haveIDs[id] {
			t.Errorf("lucid shard ID %d inside Scepter Frame: not detected", id)
		}
	}

	// The Frame itself must be detected.
	if !haveIDs[17323] {
		t.Error("Shadowed Scepter Frame (17323): not detected")
	}

	// Empty slots (ID=0) should not leak into the shared-bank set.
	if sharedBank[0].ID == 0 || sharedBank[1].ID == 0 {
		t.Error("empty-slot entry (ID=0) leaked into SharedBank results")
	}

	// No SharedBank entries must appear in character entries.
	for _, e := range chars[0].Entries {
		if e.Location != "" && len(e.Location) >= 10 && e.Location[:10] == "SharedBank" {
			t.Errorf("SharedBank entry leaked into character entries: %+v", e)
		}
	}
}

func TestScanAllInventories_DropsOutOfRangeSharedBank(t *testing.T) {
	dir := t.TempDir()

	// SharedBank1–10 are real on Project Quarm; 11–30 are Zeal-only artefacts
	// of the modern-EQ inventory layout and must be dropped.
	content := "Location\tName\tID\tCount\tSlots\n" +
		"SharedBank1\tValid Item\t1001\t1\t0\n" +
		"SharedBank10\tEdge Of Range\t1010\t1\t0\n" +
		"SharedBank10-Slot3\tInsideBag\t1011\t1\t0\n" +
		"SharedBank11\tOut Of Range\t1099\t1\t0\n" +
		"SharedBank15-Slot2\tOut Of Range Slot\t1098\t1\t0\n" +
		"SharedBank30\tOut Of Range Far\t1097\t1\t0\n"

	path := filepath.Join(dir, "TestChar-Inventory.txt")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, sharedBank, err := ScanAllInventories(dir)
	if err != nil {
		t.Fatalf("ScanAllInventories: %v", err)
	}

	if len(sharedBank) != 3 {
		t.Fatalf("expected 3 in-range SharedBank entries, got %d: %+v", len(sharedBank), sharedBank)
	}

	for _, e := range sharedBank {
		if e.ID == 1099 || e.ID == 1098 || e.ID == 1097 {
			t.Errorf("out-of-range SharedBank entry leaked: %+v", e)
		}
	}
}

func TestScanAllInventories_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	chars, sharedBank, err := ScanAllInventories(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(chars) != 0 {
		t.Errorf("expected 0 chars, got %d", len(chars))
	}
	if len(sharedBank) != 0 {
		t.Errorf("expected 0 SharedBank entries, got %d", len(sharedBank))
	}
}

func TestScanAllInventories_MultipleCharacters(t *testing.T) {
	dir := t.TempDir()

	// Char A has the Scepter of Shadows (fully keyed).
	contentA := "Location\tName\tID\tCount\tSlots\n" +
		"Bank1-Slot1\tThe Scepter of Shadows\t22198\t1\t0\n"
	os.WriteFile(filepath.Join(dir, "CharA-Inventory.txt"), []byte(contentA), 0o644)

	// Char B has individual lucid shards.
	contentB := "Location\tName\tID\tCount\tSlots\n" +
		"General1\tBackpack\t17005\t1\t8\n" +
		"General1-Slot1\tA Lucid Shard\t22185\t1\t0\n" +
		"General1-Slot2\tA Lucid Shard\t22190\t1\t0\n"
	os.WriteFile(filepath.Join(dir, "CharB-Inventory.txt"), []byte(contentB), 0o644)

	chars, _, err := ScanAllInventories(dir)
	if err != nil {
		t.Fatalf("ScanAllInventories: %v", err)
	}

	if len(chars) != 2 {
		t.Fatalf("expected 2 characters, got %d", len(chars))
	}

	byChar := make(map[string]map[int]bool)
	for _, inv := range chars {
		ids := make(map[int]bool)
		for _, e := range inv.Entries {
			ids[e.ID] = true
		}
		byChar[inv.Character] = ids
	}

	if !byChar["CharA"][22198] {
		t.Error("CharA: Scepter of Shadows (22198) not detected")
	}
	if !byChar["CharB"][22185] {
		t.Error("CharB: Lucid Shard (22185) not detected")
	}
	if !byChar["CharB"][22190] {
		t.Error("CharB: Lucid Shard (22190) not detected")
	}
}

// ScanAllMacros must return one entry per real character and must NOT treat the
// client's other _pq.proj.ini config files (UI_<Char> window layouts, BZR_<Char>
// bazaar state) as characters — their prefixes contain an underscore, which real
// EQ names never do.
func TestScanAllMacros_ExcludesUIandBZR(t *testing.T) {
	dir := t.TempDir()
	socials := "[Socials]\nPage1Button1Name=Hi\nPage1Button1Color=0\nPage1Button1Line1=/say hi\n"
	files := map[string]string{
		"Osui_pq.proj.ini":      socials,
		"Feane_pq.proj.ini":     socials,
		"BZR_Feane_pq.proj.ini": "[ItemToSell]\nSlot0=nothing\n",
		"UI_Osui_pq.proj.ini":   "[Main]\nX=100\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}

	macros, err := ScanAllMacros(dir)
	if err != nil {
		t.Fatalf("ScanAllMacros: %v", err)
	}

	got := make(map[string]bool)
	for _, mf := range macros {
		got[mf.Character] = true
	}
	if len(got) != 2 || !got["Osui"] || !got["Feane"] {
		t.Fatalf("characters = %v, want exactly Osui and Feane", got)
	}
	if got["BZR_Feane"] || got["UI_Osui"] {
		t.Errorf("client config files leaked as characters: %v", got)
	}
}
