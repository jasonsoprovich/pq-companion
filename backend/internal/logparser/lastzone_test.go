package logparser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeLogBody writes an eqlog file with the given contents and returns its
// directory (the "EQ path" LastZoneInLog expects).
func writeLogBody(t *testing.T, character, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "eqlog_"+character+"_pq.proj.txt")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}
	return dir
}

func TestLastZoneInLog_PicksLastRealZone(t *testing.T) {
	body := strings.Join([]string{
		"[Wed May 20 21:10:00 2026] You have entered The Nexus.",
		"[Wed May 20 21:11:00 2026] You have entered Plane of Knowledge.",
		"[Wed May 20 21:12:00 2026] You say, 'hi'",
		"[Wed May 20 21:13:00 2026] You have entered an area where levitation effects do not function.",
		"[Wed May 20 21:14:00 2026] A bat hits you for 3 points of damage.",
		"",
	}, "\n")
	eqPath := writeLogBody(t, "Osui", body)

	zone, ts, ok := LastZoneInLog(eqPath, "Osui")
	if !ok {
		t.Fatal("expected a zone, got ok=false")
	}
	if zone != "Plane of Knowledge" {
		t.Errorf("zone = %q, want %q (the levitation line must be skipped)", zone, "Plane of Knowledge")
	}
	if got := ts.Format("15:04"); got != "21:11" {
		t.Errorf("timestamp = %s, want 21:11", got)
	}
}

func TestLastZoneInLog_NoZoneLine(t *testing.T) {
	eqPath := writeLogBody(t, "Feane", "[Wed May 20 21:10:00 2026] You say, 'nothing here'\n")
	if _, _, ok := LastZoneInLog(eqPath, "Feane"); ok {
		t.Error("expected ok=false when the log has no zone-in line")
	}
}

func TestLastZoneInLog_MissingFile(t *testing.T) {
	if _, _, ok := LastZoneInLog(t.TempDir(), "Ghost"); ok {
		t.Error("expected ok=false for a missing log file")
	}
	if _, _, ok := LastZoneInLog("", "Ghost"); ok {
		t.Error("expected ok=false for an empty eqPath")
	}
}

func TestLastZoneInLog_ZoneStraddlesChunkBoundary(t *testing.T) {
	// Put the only zone-in line near the very start, then pad past the 1 MiB
	// backward-read chunk so the parser has to stitch it across a boundary.
	var b strings.Builder
	b.WriteString("[Wed May 20 20:00:00 2026] You have entered Firiona Vie.\n")
	filler := "[Wed May 20 20:00:01 2026] The bat tries to hit you, but misses!\n"
	for b.Len() < (2 << 20) {
		b.WriteString(filler)
	}
	eqPath := writeLogBody(t, "Nariana", b.String())

	zone, _, ok := LastZoneInLog(eqPath, "Nariana")
	if !ok || zone != "Firiona Vie" {
		t.Fatalf("zone = %q ok = %v, want %q true", zone, ok, "Firiona Vie")
	}
}
