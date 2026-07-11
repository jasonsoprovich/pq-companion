package logparser

import (
	"bufio"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"
)

// Replay pacing knobs.
const (
	// replayMaxGap caps the real-time wait between two consecutive replayed
	// lines so AFK stretches in the log don't stall the session. The cap
	// applies after the speed divisor — a 10-minute gap at 1× still plays
	// back as replayMaxGap.
	replayMaxGap = 3 * time.Second
	// replayPollStep is how often the playback goroutine re-checks for
	// pause/stop while sleeping between lines.
	replayPollStep = 100 * time.Millisecond
	// replayStatusInterval is how often a position status update is pushed
	// while playing.
	replayStatusInterval = time.Second
	// replayScanBufSize is the bufio.Scanner max token size. EQ log lines are
	// short, but corrupted/binary junk shouldn't kill the session.
	replayScanBufSize = 1 << 20
)

// ReplayState enumerates the replayer's lifecycle states.
type ReplayState string

const (
	ReplayIdle    ReplayState = "idle"
	ReplayPlaying ReplayState = "playing"
	ReplayPaused  ReplayState = "paused"
)

// ReplayStatus is the REST/WebSocket snapshot of a replay session.
type ReplayStatus struct {
	State ReplayState `json:"state"`
	File  string      `json:"file,omitempty"` // base name of the log file being replayed
	From  *time.Time  `json:"from,omitempty"`
	To    *time.Time  `json:"to,omitempty"`
	// Position is the timestamp of the most recently emitted line.
	Position     *time.Time `json:"position,omitempty"`
	Speed        float64    `json:"speed,omitempty"`
	LinesEmitted int        `json:"lines_emitted"`
}

// Replayer streams a historical segment of an EQ log file through the same
// dispatch callbacks the live Tailer uses, pacing lines by their log
// timestamps so timers, triggers, the combat meter, and overlays behave as
// if the session were happening live. The file is opened read-only and never
// modified.
//
// One session at a time. onSession(true) fires when a session starts (the
// caller pauses the live tailer there); onSession(false) fires when it ends
// for any reason (resume tailing, clear replay-driven state).
type Replayer struct {
	handler     func(LogEvent)
	lineHandler func(time.Time, string)
	onSession   func(active bool)
	onStatus    func(ReplayStatus)

	mu       sync.Mutex
	state    ReplayState
	file     string // display name (base name)
	from, to time.Time
	position time.Time
	speed    float64
	lines    int
	stop     chan struct{} // closed to abort the playback goroutine
	paused   bool
}

// NewReplayer creates a Replayer. handler/lineHandler are the same callbacks
// wired into the live Tailer. onSession and onStatus may be nil.
func NewReplayer(handler func(LogEvent), lineHandler func(time.Time, string), onSession func(bool), onStatus func(ReplayStatus)) *Replayer {
	return &Replayer{
		handler:     handler,
		lineHandler: lineHandler,
		onSession:   onSession,
		onStatus:    onStatus,
		state:       ReplayIdle,
	}
}

// Status returns a snapshot of the current session.
func (r *Replayer) Status() ReplayStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.statusLocked()
}

func (r *Replayer) statusLocked() ReplayStatus {
	st := ReplayStatus{
		State:        r.state,
		File:         r.file,
		Speed:        r.speed,
		LinesEmitted: r.lines,
	}
	if r.state != ReplayIdle {
		from, to, pos := r.from, r.to, r.position
		st.From, st.To = &from, &to
		if !pos.IsZero() {
			st.Position = &pos
		}
	}
	return st
}

func (r *Replayer) pushStatus() {
	if r.onStatus == nil {
		return
	}
	r.mu.Lock()
	st := r.statusLocked()
	r.mu.Unlock()
	r.onStatus(st)
}

// Start begins replaying path between from and to at the given speed
// multiplier (1.0 = real time; min clamped to 0.1, max to 100). displayName
// is the base file name reported in status. Errors when a session is already
// active or the file can't be opened.
// Sentinel errors from Start so the API layer can map them to the right HTTP
// status (inverted range → 400, already active → 409) instead of lumping every
// failure into one code. A wrapped open error (neither sentinel) is a 500.
var (
	ErrReplayRangeInverted = errors.New("replay: 'to' must be after 'from'")
	ErrReplayAlreadyActive = errors.New("replay: a session is already active")
)

func (r *Replayer) Start(path, displayName string, from, to time.Time, speed float64) error {
	if speed <= 0 {
		speed = 1
	}
	if speed < 0.1 {
		speed = 0.1
	}
	if speed > 100 {
		speed = 100
	}
	if !to.After(from) {
		return ErrReplayRangeInverted
	}

	f, err := openShared(path)
	if err != nil {
		return fmt.Errorf("replay: open log: %w", err)
	}

	r.mu.Lock()
	if r.state != ReplayIdle {
		r.mu.Unlock()
		_ = f.Close()
		return ErrReplayAlreadyActive
	}
	stop := make(chan struct{})
	r.state = ReplayPlaying
	r.paused = false
	r.file = displayName
	r.from, r.to = from, to
	r.position = time.Time{}
	r.speed = speed
	r.lines = 0
	r.stop = stop
	r.mu.Unlock()

	if r.onSession != nil {
		r.onSession(true)
	}
	r.pushStatus()
	slog.Info("replay: session started", "file", displayName, "from", from, "to", to, "speed", speed)

	go r.run(f, stop)
	return nil
}

