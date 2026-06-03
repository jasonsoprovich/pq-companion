package tells

import (
	"os"
	"path/filepath"
	"testing"
)

// TestScanFileRealLog runs ScanFile against the committed Osui testdata log and
// asserts it captures the known real conversation (Laoding) while excluding
// channel chatter (LFG/General) and any NPC merchant replies. Skipped when the
// fixture isn't present.
func TestScanFileRealLog(t *testing.T) {
	logPath := filepath.Join("..", "..", "..", "testdata", "eqlog_Osui_pq.proj.txt")
	if _, err := os.Stat(logPath); err != nil {
		t.Skipf("testdata log not available: %v", err)
	}

	s, err := OpenStore(filepath.Join(t.TempDir(), "user.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer s.Close()

	n, err := ScanFile(s, logPath, "Osui")
	if err != nil {
		t.Fatalf("ScanFile: %v", err)
	}
	if n == 0 {
		t.Fatal("scan inserted 0 tells; expected real conversations in the fixture")
	}

	// Re-scan is idempotent.
	n2, err := ScanFile(s, logPath, "Osui")
	if err != nil {
		t.Fatalf("ScanFile rescan: %v", err)
	}
	if n2 != 0 {
		t.Errorf("rescan inserted %d rows, want 0 (idempotent)", n2)
	}

	// The fixture contains a real back-and-forth with Laoding.
	msgs, err := s.Messages("Osui", "Laoding", false)
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	if len(msgs) == 0 {
		t.Error("expected Laoding conversation from fixture, got none")
	}
	var sawIn, sawOut bool
	for _, m := range msgs {
		if m.Direction == DirectionIn {
			sawIn = true
		}
		if m.Direction == DirectionOut {
			sawOut = true
		}
	}
	if !sawIn || !sawOut {
		t.Errorf("Laoding thread missing a direction: in=%v out=%v", sawIn, sawOut)
	}

	// Channel chatter must never become a "conversation". Theythem/Gernumbli
	// only ever spoke in LFG:3 in the fixture, so they must not appear.
	for _, peer := range []string{"Theythem", "Gernumbli", "Nekomancer"} {
		cm, err := s.Messages("Osui", peer, false)
		if err != nil {
			t.Fatalf("Messages(%s): %v", peer, err)
		}
		if len(cm) != 0 {
			t.Errorf("channel speaker %q leaked into tells (%d rows)", peer, len(cm))
		}
	}
}
