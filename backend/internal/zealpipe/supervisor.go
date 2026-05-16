package zealpipe

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"runtime"
	"sync"
	"time"
)

// State is the supervisor's coarse connection state, exposed to the UI.
type State string

const (
	StateIdle         State = "idle"          // no pipe discovered yet
	StateConnected    State = "connected"     // actively reading from a pipe
	StateDisconnected State = "disconnected"  // previously connected, lost it
	StateUnsupported  State = "unsupported"   // non-Windows build
)

// Status is the snapshot returned by Supervisor.Status(). Safe to JSON-encode.
type Status struct {
	State       State     `json:"state"`
	PipeName    string    `json:"pipe_name,omitempty"`
	PID         uint32    `json:"pid,omitempty"`
	Character   string    `json:"character,omitempty"`
	LastError   string    `json:"last_error,omitempty"`
	ConnectedAt time.Time `json:"connected_at,omitempty"`
}

// EventHandler receives each decoded Envelope. It MUST be fast and
// non-blocking — the supervisor's reader goroutine calls it inline.
type EventHandler func(Envelope)

// Supervisor manages the discover → dial → read → reconnect lifecycle for a
// single Zeal pipe. Multibox support is intentionally deferred: the
// supervisor binds to the first pipe it finds (see plan Stage A risks).
type Supervisor struct {
	onEvent EventHandler

	mu     sync.RWMutex
	status Status
}

// Tuning knobs. Tuned for snappy local IPC — Zeal's default cadence is ~100ms.
const (
	discoverInterval = 2 * time.Second
	backoffInitial   = 1 * time.Second
	backoffMax       = 30 * time.Second
)

// NewSupervisor builds a supervisor. onEvent may be nil during early dev; the
// supervisor still tracks status either way.
func NewSupervisor(onEvent EventHandler) *Supervisor {
	s := &Supervisor{onEvent: onEvent}
	if runtime.GOOS != "windows" {
		s.set(Status{State: StateUnsupported})
	} else {
		s.set(Status{State: StateIdle})
	}
	return s
}

// Status returns a snapshot of the current connection state.
func (s *Supervisor) Status() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}

func (s *Supervisor) set(st Status) {
	s.mu.Lock()
	s.status = st
	s.mu.Unlock()
}

func (s *Supervisor) update(fn func(*Status)) {
	s.mu.Lock()
	fn(&s.status)
	s.mu.Unlock()
}

// Start runs until ctx is cancelled. Safe to call once per Supervisor; calling
// twice concurrently produces undefined behaviour. Blocks; usually invoked
// via `go supervisor.Start(ctx)`.
func (s *Supervisor) Start(ctx context.Context) {
	// macOS dev builds: the stub Discover() always returns nil, so the loop
	// below would just spin uselessly. Short-circuit and park.
	if runtime.GOOS != "windows" {
		<-ctx.Done()
		return
	}

	backoff := backoffInitial
	for {
		if err := ctx.Err(); err != nil {
			return
		}

		ref, err := pickPipe(ctx)
		if err != nil {
			s.update(func(st *Status) {
				st.State = StateDisconnected
				st.LastError = err.Error()
			})
			if sleepCtx(ctx, backoff) {
				return
			}
			backoff = nextBackoff(backoff)
			continue
		}
		if ref == nil {
			// No pipe yet — wait and retry. Reset backoff so the first dial
			// after a Zeal launch is fast. Also clear any stale error so the
			// Settings UI doesn't keep showing a failure from a previous EQ
			// session — "no Zeal running" is an expected idle state, not an
			// error worth surfacing to the user.
			backoff = backoffInitial
			s.update(func(st *Status) {
				st.State = StateIdle
				st.LastError = ""
				st.PipeName = ""
				st.PID = 0
			})
			if sleepCtx(ctx, discoverInterval) {
				return
			}
			continue
		}

		conn, err := Dial(ctx, ref.Name)
		if err != nil {
			// Logged at Info (not Debug) because this is the most useful
			// signal when diagnosing a non-working integration on a user's
			// machine — backoff means it would otherwise scroll by silently.
			slog.Info("zealpipe: dial failed", "pipe", ref.Name, "err", err)
			s.update(func(st *Status) {
				st.State = StateDisconnected
				st.PipeName = ref.Name
				st.PID = ref.PID
				st.LastError = err.Error()
			})
			if sleepCtx(ctx, backoff) {
				return
			}
			backoff = nextBackoff(backoff)
			continue
		}

		slog.Info("zealpipe: connected", "pipe", ref.Name, "pid", ref.PID)
		s.set(Status{
			State:       StateConnected,
			PipeName:    ref.Name,
			PID:         ref.PID,
			ConnectedAt: time.Now(),
		})
		backoff = backoffInitial

		s.readLoop(ctx, conn)
		_ = conn.Close()

		// readLoop only returns on EOF, ctx cancellation, or read error. Mark
		// disconnected and let the outer loop rediscover.
		if ctx.Err() != nil {
			return
		}
		s.update(func(st *Status) {
			st.State = StateDisconnected
			st.ConnectedAt = time.Time{}
		})
		slog.Info("zealpipe: disconnected; will reconnect")
	}
}

// readLoop stream-decodes JSON envelopes from conn until EOF, error, or ctx
// cancellation. Zeal writes envelopes back-to-back into a PIPE_TYPE_BYTE pipe
// with no delimiter or framing (see Zeal/named_pipe.cpp WriteDataWithRetry —
// it just hands data.c_str() + data.length() to WriteFileEx), so we can't
// scan-by-line. json.Decoder reads one balanced JSON value at a time from the
// byte stream, which is exactly the shape we get.
func (s *Supervisor) readLoop(ctx context.Context, conn io.Reader) {
	dec := json.NewDecoder(conn)
	for {
		if ctx.Err() != nil {
			return
		}
		var env Envelope
		if err := dec.Decode(&env); err != nil {
			if errors.Is(err, io.EOF) {
				return
			}
			slog.Info("zealpipe: read ended", "err", err)
			s.update(func(st *Status) { st.LastError = err.Error() })
			return
		}
		if env.Character != "" {
			s.update(func(st *Status) { st.Character = env.Character })
		}
		if s.onEvent != nil {
			s.onEvent(env)
		}
	}
}

// pickPipe returns the first listening Zeal pipe, or nil if none. Multibox is
// out of scope for Stage A — when the user runs multiple clients we just
// attach to whichever Discover() returns first.
func pickPipe(ctx context.Context) (*PipeRef, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	refs, err := Discover()
	if err != nil {
		return nil, err
	}
	if len(refs) == 0 {
		return nil, nil
	}
	r := refs[0]
	return &r, nil
}

// sleepCtx sleeps for d or returns true if ctx fires first.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return true
	case <-t.C:
		return false
	}
}

// nextBackoff doubles up to backoffMax.
func nextBackoff(current time.Duration) time.Duration {
	next := current * 2
	if next > backoffMax {
		return backoffMax
	}
	return next
}
