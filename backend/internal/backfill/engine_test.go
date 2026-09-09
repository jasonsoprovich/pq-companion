package backfill

import (
	"archive/zip"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

// stubHandler counts lines and zone events it receives, and records the
// character it was built for.
type stubHandler struct {
	character string
	lines     int
	zones     int
}

func (h *stubHandler) HandleEvent(ev logparser.LogEvent) {
	if ev.Type == logparser.EventZone {
		h.zones++
	}
}
func (h *stubHandler) HandleLine(time.Time, string) { h.lines++ }
func (h *stubHandler) Finalize()                    {}
func (h *stubHandler) Inserted() int                { return h.lines }

func TestRegistryRun(t *testing.T) {
	log := `[Mon Apr 13 06:00:00 2026] You have entered The North Karana.
[Mon Apr 13 06:00:05 2026] Soandso tells you, 'hi'
[Mon Apr 13 06:00:10 2026] gibberish that is not a valid eq line
[Mon Apr 13 06:00:15 2026] You told Soandso, 'hey'
`
	dir := t.TempDir()
	path := filepath.Join(dir, "log.txt")
	if err := os.WriteFile(path, []byte(log), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	var built string
	r := NewRegistry()
	var captured *stubHandler
	r.Register(Section{
		Key:   "stub",
		Label: "Stub",
		NewHandler: func(character string) Handler {
			built = character
			captured = &stubHandler{character: character}
			return captured
		},
	})
	// A section that isn't selected must never have its handler built.
	r.Register(Section{
		Key:        "other",
		Label:      "Other",
		NewHandler: func(string) Handler { t.Fatal("unselected section built"); return nil },
	})

	var progressCalls int
	var lastDone, lastTotal int64
	res, err := r.Run(path, "Osui", []string{"stub"}, func(done, total int64) {
		progressCalls++
		lastDone, lastTotal = done, total
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if progressCalls < 2 || lastTotal == 0 || lastDone == 0 {
		t.Errorf("progress: calls=%d done=%d total=%d, want >=2 calls and non-zero final", progressCalls, lastDone, lastTotal)
	}
	if built != "Osui" {
		t.Errorf("handler built for %q, want Osui", built)
	}
	// 3 valid EQ lines (the gibberish line has a valid timestamp too → 4 lines
	// reach HandleLine; only lines with a parseable timestamp count).
	if captured.lines != 4 {
		t.Errorf("HandleLine called %d times, want 4 (timestamped lines)", captured.lines)
	}
	if captured.zones != 1 {
		t.Errorf("zone events = %d, want 1", captured.zones)
	}
	if res["stub"] != 4 {
		t.Errorf("result count = %d, want 4", res["stub"])
	}

	// Empty selection is a no-op.
	res2, err := r.Run(path, "Osui", nil, nil)
	if err != nil || len(res2) != 0 {
		t.Errorf("empty selection: res=%v err=%v, want empty/nil", res2, err)
	}
}

// dedupHandler records the distinct timestamped lines it has seen, so a
// re-scan of overlapping content inserts nothing the second time — the same
// contract every real backfill handler upholds via an INSERT OR IGNORE.
type dedupHandler struct {
	seen     map[string]bool
	inserted int
}

func newDedupHandler() *dedupHandler { return &dedupHandler{seen: map[string]bool{}} }

func (h *dedupHandler) HandleEvent(logparser.LogEvent) {}
func (h *dedupHandler) HandleLine(ts time.Time, msg string) {
	key := ts.Format(time.RFC3339) + "|" + msg
	if h.seen[key] {
		return
	}
	h.seen[key] = true
	h.inserted++
}
func (h *dedupHandler) Finalize()     {}
func (h *dedupHandler) Inserted() int { return h.inserted }

func writeZipLog(t *testing.T, path, entry, content string) {
	t.Helper()
	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	w, err := zw.Create(entry)
	if err != nil {
		t.Fatalf("zip entry: %v", err)
	}
	if _, err := io.WriteString(w, content); err != nil {
		t.Fatalf("zip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
}

func TestRunMultiOverlapAndZip(t *testing.T) {
	dir := t.TempDir()

	// Archive (zip): days 1-3. Live log: days 3-4 — day 3 overlaps.
	archiveContent := "[Mon Apr 13 06:00:00 2026] day one\n" +
		"[Tue Apr 14 06:00:00 2026] day two\n" +
		"[Wed Apr 15 06:00:00 2026] day three\n"
	liveContent := "[Wed Apr 15 06:00:00 2026] day three\n" +
		"[Thu Apr 16 06:00:00 2026] day four\n"

	archivePath := filepath.Join(dir, "eqlog_Grok_pq.proj.2026-04-15.bak.zip")
	writeZipLog(t, archivePath, "eqlog_Grok_pq.proj.txt", archiveContent)
	livePath := filepath.Join(dir, "eqlog_Grok_pq.proj.txt")
	if err := os.WriteFile(livePath, []byte(liveContent), 0o644); err != nil {
		t.Fatal(err)
	}

	var captured *dedupHandler
	r := NewRegistry()
	r.Register(Section{
		Key:   "stub",
		Label: "Stub",
		NewHandler: func(string) Handler {
			captured = newDedupHandler()
			return captured
		},
	})

	paths := []string{archivePath, livePath}

	var lastDone, lastTotal int64
	res, err := r.RunMulti(paths, "Grok", []string{"stub"}, func(done, total int64) {
		lastDone, lastTotal = done, total
	})
	if err != nil {
		t.Fatalf("RunMulti: %v", err)
	}
	// 4 distinct days across the two files; the shared "day three" line counts once.
	if res["stub"] != 4 {
		t.Errorf("distinct lines = %d, want 4 (overlap de-duplicated)", res["stub"])
	}
	if lastTotal == 0 || lastDone != lastTotal {
		t.Errorf("progress final = %d/%d, want done==total and non-zero", lastDone, lastTotal)
	}

	// Re-running over the same inputs must insert nothing new.
	res2, err := r.RunMulti(paths, "Grok", []string{"stub"}, nil)
	if err != nil {
		t.Fatalf("RunMulti rerun: %v", err)
	}
	if res2["stub"] != 4 {
		t.Errorf("rerun distinct lines = %d, want 4 (fresh handler, same inputs)", res2["stub"])
	}
}

func TestRunMultiSkipsUnreadablePath(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "eqlog_Grok_pq.proj.txt")
	if err := os.WriteFile(good, []byte("[Mon Apr 13 06:00:00 2026] hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "eqlog_Grok_pq.proj.2026-01-01.bak.zip") // does not exist

	r := NewRegistry()
	var captured *dedupHandler
	r.Register(Section{
		Key:        "stub",
		Label:      "Stub",
		NewHandler: func(string) Handler { captured = newDedupHandler(); return captured },
	})

	var lastDone, lastTotal int64
	res, err := r.RunMulti([]string{missing, good}, "Grok", []string{"stub"}, func(done, total int64) {
		lastDone, lastTotal = done, total
	})
	if err != nil {
		t.Fatalf("RunMulti with a missing path should not error: %v", err)
	}
	if res["stub"] != 1 {
		t.Errorf("lines from the readable path = %d, want 1", res["stub"])
	}
	if lastDone != lastTotal {
		t.Errorf("progress final = %d/%d, want done==total even with a skipped path", lastDone, lastTotal)
	}
}
