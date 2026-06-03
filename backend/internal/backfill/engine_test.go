package backfill

import (
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
