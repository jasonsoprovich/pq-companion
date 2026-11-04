package logparser

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jasonsoprovich/pq-companion/backend/internal/config"
)

// The EQ client's debug log, dbg.txt in the EQ folder, records the zone
// server each zone-in connects to:
//
//	[Mon Oct 12 16:00:00 2026]00521:Zone addr [pq.projectquarm.pq3.projectquarm.com:7312] received...
//	[Mon Oct 12 16:00:00 2026]00522:Zone info received.
//	[Mon Oct 12 16:00:01 2026]00523:         StartWorldDisplay: thedeep
//
// The host and port name one running copy of a zone. Two groups in the same
// zone on different ports are in different copies with their own spawns, so
// respawn timers are keyed by it. A port is only good while that copy runs:
// the server hands ports out from a pool, and the same zone can come back on
// another one.
//
// The file also holds install paths and hardware details. Only the zone-in
// lines above and the two session-end lines are read; nothing else is kept.

const (
	dbgLogName = "dbg.txt"

	dbgPollInterval = time.Second

	// dbgSeedBytes is how much of the end of dbg.txt is read when the reader
	// first opens it, to find the zone the client is in now. A zone-in writes
	// a few hundred lines at most.
	dbgSeedBytes = 256 << 10
)

var (
	reDbgZoneAddr = regexp.MustCompile(`^Zone addr \[([^\]\s:]+):(\d+)\]`)
	reDbgWorld    = regexp.MustCompile(`^StartWorldDisplay: (\S+)$`)
)

// ZoneCopy is the zone copy the client is in, from dbg.txt. Zone is empty
// when the client is at character select or has camped. Token is empty when
// the zone-in had no server address before it.
type ZoneCopy struct {
	Zone  string    // zone short name
	Token string    // "host:port" of the zone server
	Since time.Time // the StartWorldDisplay line's timestamp
}

// dbgState follows the allow-listed dbg.txt lines.
type dbgState struct {
	addr string // host:port from the last "Zone addr" line, until used
	cur  ZoneCopy
}

// feed applies one dbg.txt line and reports whether the current copy
// changed.
func (s *dbgState) feed(line string) bool {
	line = strings.TrimRight(line, "\r")
	if len(line) < tsLen || line[0] != '[' {
		return false
	}
	ts, err := time.ParseInLocation(tsLayout, line[:tsLen], time.Local)
	if err != nil {
		return false
	}
	msg := line[tsLen:]
	// Each message starts with a sequence number: "00521:".
	i := strings.IndexByte(msg, ':')
	if i < 0 {
		return false
	}
	if _, err := strconv.Atoi(msg[:i]); err != nil {
		return false
	}
	msg = strings.TrimSpace(msg[i+1:])

	switch {
	case strings.HasPrefix(msg, "Zone addr "):
		if m := reDbgZoneAddr.FindStringSubmatch(msg); m != nil {
			s.addr = m[1] + ":" + m[2]
		}
		return false
	case strings.HasPrefix(msg, "StartWorldDisplay: "):
		m := reDbgWorld.FindStringSubmatch(msg)
		if m == nil {
			return false
		}
		zone := strings.ToLower(m[1])
		// The zone's object, character and light files load under their
		// own names (thedeep_obj, thedeep_chr2, objects, lights).
		if strings.Contains(zone, "_") || zone == "objects" || zone == "lights" {
			return false
		}
		next := ZoneCopy{Zone: zone, Token: s.addr, Since: ts}
		s.addr = ""
		changed := next.Zone != s.cur.Zone || next.Token != s.cur.Token
		s.cur = next
		return changed
	case msg == "Starting char select." || strings.HasPrefix(msg, "*** EXITING"):
		s.addr = ""
		changed := s.cur.Zone != ""
		s.cur = ZoneCopy{}
		return changed
	}
	return false
}

// ZoneCopyReader follows dbg.txt for the zone copy the client is in. Each
// poll opens the file, reads what was appended and closes it again, so the
// client is never kept from writing, renaming or replacing it.
type ZoneCopyReader struct {
	cfgMgr   *config.Manager
	onChange func(ZoneCopy)

	mu        sync.Mutex
	path      string
	offset    int64
	remainder []byte
	// skipLine drops the next line: after seeking into the middle of the
	// file it starts partway through.
	skipLine bool
	state    dbgState
}

// NewZoneCopyReader returns a reader that calls onChange whenever the
// current zone copy changes, including once for the copy found when the
// file is first read.
func NewZoneCopyReader(cfgMgr *config.Manager, onChange func(ZoneCopy)) *ZoneCopyReader {
	return &ZoneCopyReader{cfgMgr: cfgMgr, onChange: onChange}
}

// Start polls dbg.txt until ctx is cancelled.
func (r *ZoneCopyReader) Start(ctx context.Context) {
	ticker := time.NewTicker(dbgPollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			path := ""
			if eq := r.cfgMgr.Get().EQPath; eq != "" {
				path = filepath.Join(eq, dbgLogName)
			}
			if cur, changed := r.poll(path); changed && r.onChange != nil {
				r.onChange(cur)
			}
		}
	}
}

// Current returns the zone copy last read from dbg.txt.
func (r *ZoneCopyReader) Current() ZoneCopy {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.state.cur
}

// poll reads whatever was appended to path since the last call and reports
// the current copy and whether it changed. A new path, or a file smaller
// than what was already read (the client started a new one), is read again
// from its last dbgSeedBytes.
func (r *ZoneCopyReader) poll(path string) (ZoneCopy, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	before := r.state.cur
	if path != r.path {
		r.path = path
		r.reset()
	}
	if path == "" {
		return r.state.cur, r.state.cur != before
	}
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			r.reset()
		}
		return r.state.cur, r.state.cur != before
	}
	size := info.Size()
	if size < r.offset {
		r.reset()
	}
	if r.offset == 0 && size > dbgSeedBytes {
		r.offset = size - dbgSeedBytes
		r.skipLine = true
	}
	if size > r.offset {
		r.readFrom(path, size)
	}
	return r.state.cur, r.state.cur != before
}

func (r *ZoneCopyReader) reset() {
	r.offset = 0
	r.remainder = nil
	r.skipLine = false
	r.state = dbgState{}
}

func (r *ZoneCopyReader) readFrom(path string, size int64) {
	f, err := openShared(path)
	if err != nil {
		return
	}
	defer f.Close() //nolint:errcheck
	if _, err := f.Seek(r.offset, io.SeekStart); err != nil {
		return
	}
	n := size - r.offset
	if n > maxReadPerTick {
		n = maxReadPerTick
	}
	buf := make([]byte, n)
	got, err := io.ReadFull(f, buf)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return
	}
	r.offset += int64(got)
	data := append(r.remainder, buf[:got]...)
	for {
		j := bytes.IndexByte(data, '\n')
		if j < 0 {
			break
		}
		line := data[:j]
		data = data[j+1:]
		if r.skipLine {
			r.skipLine = false
			continue
		}
		r.state.feed(string(line))
	}
	r.remainder = append([]byte(nil), data...)
}
