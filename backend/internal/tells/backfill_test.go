package tells

import (
	"bufio"
	"os"
	"path/filepath"
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

// runBackfill replays a log file through a BackfillHandler, mirroring what the
// backfill engine does (line-by-line HandleEvent + HandleLine). Kept local to
// avoid an import cycle on internal/backfill.
func runBackfill(t *testing.T, store *Store, path, character string) int {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	h := NewBackfillHandler(store, character)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		ts, msg, ok := logparser.ParseRawLine(line)
		if !ok {
			continue
		}
		if ev, ok := logparser.ParseLine(line); ok {
			h.HandleEvent(ev)
		}
		h.HandleLine(ts, msg)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan: %v", err)
	}
	h.Finalize()
	return h.Inserted()
}

// TestBackfillRealLog drives the tells backfill handler over the committed Osui
// fixture: it must capture the real Laoding conversation (both directions),
// exclude channel speakers, and be idempotent on a second pass.
func TestBackfillRealLog(t *testing.T) {
	logPath := filepath.Join("..", "..", "..", "testdata", "eqlog_Osui_pq.proj.txt")
	if _, err := os.Stat(logPath); err != nil {
		t.Skipf("testdata log not available: %v", err)
	}

	s, err := OpenStore(filepath.Join(t.TempDir(), "user.db"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	defer s.Close()

	if n := runBackfill(t, s, logPath, "Osui"); n == 0 {
		t.Fatal("backfill inserted 0 tells; expected real conversations in the fixture")
	}
	if n := runBackfill(t, s, logPath, "Osui"); n != 0 {
		t.Errorf("re-run inserted %d rows, want 0 (idempotent)", n)
	}

	msgs, err := s.Messages("Osui", "Laoding", false)
	if err != nil {
		t.Fatalf("Messages: %v", err)
	}
	if len(msgs) == 0 {
		t.Error("expected Laoding conversation from fixture, got none")
	}
	var sawIn, sawOut bool
	for _, m := range msgs {
		sawIn = sawIn || m.Direction == DirectionIn
		sawOut = sawOut || m.Direction == DirectionOut
	}
	if !sawIn || !sawOut {
		t.Errorf("Laoding thread missing a direction: in=%v out=%v", sawIn, sawOut)
	}

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
