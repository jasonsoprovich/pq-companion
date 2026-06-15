package logparser

import "testing"

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
