package lockout

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

// feedFixture replays every line of the /sll sample through the consumer the
// same way the tailer would (ParseRawLine → HandleLine), then flushes.
func feedFixture(t *testing.T, c *Consumer) {
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
		c.HandleLine(ts, msg)
	}
	// The fixture ends with the block and no trailing unrelated line, so the
	// snapshot is committed via the shutdown flush (or it would commit on
	// idle in production).
	c.Shutdown()
}

func TestConsumerSnapshotFromFixture(t *testing.T) {
	s := openTestStore(t)
	c := NewConsumer(s, func() string { return "Tester" })

	var snapshotChar string
	c.SetOnSnapshot(func(char string) { snapshotChar = char })

	feedFixture(t, c)

	if snapshotChar != "Tester" {
		t.Errorf("onSnapshot fired with %q, want Tester", snapshotChar)
	}

	entries, err := s.ListByCharacter("Tester")
	if err != nil {
		t.Fatalf("ListByCharacter: %v", err)
	}

	// 57 loot rows (lines 2–58) + 1 legacy row (line 60) = 58.
	if len(entries) != 58 {
		t.Fatalf("got %d entries, want 58", len(entries))
	}

	var loot, legacy int
	byName := map[string][]Entry{}
	for _, e := range entries {
		switch e.Section {
		case SectionLoot:
			loot++
		case SectionLegacy:
			legacy++
		default:
			t.Errorf("unexpected section %q", e.Section)
		}
		byName[e.TargetName] = append(byName[e.TargetName], e)
	}
	if loot != 57 || legacy != 1 {
		t.Errorf("section counts = loot %d / legacy %d, want 57 / 1", loot, legacy)
	}

	// The /sll lines are all stamped "Sat May 23 12:00:04 2026" (local).
	base := time.Date(2026, time.May, 23, 12, 0, 4, 0, time.Local)

	// Available row → ExpiresAt 0.
	if got := byName["King Tranix"]; len(got) != 1 || got[0].ExpiresAt != 0 {
		t.Errorf("King Tranix = %+v, want one Available (ExpiresAt 0) row", got)
	}
	// Legacy Available row.
	if got := byName["Shining Metallic Robes"]; len(got) != 1 || got[0].Section != SectionLegacy || got[0].ExpiresAt != 0 {
		t.Errorf("Shining Metallic Robes = %+v, want one legacy Available row", got)
	}
	// Timed row → absolute expiry = line timestamp + remaining.
	wantNagafen := base.Add(5*time.Hour + 50*time.Minute + 55*time.Second).Unix()
	if got := byName["Lord Nagafen"]; len(got) != 1 || got[0].ExpiresAt != wantNagafen {
		t.Errorf("Lord Nagafen ExpiresAt = %v, want %d", got, wantNagafen)
	}
	// Backtick name parsed intact.
	if got := byName["Arch Lich Rhag`Zadune"]; len(got) != 1 {
		t.Errorf("Arch Lich Rhag`Zadune = %+v, want exactly one row", got)
	}
	// Duplicate name preserved as two distinct rows.
	if got := byName["Kaas Thox Xi Aten Ha Ra"]; len(got) != 2 {
		t.Errorf("Kaas Thox Xi Aten Ha Ra appears %d times, want 2", len(got))
	}
}

func TestConsumerNoActiveCharacterSkips(t *testing.T) {
	s := openTestStore(t)
	c := NewConsumer(s, func() string { return "" }) // no active character

	feedFixture(t, c)

	chars, err := s.Characters()
	if err != nil {
		t.Fatalf("Characters: %v", err)
	}
	if len(chars) != 0 {
		t.Fatalf("expected no snapshot committed without an active character, got %v", chars)
	}
}

func TestConsumerFlushesOnUnrelatedLine(t *testing.T) {
	s := openTestStore(t)
	c := NewConsumer(s, func() string { return "Tester" })
	ts := time.Unix(1_700_000_000, 0)

	c.HandleLine(ts, "=== Current Loot Lockouts ===")
	c.HandleLine(ts, "== King Tranix: Available")
	// An unrelated chat line ends the block and commits the snapshot
	// immediately (no need to wait for the idle timer).
	c.HandleLine(ts, "You say, 'hello'")

	entries, err := s.ListByCharacter("Tester")
	if err != nil {
		t.Fatalf("ListByCharacter: %v", err)
	}
	if len(entries) != 1 || entries[0].TargetName != "King Tranix" {
		t.Fatalf("got %+v, want a single King Tranix row flushed on the unrelated line", entries)
	}
}
