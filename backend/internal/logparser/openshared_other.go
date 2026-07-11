//go:build !windows

package logparser

import "os"

// openShared opens path for reading. On non-Windows platforms deleting or
// renaming an open file is always allowed, so a plain os.Open behaves the
// same as the Windows FILE_SHARE_DELETE-enabled open.
func openShared(path string) (*os.File, error) {
	return os.Open(path)
}
