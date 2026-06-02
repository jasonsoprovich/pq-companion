package zeal

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// touch writes a file with the given mod time so "newest wins" is deterministic.
func touch(t *testing.T, dir, name string, mod time.Time) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	if err := os.Chtimes(p, mod, mod); err != nil {
		t.Fatalf("chtimes %s: %v", name, err)
	}
	return p
}

// FindInventoryFile must accept both /outputfile naming formats (#133) and,
// when both are present, return the most recently modified.
func TestFindInventoryFile_BothFormats(t *testing.T) {
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	t.Run("format 0 only", func(t *testing.T) {
		dir := t.TempDir()
		want := touch(t, dir, "Osui-Inventory.txt", base)
		if got := FindInventoryFile(dir, "Osui"); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("format 1 only", func(t *testing.T) {
		dir := t.TempDir()
		want := touch(t, dir, "Osui-Inventory_pq.proj.txt", base)
		if got := FindInventoryFile(dir, "Osui"); got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})

	t.Run("both present — newer wins", func(t *testing.T) {
		dir := t.TempDir()
		touch(t, dir, "Osui-Inventory.txt", base)
		newer := touch(t, dir, "Osui-Inventory_pq.proj.txt", base.Add(time.Hour))
		if got := FindInventoryFile(dir, "Osui"); got != newer {
			t.Errorf("expected the newer format-1 file %q, got %q", newer, got)
		}

		// Make format 0 the newer one — it should now win.
		older0 := touch(t, dir, "Osui-Inventory.txt", base.Add(2*time.Hour))
		if got := FindInventoryFile(dir, "Osui"); got != older0 {
			t.Errorf("expected the newer format-0 file %q, got %q", older0, got)
		}
	})

	t.Run("neither present", func(t *testing.T) {
		dir := t.TempDir()
		if got := FindInventoryFile(dir, "Osui"); got != "" {
			t.Errorf("expected empty path, got %q", got)
		}
	})
}

// ScanAllInventories must not return the same character twice when both naming
// formats are on disk — it keeps the most recent (#133).
func TestScanAllInventories_DedupesBothFormats(t *testing.T) {
	dir := t.TempDir()
	const header = "Location\tName\tID\tCount\tSlots\n"
	base := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)

	// Older format-0 export, newer format-1 export — same character.
	old := touch(t, dir, "Osui-Inventory.txt", base)
	if err := os.WriteFile(old, []byte(header+"General1\tRusty Dagger\t1001\t1\t0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(old, base, base)

	newp := touch(t, dir, "Osui-Inventory_pq.proj.txt", base.Add(time.Hour))
	if err := os.WriteFile(newp, []byte(header+"General1\tFine Steel Dagger\t1002\t1\t0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_ = os.Chtimes(newp, base.Add(time.Hour), base.Add(time.Hour))

	chars, _, err := ScanAllInventories(dir)
	if err != nil {
		t.Fatalf("scan: %v", err)
	}
	if len(chars) != 1 {
		t.Fatalf("expected 1 character (deduped), got %d", len(chars))
	}
	// The newer (format-1) export should have won.
	if len(chars[0].Entries) != 1 || chars[0].Entries[0].ID != 1002 {
		t.Errorf("expected the newer export (item 1002) to win, got %+v", chars[0].Entries)
	}
}
