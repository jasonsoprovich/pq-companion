//go:build !windows

package zealpipe

import (
	"context"
	"errors"
	"io"
)

// ErrUnsupported is returned by Dial on non-Windows builds. The named-pipe
// IPC is a Windows-only mechanism — the ship target is Windows-only, but the
// app still needs to compile on macOS for dev work.
var ErrUnsupported = errors.New("zealpipe: not supported on this platform")

func dialPlatform(_ context.Context, _ string) (io.ReadCloser, error) {
	return nil, ErrUnsupported
}

// discoverPlatform always reports an empty pipe list on non-Windows builds.
// Reporting "no pipes" rather than an error keeps the supervisor's polling
// loop quiet on macOS dev machines — there's nothing to retry, the absence
// is expected.
func discoverPlatform() ([]PipeRef, error) {
	return nil, nil
}
