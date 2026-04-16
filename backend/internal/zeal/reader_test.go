package zeal

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return path
}

func TestParseInventory(t *testing.T) {
	content := "Location\tName\tID\tCount\tSlots\n" +
		"Head\tIron Cap\t1001\t1\t0\n" +
		"Primary\tLong Sword\t1002\t1\t0\n" +
		"General1\tSmall Box\t1003\t1\t4\n" +
		"General1:Slot1\tApple\t1004\t10\t0\n" +
		"General1:Slot2\tBandage\t1005\t20\t0\n"

	path := writeTemp(t, "Tester_pq.proj-Inventory.txt", content)
	inv, err := ParseInventory(path, "Tester")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if inv.Character != "Tester" {
		t.Errorf("character = %q, want Tester", inv.Character)
	}
	if len(inv.Entries) != 5 {
		t.Errorf("entries count = %d, want 5", len(inv.Entries))
	}

	// Verify specific entries.
	tests := []struct {
		idx      int
		location string
		name     string
		id       int
		count    int
		slots    int
	}{
		{0, "Head", "Iron Cap", 1001, 1, 0},
		{1, "Primary", "Long Sword", 1002, 1, 0},
		{2, "General1", "Small Box", 1003, 1, 4},
		{3, "General1:Slot1", "Apple", 1004, 10, 0},
		{4, "General1:Slot2", "Bandage", 1005, 20, 0},
	}
	for _, tc := range tests {
		e := inv.Entries[tc.idx]
		if e.Location != tc.location || e.Name != tc.name || e.ID != tc.id ||
			e.Count != tc.count || e.Slots != tc.slots {
			t.Errorf("entry[%d] = %+v, want location=%s name=%s id=%d count=%d slots=%d",
				tc.idx, e, tc.location, tc.name, tc.id, tc.count, tc.slots)
		}
	}
}

func TestParseInventory_NoHeader(t *testing.T) {
	// File without a header row should still parse correctly.
	content := "Head\tIron Cap\t1001\t1\t0\n"
	path := writeTemp(t, "NoHeader_pq.proj-Inventory.txt", content)
	inv, err := ParseInventory(path, "NoHeader")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(inv.Entries) != 1 {
		t.Errorf("entries count = %d, want 1", len(inv.Entries))
	}
}

func TestParseInventory_EmptyFile(t *testing.T) {
	path := writeTemp(t, "Empty_pq.proj-Inventory.txt", "")
	inv, err := ParseInventory(path, "Empty")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(inv.Entries) != 0 {
		t.Errorf("expected empty entries, got %d", len(inv.Entries))
	}
}

func TestParseInventory_MissingFile(t *testing.T) {
	_, err := ParseInventory("/nonexistent/path/file.txt", "X")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestParseSpellbook_SpellIDOnly(t *testing.T) {
	content := "1200\n2100\n3050\n"
	path := writeTemp(t, "Wizard_pq.proj-Spells.txt", content)
	sb, err := ParseSpellbook(path, "Wizard")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []int{1200, 2100, 3050}
	if len(sb.SpellIDs) != len(want) {
		t.Fatalf("spell count = %d, want %d", len(sb.SpellIDs), len(want))
	}
	for i, id := range want {
		if sb.SpellIDs[i] != id {
			t.Errorf("spell[%d] = %d, want %d", i, sb.SpellIDs[i], id)
		}
	}
}

func TestParseSpellbook_SlotTabID(t *testing.T) {
	// Format: slot\tspell_id
	content := "1\t1200\n2\t2100\n3\t3050\n"
	path := writeTemp(t, "Enc_pq.proj-Spells.txt", content)
	sb, err := ParseSpellbook(path, "Enc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []int{1200, 2100, 3050}
	if len(sb.SpellIDs) != len(want) {
		t.Fatalf("spell count = %d, want %d", len(sb.SpellIDs), len(want))
	}
	for i, id := range want {
		if sb.SpellIDs[i] != id {
			t.Errorf("spell[%d] = %d, want %d", i, sb.SpellIDs[i], id)
		}
	}
}

func TestParseSpellbook_Deduplication(t *testing.T) {
	content := "1200\n1200\n2100\n"
	path := writeTemp(t, "Dup_pq.proj-Spells.txt", content)
	sb, err := ParseSpellbook(path, "Dup")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sb.SpellIDs) != 2 {
		t.Errorf("expected 2 unique spell IDs, got %d", len(sb.SpellIDs))
	}
}

func TestInventoryPath(t *testing.T) {
	got := InventoryPath("/eq", "Aradune")
	want := filepath.Join("/eq", "Aradune-Inventory.txt")
	if got != want {
		t.Errorf("InventoryPath = %q, want %q", got, want)
	}
}

func TestSpellbookPath(t *testing.T) {
	got := SpellbookPath("/eq", "Aradune")
	want := filepath.Join("/eq", "Aradune-Spellbook.txt")
	if got != want {
		t.Errorf("SpellbookPath = %q, want %q", got, want)
	}
}

func TestModTime_Missing(t *testing.T) {
	mt := ModTime("/nonexistent/file.txt")
	if !mt.Equal(time.Time{}) {
		t.Error("expected zero time for missing file")
	}
}

func TestModTime_Present(t *testing.T) {
	path := writeTemp(t, "test.txt", "hello")
	mt := ModTime(path)
	if mt.IsZero() {
		t.Error("expected non-zero mod time for existing file")
	}
}
