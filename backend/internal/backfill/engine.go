// Package backfill replays a character's entire EQ log file through one or
// more registered trackers ("sections") to retroactively populate them. The
// log is read ONCE and fanned out to every selected section, so backfilling
// several trackers for one character costs a single pass over a potentially
// large file.
//
// Each section provides a dedup-safe, timestamp-aware Handler: re-running a
// backfill is idempotent and never overwrites newer live data. This is the
// engine behind the Settings → Log Backfill panel; it is never run
// automatically because large logs take time to walk.
package backfill

import (
	"bufio"
	"fmt"
	"os"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/logparser"
)

// Handler consumes one character's log during a backfill. HandleEvent receives
// parsed events (zone changes, /who rows, …); HandleLine receives every raw
// line (for trackers that match text directly, like tells). Finalize is called
// once after the last line so a handler can flush any buffered state. Inserted
// reports how many rows the handler created or updated.
type Handler interface {
	HandleEvent(logparser.LogEvent)
	HandleLine(ts time.Time, msg string)
	Finalize()
	Inserted() int
}

// Section is a registered backfillable tracker. NewHandler builds a fresh
// handler bound to the character being backfilled (used to attribute rows and
// stamp the owning character).
type Section struct {
	Key        string
	Label      string
	NewHandler func(character string) Handler
}

// SectionInfo is the public listing returned to the UI.
type SectionInfo struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// Registry holds the available sections in registration order.
type Registry struct {
	sections []Section
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry { return &Registry{} }

// Register adds a section. Call once per tracker at startup.
func (r *Registry) Register(s Section) { r.sections = append(r.sections, s) }

// Sections lists the registered sections for the UI.
func (r *Registry) Sections() []SectionInfo {
	out := make([]SectionInfo, 0, len(r.sections))
	for _, s := range r.sections {
		out = append(out, SectionInfo{Key: s.Key, Label: s.Label})
	}
	return out
}

// progressInterval throttles progress callbacks so a multi-minute scan emits a
// handful of updates per second rather than one per line.
const progressInterval = 150 * time.Millisecond

// Run replays logPath once, dispatching every line to the handlers for the
// requested section keys, attributing rows to character. Returns
// inserted/updated counts keyed by section. Unknown keys are ignored; an empty
// selection is a no-op.
//
// progress, if non-nil, is called periodically with the number of bytes
// processed and the total file size, so callers can render a progress bar and
// ETA for long backfills. It is always called once at the start (0/total) and
// once at the end (total/total).
func (r *Registry) Run(logPath, character string, keys []string, progress func(done, total int64)) (map[string]int, error) {
	want := make(map[string]bool, len(keys))
	for _, k := range keys {
		want[k] = true
	}
	type active struct {
		key string
		h   Handler
	}
	var handlers []active
	for _, s := range r.sections {
		if want[s.Key] {
			handlers = append(handlers, active{key: s.Key, h: s.NewHandler(character)})
		}
	}
	res := map[string]int{}
	if len(handlers) == 0 {
		return res, nil
	}

	f, err := os.Open(logPath)
	if err != nil {
		return nil, fmt.Errorf("open log: %w", err)
	}
	defer f.Close()

	var total int64
	if fi, err := f.Stat(); err == nil {
		total = fi.Size()
	}
	var done int64
	var lastEmit time.Time
	emit := func(force bool) {
		if progress == nil {
			return
		}
		now := time.Now()
		if force || now.Sub(lastEmit) >= progressInterval {
			lastEmit = now
			progress(done, total)
		}
	}
	emit(true) // 0 / total so the bar appears immediately

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Text()
		done += int64(len(line)) + 1 // +1 approximates the stripped newline
		ts, msg, ok := logparser.ParseRawLine(line)
		if ok {
			if ev, ok := logparser.ParseLine(line); ok {
				for _, a := range handlers {
					a.h.HandleEvent(ev)
				}
			}
			for _, a := range handlers {
				a.h.HandleLine(ts, msg)
			}
		}
		emit(false)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read log: %w", err)
	}
	for _, a := range handlers {
		a.h.Finalize()
		res[a.key] = a.h.Inserted()
	}
	emit(true) // 100%
	return res, nil
}
