package logparser

import (
	"context"
	"testing"
)

// browseAll pages through a file with the given filters and returns every line
// the browser yields, newest-first, following the offset cursor to exhaustion.
func browseAll(t *testing.T, path, q, evType string, limit int) []BrowseLine {
	t.Helper()
	var all []BrowseLine
	var before int64
	for i := 0; i < 1000; i++ { // guard against a cursor that never terminates
		res, err := BrowseLines(context.Background(), path, before, q, evType, limit)
		if err != nil {
			t.Fatalf("BrowseLines: %v", err)
		}
		all = append(all, res.Lines...)
		if res.NextOffset == nil {
			return all
		}
		if *res.NextOffset >= before && before != 0 {
			t.Fatalf("cursor did not advance: before=%d next=%d", before, *res.NextOffset)
		}
		before = *res.NextOffset
	}
	t.Fatal("pagination did not terminate")
	return nil
}

func TestBrowseLines_OrderAndRawFallback(t *testing.T) {
	path := writeTestLog(t, []string{
		"[Mon Apr 13 06:00:00 2026] You have entered The North Karana.",
		"[Mon Apr 13 06:00:01 2026] You say, 'hello there'", // unclassified -> log:raw
		"no timestamp here, should be skipped",
		"[Mon Apr 13 06:00:02 2026] You slash a gnoll for 150 points of damage.",
	})

	lines := browseAll(t, path, "", "", 300)
	if len(lines) != 3 {
		t.Fatalf("want 3 lines (garbage skipped), got %d", len(lines))
	}
	// Newest-first.
	if lines[0].Type != EventCombatHit {
		t.Errorf("line[0] type = %q, want %q", lines[0].Type, EventCombatHit)
	}
	if lines[1].Type != EventRaw {
		t.Errorf("line[1] type = %q, want %q (unclassified -> raw)", lines[1].Type, EventRaw)
	}
	if lines[2].Type != EventZone {
		t.Errorf("line[2] type = %q, want %q", lines[2].Type, EventZone)
	}
	if lines[1].Message != "You say, 'hello there'" {
		t.Errorf("raw message = %q", lines[1].Message)
	}
	// Offsets strictly decrease newest -> oldest, and the oldest is 0.
	if lines[2].Offset != 0 {
		t.Errorf("oldest line offset = %d, want 0", lines[2].Offset)
	}
	if !(lines[0].Offset > lines[1].Offset && lines[1].Offset > lines[2].Offset) {
		t.Errorf("offsets not strictly decreasing: %d %d %d",
			lines[0].Offset, lines[1].Offset, lines[2].Offset)
	}
}

func TestBrowseLines_PaginationAcrossChunks(t *testing.T) {
	// Enough lines to span many backward chunks and exercise the partial-line
	// carry across chunk boundaries.
	const n = 5000
	src := make([]string, n)
	for i := 0; i < n; i++ {
		// Vary the second field so messages are long-ish and offsets advance.
		src[i] = "[Mon Apr 13 06:00:00 2026] You slash a gnoll for " +
			itoa(i) + " points of damage."
	}
	path := writeTestLog(t, src)

	// Page in small batches; every line must come back exactly once, newest
	// (last-written) first, with no gaps or dupes.
	got := browseAll(t, path, "", "", 37)
	if len(got) != n {
		t.Fatalf("paged %d lines, want %d", len(got), n)
	}
	for i := 0; i < n; i++ {
		want := itoa(n - 1 - i) // newest-first
		// message form: "... for <num> points of damage."
		if !hasNum(got[i].Message, want) {
			t.Fatalf("line %d message = %q, want damage %s", i, got[i].Message, want)
		}
	}
}

func TestBrowseLines_FilterByTypeAndQuery(t *testing.T) {
	path := writeTestLog(t, []string{
		"[Mon Apr 13 06:00:00 2026] You slash a gnoll for 10 points of damage.",
		"[Mon Apr 13 06:00:01 2026] You have entered The North Karana.",
		"[Mon Apr 13 06:00:02 2026] You slash a kobold for 20 points of damage.",
		"[Mon Apr 13 06:00:03 2026] You say, 'gnoll incoming'",
	})

	hits := browseAll(t, path, "", "log:combat_hit", 300)
	if len(hits) != 2 {
		t.Fatalf("type filter: got %d, want 2", len(hits))
	}

	gnoll := browseAll(t, path, "gnoll", "", 300)
	if len(gnoll) != 2 { // the combat hit + the say line both contain "gnoll"
		t.Fatalf("query filter: got %d, want 2", len(gnoll))
	}
}

// small helpers so the test has no extra imports.

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func hasNum(msg, num string) bool {
	target := "for " + num + " points"
	for i := 0; i+len(target) <= len(msg); i++ {
		if msg[i:i+len(target)] == target {
			return true
		}
	}
	return false
}
