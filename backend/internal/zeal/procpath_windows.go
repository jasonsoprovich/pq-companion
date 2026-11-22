//go:build windows

package zeal

import (
	"path/filepath"

	"golang.org/x/sys/windows"
)

// processExeDir returns the folder of process pid's executable, or "" when it
// can't be read (process gone, access denied).
func processExeDir(pid uint32) string {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return ""
	}
	defer windows.CloseHandle(h)
	buf := make([]uint16, windows.MAX_LONG_PATH)
	n := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &n); err != nil {
		return ""
	}
	return filepath.Dir(windows.UTF16ToString(buf[:n]))
}
