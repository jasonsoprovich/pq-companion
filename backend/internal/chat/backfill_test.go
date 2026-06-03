package chat

import (
	"bufio"
	"os"
	"path/filepath"
	"testing"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

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

// TestBackfillRealLog drives the chat backfill over the committed Osui fixture:
// it captures the Laoding tell conversation, picks up multiple channels
// (guild/raid/ooc/named), and is idempotent on a second pass.
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
		t.Fatal("backfill inserted 0 messages; expected chat in the fixture")
	}
	if n := runBackfill(t, s, logPath, "Osui"); n != 0 {
		t.Errorf("re-run inserted %d, want 0 (idempotent)", n)
	}

	// Tell conversation with Laoding, both directions.
	thread, err := s.Thread("Osui", "Laoding", false)
	if err != nil {
		t.Fatalf("Thread: %v", err)
	}
	if len(thread) == 0 {
		t.Error("expected Laoding tell thread from fixture")
	}

	// The fixture has named-channel (Lfg/General) traffic — those channels must
	// now be tracked (they were previously excluded as "not a tell").
	chans, _ := s.Channels("Osui")
	if len(chans) < 2 {
		t.Errorf("expected multiple channels from fixture, got %v", chans)
	}
}
