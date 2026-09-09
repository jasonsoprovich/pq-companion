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
	"log/slog"
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

type active struct {
	key string
	h   Handler
}

// Run replays a single log file. It is exactly RunMulti with a one-element
// path list — kept as a named entry point for the common case.
func (r *Registry) Run(logPath, character string, keys []string, progress func(done, total int64)) (map[string]int, error) {
	return r.RunMulti([]string{logPath}, character, keys, progress)
}

// RunMulti replays each path in order through ONE shared set of handlers for
// the requested section keys, attributing rows to character, calling
// Finalize once after the last path. Returns inserted/updated counts keyed
// by section. Unknown keys are ignored; an empty selection or empty path
// list is a no-op.
//
// Paths should be ordered oldest-first (archives, then the live log) so that
// handlers tracking "latest seen" state advance monotonically. Each handler
// dedups on a natural key, so the ~30-day overlap between an archive and the
// next file inserts nothing the second time and re-running stays safe.
//
// A path is opened via logparser.OpenArchive when it ends ".zip" (streaming
// the log entry out of the .bak.zip) and as a plain file otherwise, so the
// live log and legacy uncompressed .bak.txt archives both just work. A path
// that can't be opened or read is logged and skipped — its share of the
// byte budget is still credited so the bar reaches 100% — rather than
// failing the whole batch for one bad archive.
//
// progress, if non-nil, is called periodically with bytes processed across
// ALL paths and their summed (uncompressed) size, so a multi-file scan
// renders as one continuous bar. It fires once at the start (0/total) and
// once at the end (total/total).
func (r *Registry) RunMulti(paths []string, character string, keys []string, progress func(done, total int64)) (map[string]int, error) {
	want := make(map[string]bool, len(keys))
	for _, k := range keys {
		want[k] = true
	}
	var handlers []active
	for _, s := range r.sections {
		if want[s.Key] {
			handlers = append(handlers, active{key: s.Key, h: s.NewHandler(character)})
		}
	}
	res := map[string]int{}
	if len(handlers) == 0 || len(paths) == 0 {
		return res, nil
	}

	sizes := make([]int64, len(paths))
	var total int64
	for i, p := range paths {
		if n, err := logparser.ArchiveUncompressedSize(p); err == nil {
			sizes[i] = n
			total += n
		}
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

	var base int64 // bytes from paths already fully scanned
	for i, p := range paths {
		if err := scanPath(p, handlers, func(n int64) { done = base + n; emit(false) }); err != nil {
			slog.Warn("backfill: skipping unreadable log", "path", p, "err", err)
		}
		base += sizes[i]
		done = base
		emit(false)
	}

	for _, a := range handlers {
		a.h.Finalize()
		res[a.key] = a.h.Inserted()
	}
	emit(true) // 100%
	return res, nil
}

// scanPath streams one log file (plain, .bak.txt, or .bak.zip) through every
// handler. onBytes is called after each line with the running byte count for
// this path (progress within the file).
func scanPath(path string, handlers []active, onBytes func(n int64)) error {
	f, err := logparser.OpenArchive(path) // plain os.Open for non-.zip paths
	if err != nil {
		return fmt.Errorf("open log: %w", err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	var n int64
	for sc.Scan() {
		line := sc.Text()
		n += int64(len(line)) + 1 // +1 approximates the stripped newline
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
		onBytes(n)
	}
	if err := sc.Err(); err != nil {
		return fmt.Errorf("read log: %w", err)
	}
	return nil
}
