//go:build windows

package mapgen

import (
	"os"
	"syscall"
)

// openShared opens path for reading like os.Open, but also grants
// FILE_SHARE_DELETE. A LazyArchive keeps its file open, and os.Open's
// handle would stop the game's patcher replacing that .s3d while PQ
// Companion runs. Same as logparser's openShared.
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
