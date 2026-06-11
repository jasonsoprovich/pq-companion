package logparser

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func writeTestLog(t *testing.T, lines []string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "eqlog_Test_pq.proj.txt")
	content := ""
	for _, l := range lines {
		content += l + "\r\n"
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write test log: %v", err)
	}
	return path
}

func TestReplayer_WindowAndOrder(t *testing.T) {
	path := writeTestLog(t, []string{
		"[Mon Apr 13 06:00:00 2026] before the window",
		"[Mon Apr 13 06:01:00 2026] first in window",
		"not a log line at all",
		"[Mon Apr 13 06:01:01 2026] second in window",
		"[Mon Apr 13 06:01:02 2026] You have entered The North Karana.",
		"[Mon Apr 13 06:05:00 2026] after the window",
	})

	var mu sync.Mutex
	var lines []string
	var events []LogEvent
	var sessions []bool

	r := NewReplayer(
		func(ev LogEvent) {
			mu.Lock()
			events = append(events, ev)
			mu.Unlock()
		},
		func(ts time.Time, msg string) {
			mu.Lock()
			lines = append(lines, msg)
			mu.Unlock()
		},
		func(active bool) {
			mu.Lock()
			sessions = append(sessions, active)
			mu.Unlock()
		},
		nil,
	)

	from := time.Date(2026, 4, 13, 6, 1, 0, 0, time.Local)
	to := time.Date(2026, 4, 13, 6, 2, 0, 0, time.Local)
	if err := r.Start(path, "eqlog_Test_pq.proj.txt", from, to, 100); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Starting again while active must fail.
	if err := r.Start(path, "x", from, to, 100); err == nil {
		t.Errorf("second Start should error while a session is active")
	}

	// Wait for the session to finish (1s of log time at 100× + slack).
	deadline := time.Now().Add(5 * time.Second)
	for {
		if r.Status().State == ReplayIdle {
			mu.Lock()
			done := len(sessions) == 2
			mu.Unlock()
			if done {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("replay did not finish; status=%+v", r.Status())
		}
		time.Sleep(20 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	want := []string{"first in window", "second in window", "You have entered The North Karana."}
	if len(lines) != len(want) {
		t.Fatalf("lines = %q, want %q", lines, want)
	}
	for i := range want {
		if lines[i] != want[i] {
			t.Errorf("line %d = %q, want %q", i, lines[i], want[i])
		}
	}
	// The zone line must have produced a parsed event for downstream consumers.
	foundZone := false
	for _, ev := range events {
		if ev.Type == EventZone {
			foundZone = true
		}
	}
	if !foundZone {
		t.Errorf("expected a zone LogEvent from the replayed line; events=%+v", events)
	}
	// Session bracketed: started (true) then ended (false).
	if len(sessions) != 2 || !sessions[0] || sessions[1] {
		t.Errorf("session callbacks = %v, want [true false]", sessions)
	}

	st := r.Status()
	if st.LinesEmitted != 3 {
		t.Errorf("LinesEmitted = %d, want 3", st.LinesEmitted)
	}
}

func TestReplayer_StopAborts(t *testing.T) {
	// A 10-minute gap at 1× would stall for replayMaxGap; Stop must abort
	// promptly mid-sleep.
	path := writeTestLog(t, []string{
		"[Mon Apr 13 06:00:00 2026] line one",
		"[Mon Apr 13 06:10:00 2026] line two",
	})

	r := NewReplayer(func(LogEvent) {}, func(time.Time, string) {}, nil, nil)
	from := time.Date(2026, 4, 13, 6, 0, 0, 0, time.Local)
	to := time.Date(2026, 4, 13, 7, 0, 0, 0, time.Local)
	if err := r.Start(path, "t", from, to, 1); err != nil {
		t.Fatalf("Start: %v", err)
	}
	time.Sleep(50 * time.Millisecond)
	stopped := time.Now()
	r.Stop()

	deadline := time.Now().Add(2 * time.Second)
	for r.Status().State != ReplayIdle {
		if time.Now().After(deadline) {
			t.Fatalf("Stop did not end the session")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if elapsed := time.Since(stopped); elapsed > time.Second {
		t.Errorf("Stop took %v, want prompt abort", elapsed)
	}
}
