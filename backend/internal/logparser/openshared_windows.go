//go:build windows

package logparser

import (
	"os"
	"syscall"
)

// openShared opens path for reading the way os.Open does, but additionally
// requests FILE_SHARE_DELETE. Go's os.Open on Windows only grants
// FILE_SHARE_READ|FILE_SHARE_WRITE, which lets other processes read/append
// the file concurrently but blocks them from deleting or renaming it for as
// long as our handle stays open — and the live tailer holds its handle for
// an entire play session. That's enough to turn an ordinary "delete the log
// file to start fresh" troubleshooting step (or a rename by another tool)
// into a Windows sharing-violation error that looks like PQ Companion is
// hanging onto the file.
func openShared(path string) (*os.File, error) {
	pathp, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	sharemode := uint32(syscall.FILE_SHARE_READ | syscall.FILE_SHARE_WRITE | syscall.FILE_SHARE_DELETE)
	h, err := syscall.CreateFile(pathp, syscall.GENERIC_READ, sharemode, nil,
		syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: path, Err: err}
	}
	return os.NewFile(uintptr(h), path), nil
}
