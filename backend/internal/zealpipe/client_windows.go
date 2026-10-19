//go:build windows

package zealpipe

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
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
	// Zeal creates its pipe with PIPE_ACCESS_OUTBOUND (server writes, client
	// reads) — see Zeal/named_pipe.cpp:486. winio.DialPipe / DialPipeContext
	// default to GENERIC_READ|GENERIC_WRITE, which the kernel rejects on an
	// outbound-only pipe with ERROR_ACCESS_DENIED. Request read-only access
	// explicitly so the dial succeeds.
	conn, err := winio.DialPipeAccess(dialCtx, pipeName, windows.GENERIC_READ)
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
		// Wine doesn't implement directory listing on the pipe namespace and
		// fails with ERROR_NOT_SUPPORTED. Fall back to deriving the pipe names
		// from running eqgame.exe PIDs; the dial confirms the pipe exists.
		if refs, perr := discoverByProcess(); perr == nil {
			return refs, nil
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

// discoverByProcess returns a candidate pipe for every running eqgame.exe,
// named the way Zeal names its pipe (prefix + PID).
func discoverByProcess() ([]PipeRef, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, fmt.Errorf("zealpipe: process snapshot: %w", err)
	}
	defer windows.CloseHandle(snap)

	var entry windows.ProcessEntry32
	entry.Size = uint32(unsafe.Sizeof(entry))
	var refs []PipeRef
	for err = windows.Process32First(snap, &entry); err == nil; err = windows.Process32Next(snap, &entry) {
		if strings.EqualFold(windows.UTF16ToString(entry.ExeFile[:]), "eqgame.exe") {
			refs = append(refs, PipeRef{
				Name: PipeNamePrefix + strconv.FormatUint(uint64(entry.ProcessID), 10),
				PID:  entry.ProcessID,
			})
		}
	}
	if err != windows.ERROR_NO_MORE_FILES {
		return nil, fmt.Errorf("zealpipe: process walk: %w", err)
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
