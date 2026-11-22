//go:build !windows

package procmem

import (
	"runtime"
	"syscall"
)

// Read returns this process's peak RSS as the working set. getrusage has no
// current-RSS field, so this is an upper bound — fine for the dev builds that
// run on macOS/Linux; the numbers that matter come from Windows.
func Read() (Stats, error) {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return Stats{}, err
	}
	rss := uint64(ru.Maxrss)
	if runtime.GOOS != "darwin" {
		rss *= 1024 // Linux reports KB; macOS reports bytes.
	}
	return Stats{WorkingSet: rss}, nil
}
