package zealpipe

import (
	"context"
	"io"
	"strconv"
	"strings"
)

// PipeNamePrefix is the literal prefix Zeal uses when creating its named pipe.
// The full pipe path is PipeNamePrefix + <eqgame.exe PID>.
//
// Source: Zeal/named_pipe.h:
//
//	std::string name = "\\\\.\\pipe\\zeal_";
//
// Verified against Zeal HEAD on 2026-05-15.
const PipeNamePrefix = `\\.\pipe\zeal_`

// PipeNameGlob matches every Zeal pipe currently published. Used by the
// Windows discovery layer via FindFirstFileW on the pipe namespace.
const PipeNameGlob = PipeNamePrefix + `*`

// PipeRef identifies a discovered Zeal pipe. PID is the eqgame.exe process
// the pipe is bound to; it's parsed from the pipe name when available.
type PipeRef struct {
	Name string // full path, e.g. "\\.\pipe\zeal_12345"
	PID  uint32 // 0 if the suffix wasn't a valid number
}

// parsePID extracts the trailing PID from a Zeal pipe basename. Returns 0
// when the suffix can't be parsed as a number (rather than erroring — the
// pipe is still dial-able even if we can't attribute it to a process).
func parsePID(name string) uint32 {
	suffix := strings.TrimPrefix(name, PipeNamePrefix)
	// Some Windows callers report the basename only; tolerate both.
	if suffix == name {
		suffix = strings.TrimPrefix(name, "zeal_")
	}
	if n, err := strconv.ParseUint(suffix, 10, 32); err == nil {
		return uint32(n)
	}
	return 0
}

// Discover enumerates currently-listening Zeal pipes. On non-Windows builds
// it always returns nil — the named-pipe namespace doesn't exist outside
// Windows. Errors here mean enumeration *failed*; an empty result is a
// successful "no pipes found".
func Discover() ([]PipeRef, error) {
	return discoverPlatform()
}

// Dial opens a connection to a Zeal pipe. The returned reader is line-safe:
// callers can wrap it with bufio.Scanner. Closing the reader releases the
// pipe handle. On non-Windows builds Dial always returns ErrUnsupported.
func Dial(ctx context.Context, pipeName string) (io.ReadCloser, error) {
	return dialPlatform(ctx, pipeName)
}
