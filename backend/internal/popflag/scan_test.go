package popflag

import (
	"os"
	"path/filepath"
	"testing"
)

func writeLog(t *testing.T, lines []string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "eqlog_Osui_pq.proj.txt")
	if err := os.WriteFile(path, []byte(joinLines(lines)), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}
	return path
}

func joinLines(lines []string) string {
	out := ""
	for _, l := range lines {
		out += l + "\n"
	}
	return out
}

// TestScanLogForSeerLatest builds a log with an older reading and a newer one
// separated by ordinary lines, and verifies the scan returns the most recent
// burst (and only that burst's lines).
func TestScanLogForSeerLatest(t *testing.T) {
	path := writeLog(t, []string{
		"[Mon Apr 13 06:00:00 2026] You slash a gnoll for 150 points of damage.",
		// Older reading: just the preflag.
		"[Mon Apr 13 06:01:00 2026] Alder Fuirstel wishes you to obtain the Ward.",
		"[Mon Apr 13 06:02:00 2026] You have entered the Plane of Knowledge.",
		// Newer reading: mavuin at 3, fuirstel at 4 (one tight burst).
		"[Mon Apr 13 07:00:00 2026] Mavuin is grateful to you for taking his case before the Tribunal.",
		"[Mon Apr 13 07:00:00 2026] Bertoxxulous has been slain, the curse from Milyk now lifted.",
		"[Mon Apr 13 07:05:00 2026] You say, 'hello'",
	})

	burst, found, err := ScanLogForSeer(path)
	if err != nil {
		t.Fatalf("ScanLogForSeer: %v", err)
	}
	if !found {
		t.Fatal("expected a Seer reading to be found")
	}
	if burst.LineCount != 2 {
		t.Errorf("burst line count = %d, want 2 (latest burst only)", burst.LineCount)
	}
	q := ParseSeer(burst.Text)
	if q["mavuin"] != "3" {
		t.Errorf("mavuin = %q, want 3 (from the latest burst)", q["mavuin"])
	}
	if q["fuirstel"] != "4" {
		t.Errorf("fuirstel = %q, want 4 (from the latest burst)", q["fuirstel"])
	}
}

// TestScanLogForSeerNone returns found=false when the log holds no reading.
func TestScanLogForSeerNone(t *testing.T) {
	path := writeLog(t, []string{
		"[Mon Apr 13 06:00:00 2026] You slash a gnoll for 150 points of damage.",
		"[Mon Apr 13 06:00:05 2026] You have entered the Plane of Knowledge.",
	})
	_, found, err := ScanLogForSeer(path)
	if err != nil {
		t.Fatalf("ScanLogForSeer: %v", err)
	}
	if found {
		t.Error("expected no Seer reading")
	}
}

// TestScanLogForSeerMissing surfaces a missing file as a not-exist error so the
// handler can map it to a friendly "no log" response.
func TestScanLogForSeerMissing(t *testing.T) {
	_, _, err := ScanLogForSeer(filepath.Join(t.TempDir(), "nope.txt"))
	if err == nil {
		t.Fatal("expected an error for a missing file")
	}
	if !os.IsNotExist(err) {
		t.Errorf("expected an IsNotExist error, got %v", err)
	}
}
