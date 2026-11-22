// Package procmem reports this process's OS-level memory use, for the
// Developer tab's memory report. Go's runtime stats only cover the Go heap;
// the OS figures are what Task Manager shows, so they're the ones comparable
// with Electron's per-process numbers.
package procmem

// Stats is the OS view of this process's memory, in bytes. Fields the
// platform can't report are zero and Supported says which are real.
type Stats struct {
	// WorkingSet is RAM currently mapped in (Task Manager "Memory"; RSS
	// elsewhere).
	WorkingSet uint64
	// Private is memory only this process holds (Windows PrivateUsage /
	// Task Manager "Commit size"). Zero where unsupported.
	Private uint64
	// HasPrivate reports whether Private was read from the OS.
	HasPrivate bool
}
