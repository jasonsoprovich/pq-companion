package logparser

import "testing"

// Every line gets its own sequence number, shared by its raw line and its
// parsed event, and numbering carries on across chunks.
func TestParseChunk_SeqSharedByRawAndEvent(t *testing.T) {
	tl := &Tailer{}
	chunk := []byte("[Mon Apr 13 06:00:00 2026] You have entered The North Karana.\n" +
		"[Mon Apr 13 06:00:01 2026] some unparsed chatter\n" +
		"[Mon Apr 13 06:00:02 2026] You have entered West Karana.\n")
	events, raws := tl.parseChunk(chunk)
	if len(raws) != 3 || len(events) != 2 {
		t.Fatalf("got %d raws, %d events", len(raws), len(events))
	}
	for i, rl := range raws {
		if rl.seq != uint64(i+1) {
			t.Errorf("raw %d: seq %d", i, rl.seq)
		}
	}
	if events[0].Seq != raws[0].seq || events[1].Seq != raws[2].seq {
		t.Errorf("event seqs %d,%d don't match their raw lines", events[0].Seq, events[1].Seq)
	}
	events, raws = tl.parseChunk([]byte("[Mon Apr 13 06:00:03 2026] You have entered Qeynos Hills.\n"))
	if raws[0].seq != 4 || events[0].Seq != 4 {
		t.Errorf("numbering must carry on: raw %d event %d", raws[0].seq, events[0].Seq)
	}
}
