//go:build !windows

package mapgen

import "os"

// openShared opens path for reading. Elsewhere an open file never blocks
// deleting or replacing it.
func openShared(path string) (*os.File, error) {
	return os.Open(path)
}
