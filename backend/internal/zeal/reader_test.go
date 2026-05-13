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

func TestParseSpellsets(t *testing.T) {
	content := "[58group]\n" +
		"0=-1\n1=-1\n2=-1\n3=-1\n" +
		"4=2609\n5=712\n6=1760\n7=2610\n" +
		"[15base]\n" +
		"0=1100\n1=750\n2=728\n3=2605\n" +
		"4=1196\n5=-1\n6=-1\n7=-1\n"

	path := writeTemp(t, "Tester_spellsets.ini", content)
	sf, err := ParseSpellsets(path, "Tester")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sf.Character != "Tester" {
		t.Errorf("character = %q, want Tester", sf.Character)
	}
	if len(sf.Spellsets) != 2 {
		t.Fatalf("spellsets count = %d, want 2", len(sf.Spellsets))
	}

	first := sf.Spellsets[0]
	if first.Name != "58group" {
		t.Errorf("first name = %q, want 58group", first.Name)
	}
	wantFirst := []int{-1, -1, -1, -1, 2609, 712, 1760, 2610}
	if len(first.SpellIDs) != 8 {
		t.Fatalf("first slots = %d, want 8", len(first.SpellIDs))
	}
	for i, id := range wantFirst {
		if first.SpellIDs[i] != id {
			t.Errorf("first slot %d = %d, want %d", i, first.SpellIDs[i], id)
		}
	}

	second := sf.Spellsets[1]
	if second.Name != "15base" {
		t.Errorf("second name = %q, want 15base", second.Name)
	}
	if second.SpellIDs[0] != 1100 || second.SpellIDs[4] != 1196 || second.SpellIDs[7] != -1 {
		t.Errorf("second slots unexpected: %v", second.SpellIDs)
	}
}

func TestParseSpellsetsFixture(t *testing.T) {
	// testdata path is relative to the package directory.
	path := filepath.Join("..", "..", "..", "testdata", "Grokenspiel_spellsets.ini")
	if _, err := os.Stat(path); err != nil {
		t.Skipf("fixture not found: %v", err)
	}
	sf, err := ParseSpellsets(path, "Grokenspiel")
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	// Fixture has 13 sections (see testdata/Grokenspiel_spellsets.ini).
	if len(sf.Spellsets) != 13 {
		t.Fatalf("spellsets = %d, want 13", len(sf.Spellsets))
	}
	if sf.Spellsets[0].Name != "58group" {
		t.Errorf("first name = %q, want 58group", sf.Spellsets[0].Name)
	}
	// zzEmp section has all 8 slots populated.
	var zzEmp *Spellset
	for i := range sf.Spellsets {
		if sf.Spellsets[i].Name == "zzEmp" {
			zzEmp = &sf.Spellsets[i]
			break
		}
	}
	if zzEmp == nil {
		t.Fatal("zzEmp section missing")
	}
	for i, id := range zzEmp.SpellIDs {
		if id <= 0 {
			t.Errorf("zzEmp slot %d = %d, expected populated", i, id)
		}
	}
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

func TestParseQuarmy_StatsAndAAs(t *testing.T) {
	content := "Character\tName\tLastName\tLevel\tClass\tRace\tGender\tDeity\tGuild\tGuildRank\tBaseSTR\tBaseSTA\tBaseCHA\tBaseDEX\tBaseINT\tBaseAGI\tBaseWIS\n" +
		"Character\tOsui\t\t60\t14\t6\t1\t396\tSeekers of Souls\t0\t60\t65\t95\t75\t114\t90\t83\n" +
		"Location\tName\tID\tCount\tSlots\n" +
		"Head\tCirclet of the Falinkan\t1867\t1\t0\n" +
		"Primary\tWand of Tranquility\t26768\t1\t0\n" +
		"AAIndex\tRank\n" +
		"5\t3\n" +
		"13\t3\n" +
		"211\t3\n" +
		"Checksum\t12345\n"

	path := writeTemp(t, "Osui-Quarmy.txt", content)
	data, err := ParseQuarmy(path, "Osui")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if data.Character != "Osui" {
		t.Errorf("character = %q, want Osui", data.Character)
	}

	// Identity (raw EQ 1-indexed values from the file)
	if data.Level != 60 {
		t.Errorf("level = %d, want 60", data.Level)
	}
	if data.Class != 14 {
		t.Errorf("class = %d, want 14 (Enchanter)", data.Class)
	}
	if data.Race != 6 {
		t.Errorf("race = %d, want 6 (Dark Elf)", data.Race)
	}

	// Stats
	got := data.Stats
	want := CharStats{BaseSTR: 60, BaseSTA: 65, BaseCHA: 95, BaseDEX: 75, BaseINT: 114, BaseAGI: 90, BaseWIS: 83}
	if got != want {
		t.Errorf("stats = %+v, want %+v", got, want)
	}

	// Inventory
	if len(data.Inventory) != 2 {
		t.Errorf("inventory count = %d, want 2", len(data.Inventory))
	}
	if data.Inventory[0].Location != "Head" || data.Inventory[0].ID != 1867 {
		t.Errorf("inventory[0] = %+v", data.Inventory[0])
	}

	// AAs
	if len(data.AAs) != 3 {
		t.Errorf("aa count = %d, want 3", len(data.AAs))
	}
	wantAAs := []AAEntry{{ID: 5, Rank: 3}, {ID: 13, Rank: 3}, {ID: 211, Rank: 3}}
	for i, aa := range wantAAs {
		if data.AAs[i] != aa {
			t.Errorf("aa[%d] = %+v, want %+v", i, data.AAs[i], aa)
		}
	}
}

func TestParseQuarmy_MissingFile(t *testing.T) {
	_, err := ParseQuarmy("/nonexistent/path/Foo-Quarmy.txt", "Foo")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestParseQuarmy_RealOsui(t *testing.T) {
	// Use the real testdata file to verify end-to-end parsing.
	// testdata/ is gitignored, so skip when the fixture isn't present (e.g. in CI).
	path := filepath.Join("..", "..", "..", "testdata", "Osui-Quarmy.txt")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Skipf("testdata fixture %s not present", path)
	}
	data, err := ParseQuarmy(path, "Osui")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if data.Stats.BaseINT != 114 {
		t.Errorf("Osui BaseINT = %d, want 114", data.Stats.BaseINT)
	}
	if len(data.AAs) == 0 {
		t.Error("expected at least one AA for Osui")
	}
	if len(data.Inventory) == 0 {
		t.Error("expected inventory entries for Osui")
	}
}

func TestQuarmyPath(t *testing.T) {
	got := QuarmyPath("/eq", "Aradune")
	want := filepath.Join("/eq", "Aradune-Quarmy.txt")
	if got != want {
		t.Errorf("QuarmyPath = %q, want %q", got, want)
	}
}
