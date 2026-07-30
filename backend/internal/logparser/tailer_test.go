package logparser

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestParseChunkParksUnterminatedLine confirms a final line with no trailing
// newline stays in remainder (not dispatched) until flushed.
func TestParseChunkParksUnterminatedLine(t *testing.T) {
	tl := &Tailer{}
	_, raws := tl.parseChunk([]byte("[Mon Jun 15 12:00:05 2026] It will take about 10 more seconds to prepare camp."))
	if len(raws) != 0 {
		t.Fatalf("unterminated line should not dispatch yet, got %d raws", len(raws))
	}
	if len(tl.remainder) == 0 {
		t.Fatal("expected the unterminated line to be parked in remainder")
	}
}

// TestFlushStaleRemainder confirms the parked line is emitted only after it has
// survived one idle poll (stability gate), then exactly once.
func TestFlushStaleRemainder(t *testing.T) {
	tl := &Tailer{}
	const msg = "It will take about 10 more seconds to prepare camp."
	tl.parseChunk([]byte("[Mon Jun 15 12:00:05 2026] " + msg))

	// First idle poll: marks stable, emits nothing.
	if ev, raws := tl.flushStaleRemainder(); len(ev) != 0 || len(raws) != 0 {
		t.Fatalf("first idle poll should emit nothing, got ev=%d raws=%d", len(ev), len(raws))
	}
	// Second idle poll: flushes the line.
	_, raws := tl.flushStaleRemainder()
	if len(raws) != 1 || raws[0].msg != msg {
		t.Fatalf("expected flushed raw %q, got %+v", msg, raws)
	}
	// Remainder is now cleared — no double emit.
	if _, raws := tl.flushStaleRemainder(); len(raws) != 0 {
		t.Fatalf("flushed remainder must not re-emit, got %d", len(raws))
	}
}

// TestFlushStaleRemainderResetsOnNewData confirms fresh bytes restart the
// stability gate so an in-progress write is never emitted truncated.
func TestFlushStaleRemainderResetsOnNewData(t *testing.T) {
	tl := &Tailer{}
	tl.parseChunk([]byte("[Mon Jun 15 12:00:06 2026] You begin casting Complete He"))
	tl.flushStaleRemainder() // marks stable
	// More bytes complete the line before the next flush would fire.
	_, raws := tl.parseChunk([]byte("al.\n"))
	if len(raws) != 1 || raws[0].msg != "You begin casting Complete Heal." {
		t.Fatalf("expected the completed line once, got %+v", raws)
	}
	if tl.remainderStable {
		t.Fatal("new data must clear the stability flag")
	}
}

// TestReadLinesReopensOnFileReplacement confirms that deleting and recreating
// the tailed file at the same path (the "reset a corrupted log" step
// FILE_SHARE_DELETE, b558f40, deliberately allows to succeed) is detected via
// file identity rather than left on the orphaned handle forever. The
// replacement file's pre-existing content must not be replayed, and content
// appended after the swap must still be delivered.
func TestReadLinesReopensOnFileReplacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "eqlog_Test_pq.proj.txt")
	if err := os.WriteFile(path, []byte("stale content\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	tl := &Tailer{}
	readLinesLocked(tl, path)
	if tl.file == nil {
		t.Fatal("expected a file handle to be open after first tick")
	}

	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("content the swap must not replay\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	events, raws := readLinesLocked(tl, path)
	if len(events) != 0 || len(raws) != 0 {
		t.Fatalf("replacement file's pre-existing content must not be replayed, got events=%d raws=%d", len(events), len(raws))
	}

	appended := "[Mon Jun 15 12:00:05 2026] You have entered The North Karana.\n"
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(appended); err != nil {
		t.Fatal(err)
	}
	f.Close()

	events, _ = readLinesLocked(tl, path)
	if len(events) != 1 || events[0].Type != EventZone {
		t.Fatalf("expected the post-swap zone event to be picked up, got %+v", events)
	}
}

// TestClockGuardPassesThroughInSyncTimestamp confirms a log timestamp close
// to the system clock (normal single-machine operation) is left untouched.
func TestClockGuardPassesThroughInSyncTimestamp(t *testing.T) {
	tl := &Tailer{}
	ts := time.Now().Add(-2 * time.Second)
	if got := tl.clockGuard(ts); !got.Equal(ts) {
		t.Fatalf("expected in-sync timestamp to pass through unchanged, got %v want %v", got, ts)
	}
}

// TestClockGuardOverridesSkewedTimestamp confirms a timestamp that disagrees
// with the system clock by more than maxLogClockSkew (e.g. the log file was
// written by a different machine with a mismatched clock — Larcen's report,
// a ~5hr/300min skew) falls back to receipt time instead of wedging every
// downstream timer at a wildly wrong expiry.
func TestClockGuardOverridesSkewedTimestamp(t *testing.T) {
	tl := &Tailer{}
	skewed := time.Now().Add(-5 * time.Hour)
	before := time.Now()
	got := tl.clockGuard(skewed)
	after := time.Now()
	if got.Before(before) || got.After(after) {
		t.Fatalf("expected skewed timestamp to be replaced with receipt time, got %v (want between %v and %v)", got, before, after)
	}
}

// TestClockGuardHandlesFutureSkew confirms the guard catches skew in either
// direction, not just a log timestamp lagging behind the system clock.
func TestClockGuardHandlesFutureSkew(t *testing.T) {
	tl := &Tailer{}
	skewed := time.Now().Add(5 * time.Hour)
	before := time.Now()
	got := tl.clockGuard(skewed)
	after := time.Now()
	if got.Before(before) || got.After(after) {
		t.Fatalf("expected future-skewed timestamp to be replaced with receipt time, got %v", got)
	}
}

// readLinesLocked calls the mutex-guarded readLines the way tick() does.
func readLinesLocked(tl *Tailer, path string) ([]LogEvent, []rawLine) {
	tl.mu.Lock()
	defer tl.mu.Unlock()
	return tl.readLines(path)
}