// Pause suspends an active session (no-op when idle or already paused).
func (r *Replayer) Pause() {
	r.mu.Lock()
	if r.state == ReplayPlaying {
		r.state = ReplayPaused
		r.paused = true
	}
	r.mu.Unlock()
	r.pushStatus()
}

// Resume continues a paused session (no-op otherwise).
func (r *Replayer) Resume() {
	r.mu.Lock()
	if r.state == ReplayPaused {
		r.state = ReplayPlaying
		r.paused = false
	}
	r.mu.Unlock()
	r.pushStatus()
}

// Stop aborts the active session. No-op when idle.
func (r *Replayer) Stop() {
	r.mu.Lock()
	stop := r.stop
	active := r.state != ReplayIdle
	// Claim the channel under the lock so a concurrent/repeat Stop() can't also
	// close it. finish() later sets r.stop = nil again, which is harmless.
	if active && stop != nil {
		r.stop = nil
	}
	r.mu.Unlock()
	if active && stop != nil {
		close(stop) // run() finalizes state + callbacks
	}
}

// run is the playback goroutine. It owns the file handle.
func (r *Replayer) run(f *os.File, stop <-chan struct{}) {
	defer f.Close()
	defer r.finish()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), replayScanBufSize)

	var prevTS time.Time
	lastStatus := time.Now()

	for scanner.Scan() {
		select {
		case <-stop:
			return
		default:
		}

		ts, msg, ok := ParseRawLine(scanner.Text())
		if !ok {
			continue
		}
		if ts.Before(r.from) {
			continue
		}
		if ts.After(r.to) {
			return // past the requested window — done
		}

		// Pace by the log's own inter-line gap, scaled by speed and capped.
		if !prevTS.IsZero() {
			wait := time.Duration(float64(ts.Sub(prevTS)) / r.speedNow())
			if wait > replayMaxGap {
				wait = replayMaxGap
			}
			if !r.sleepInterruptible(wait, stop) {
				return
			}
		}
		// Block here while paused (also aborts promptly on stop).
		if !r.waitWhilePaused(stop) {
			return
		}
		prevTS = ts

		// Same per-line order as the live tailer: raw line first (triggers),
		// then the parsed event for every other consumer. Consumers are handed
		// the real dispatch time, not the line's original log timestamp — the
		// spell timer and trigger engines compute expiresAt as
		// (timestamp + duration), so a historical timestamp (anything but
		// "just now") produces an expiry already in the past and the overlay
		// disappears within a frame of appearing. ts/prevTS (the parsed log
		// time) still drive pacing, range filtering, and position reporting
		// below — only the timestamp forwarded to downstream state is remapped.
		dispatchTS := time.Now()
		if r.lineHandler != nil {
			r.lineHandler(dispatchTS, msg)
		}
		if ev, ok := ParseLine(scanner.Text()); ok {
			ev.Timestamp = dispatchTS
			r.handler(ev)
		}

		r.mu.Lock()
		r.position = ts
		r.lines++
		r.mu.Unlock()

		if time.Since(lastStatus) >= replayStatusInterval {
			lastStatus = time.Now()
			r.pushStatus()
		}
	}
	if err := scanner.Err(); err != nil {
		slog.Warn("replay: scan error", "err", err)
	}
}

// speedNow reads the session speed under the lock (future-proofing for a
// live speed control; currently set only at Start).
func (r *Replayer) speedNow() float64 {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.speed <= 0 {
		return 1
	}
	return r.speed
}

// sleepInterruptible sleeps for d in small steps, returning false when the
// session is stopped mid-sleep.
func (r *Replayer) sleepInterruptible(d time.Duration, stop <-chan struct{}) bool {
	deadline := time.Now().Add(d)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return true
		}
		step := replayPollStep
		if remaining < step {
			step = remaining
		}
		select {
		case <-stop:
			return false
		case <-time.After(step):
		}
		if !r.waitWhilePaused(stop) {
			return false
		}
	}
}

// waitWhilePaused blocks while the session is paused. Returns false when the
// session is stopped while waiting.
func (r *Replayer) waitWhilePaused(stop <-chan struct{}) bool {
	for {
		r.mu.Lock()
		paused := r.paused
		r.mu.Unlock()
		if !paused {
			return true
		}
		select {
		case <-stop:
			return false
		case <-time.After(replayPollStep):
		}
	}
}

// finish resets the session to idle and fires the end-of-session callbacks.
// Runs exactly once per session (deferred from run).
func (r *Replayer) finish() {
	r.mu.Lock()
	r.state = ReplayIdle
	r.paused = false
	r.stop = nil
	r.mu.Unlock()

	if r.onSession != nil {
		r.onSession(false)
	}
	r.pushStatus()
	slog.Info("replay: session ended")
}
