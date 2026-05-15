package zealpipe

import (
	"context"
	"runtime"
	"testing"
	"time"
)

func TestNewSupervisorPlatformState(t *testing.T) {
	s := NewSupervisor(nil)
	got := s.Status().State
	if runtime.GOOS == "windows" {
		if got != StateIdle {
			t.Errorf("windows initial state = %q, want idle", got)
		}
	} else {
		if got != StateUnsupported {
			t.Errorf("non-windows initial state = %q, want unsupported", got)
		}
	}
}

func TestSupervisorStartCancellableOnNonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("non-windows path under test")
	}
	s := NewSupervisor(nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		s.Start(ctx)
		close(done)
	}()
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Start did not exit after cancel")
	}
}

func TestNextBackoff(t *testing.T) {
	got := nextBackoff(backoffInitial)
	if got != 2*time.Second {
		t.Errorf("first doubling = %v, want 2s", got)
	}
	for i := 0; i < 20; i++ {
		got = nextBackoff(got)
	}
	if got != backoffMax {
		t.Errorf("after many doublings = %v, want capped at %v", got, backoffMax)
	}
}
