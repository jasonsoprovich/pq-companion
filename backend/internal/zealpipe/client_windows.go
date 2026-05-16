//go:build windows

package zealpipe

import (
	"context"
	"fmt"
	"io"
	"time"
	"unsafe"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

// dialTimeout caps the synchronous winio.DialPipeContext call. Beyond this
// window, the supervisor backs off and retries.
const dialTimeout = 2 * time.Second

func dialPlatform(ctx context.Context, pipeName string) (io.ReadCloser, error) {
	if pipeName == "" {
		return nil, fmt.Errorf("zealpipe: empty pipe name")
	}
	dialCtx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()
	conn, err := winio.DialPipeContext(dialCtx, pipeName)
	if err != nil {
		return nil, fmt.Errorf("zealpipe: dial %s: %w", pipeName, err)
	}
	return conn, nil
}

// discoverPlatform enumerates the Windows named-pipe namespace for any pipe
// matching "zeal_*". This works because the pipe namespace is exposed as a
// pseudo-filesystem via FindFirstFile — a documented Windows behaviour since
// at least Windows XP.
func discoverPlatform() ([]PipeRef, error) {
	globPtr, err := windows.UTF16PtrFromString(PipeNameGlob)
	if err != nil {
		return nil, fmt.Errorf("zealpipe: utf16 glob: %w", err)
	}
	var data windows.Win32finddata
	handle, err := windows.FindFirstFile(globPtr, &data)
	if err != nil {
		// An empty pipe namespace match returns ERROR_FILE_NOT_FOUND on a
		// regular filesystem path but ERROR_NO_MORE_FILES inside the named-pipe
		// namespace (\\.\pipe\). Both mean "Zeal isn't currently publishing" —
		// not an error condition.
		if err == windows.ERROR_FILE_NOT_FOUND || err == windows.ERROR_NO_MORE_FILES {
			return nil, nil
		}
		return nil, fmt.Errorf("zealpipe: FindFirstFile: %w", err)
	}
	defer windows.FindClose(handle)

	var refs []PipeRef
	for {
		name := windows.UTF16ToString((*[windows.MAX_PATH]uint16)(unsafe.Pointer(&data.FileName[0]))[:])
		if name != "" {
			refs = append(refs, PipeRef{
				Name: PipeNamePrefix + trimZealPrefix(name),
				PID:  parsePID(name),
			})
		}
		if err := windows.FindNextFile(handle, &data); err != nil {
			if err == windows.ERROR_NO_MORE_FILES {
				break
			}
			return refs, fmt.Errorf("zealpipe: FindNextFile: %w", err)
		}
	}
	return refs, nil
}

// trimZealPrefix strips the "zeal_" prefix from a FindFirstFile result.
// Windows returns the basename without the namespace prefix, but Zeal's
// `name` member already includes "zeal_" — so the match starts with it.
func trimZealPrefix(name string) string {
	const p = "zeal_"
	if len(name) > len(p) && name[:len(p)] == p {
		return name[len(p):]
	}
	return name
}
