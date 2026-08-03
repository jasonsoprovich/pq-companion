package lockout

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

// feedBackfill replays every line of the /sll sample through a BackfillHandler
// the same way backfill.Registry.Run would.
func feedBackfill(t *testing.T, h *BackfillHandler) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "sll-sample.log"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		ts, msg, ok := logparser.ParseRawLine(line)
		if !ok {
			t.Fatalf("fixture line did not parse: %q", line)
		}
		h.HandleLine(ts, msg)
	}
	h.Finalize()
}

func TestBackfillHandlerFromFixture(t *testing.T) {
	s := openTestStore(t)
	h := NewBackfillHandler(s, "Tester")

	feedBackfill(t, h)

	entries, err := s.ListByCharacter("Tester")
	if err != nil {
		t.Fatalf("ListByCharacter: %v", err)
	}
	// Matches TestConsumerSnapshotFromFixture: 57 loot rows + 1 legacy row.
	if len(entries) != 58 {
		t.Fatalf("got %d entries, want 58", len(entries))
	}
	if h.Inserted() != 58 {
		t.Errorf("Inserted() = %d, want 58", h.Inserted())
	}
}

func TestBackfillHandlerKillNotice(t *testing.T) {
	s := openTestStore(t)
	h := NewBackfillHandler(s, "Tester")

	ts := time.Unix(1_700_000_000, 0)
	h.HandleLine(ts, "You have incurred a lockout for Diabo Xi Xin Thall that expires in 6 Days and 18 Hours.")
	h.Finalize()

	entries, err := s.ListByCharacter("Tester")
	if err != nil {
		t.Fatalf("ListByCharacter: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	e := entries[0]
	if e.Section != SectionLoot || e.TargetName != "Diabo Xi Xin Thall" {
		t.Errorf("got %+v", e)
	}
	wantExpires := ts.Add(6*24*time.Hour + 18*time.Hour).Unix()
	if e.ExpiresAt != wantExpires {
		t.Errorf("ExpiresAt = %d, want %d", e.ExpiresAt, wantExpires)
	}
}

// TestBackfillHandlerNeverOverwritesNewerData is the whole reason backfill
// uses UpsertEntryIfNewer instead of Store.Snapshot: replaying an old or
// rotated log file must never clobber lockout data a live session already
// recorded more recently.
func TestBackfillHandlerNeverOverwritesNewerData(t *testing.T) {
	s := openTestStore(t)

	liveTS := time.Unix(1_700_100_000, 0)
	if err := s.UpsertEntry("Tester", SectionLoot, "Lord Nagafen", liveTS.Add(time.Hour), liveTS); err != nil {
		t.Fatalf("UpsertEntry (live): %v", err)
	}

	h := NewBackfillHandler(s, "Tester")
	staleTS := liveTS.Add(-time.Hour)
	h.HandleLine(staleTS, "You have incurred a lockout for Lord Nagafen that expires in 5 Hours.")
	h.Finalize()

	if h.Inserted() != 0 {
		t.Errorf("Inserted() = %d, want 0 — stale backfill row should have been skipped", h.Inserted())
	}

	entries, err := s.ListByCharacter("Tester")
	if err != nil {
		t.Fatalf("ListByCharacter: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	wantExpires := liveTS.Add(time.Hour).Unix()
	if entries[0].ExpiresAt != wantExpires {
		t.Errorf("live row was overwritten: ExpiresAt = %d, want %d", entries[0].ExpiresAt, wantExpires)
	}
}

// TestBackfillHandlerStaleBlockSkipped is TestBackfillHandlerNeverOverwritesNewerData
// for the `/sll`-block commit path rather than the kill-notice path.
func TestBackfillHandlerStaleBlockSkipped(t *testing.T) {
	s := openTestStore(t)

	liveTS := time.Unix(1_700_100_000, 0)
	if err := s.UpsertEntry("Tester", SectionLoot, "Lord Nagafen", liveTS.Add(time.Hour), liveTS); err != nil {
		t.Fatalf("UpsertEntry (live): %v", err)
	}

	h := NewBackfillHandler(s, "Tester")
	staleTS := liveTS.Add(-time.Hour)
	h.HandleLine(staleTS, "=== Current Loot Lockouts ===")
	h.HandleLine(staleTS, "== Lord Nagafen: Available")
	h.HandleLine(staleTS, "== King Tranix: Available")
	h.Finalize()

	if h.Inserted() != 0 {
		t.Errorf("Inserted() = %d, want 0 — stale /sll block should have been skipped whole", h.Inserted())
	}
	entries, err := s.ListByCharacter("Tester")
	if err != nil {
		t.Fatalf("ListByCharacter: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1 (King Tranix must not have been added from the stale block)", len(entries))
	}
}

// TestBackfillHandlerPreservesDuplicateNames guards against a per-row
// upsert-by-name design, which would silently collapse distinct raid
// instances that share a target name into a single row.
func TestBackfillHandlerPreservesDuplicateNames(t *testing.T) {
	s := openTestStore(t)
	h := NewBackfillHandler(s, "Tester")

	ts := time.Unix(1_700_000_000, 0)
	h.HandleLine(ts, "=== Current Loot Lockouts ===")
	h.HandleLine(ts, "== Kaas Thox Xi Aten Ha Ra: Available")
	h.HandleLine(ts, "== Kaas Thox Xi Aten Ha Ra: Expires in 5 Hours")
	h.Finalize()

	entries, err := s.ListByCharacter("Tester")
	if err != nil {
		t.Fatalf("ListByCharacter: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2 distinct instances", len(entries))
	}
}
