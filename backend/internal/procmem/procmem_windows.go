//go:build windows

package procmem

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

// processMemoryCountersEx mirrors PROCESS_MEMORY_COUNTERS_EX (psapi.h).
type processMemoryCountersEx struct {
	CB                         uint32
	PageFaultCount             uint32
	PeakWorkingSetSize         uintptr
	WorkingSetSize             uintptr
	QuotaPeakPagedPoolUsage    uintptr
	QuotaPagedPoolUsage        uintptr
	QuotaPeakNonPagedPoolUsage uintptr
	QuotaNonPagedPoolUsage     uintptr
	PagefileUsage              uintptr
	PeakPagefileUsage          uintptr
	PrivateUsage               uintptr
}

var procGetProcessMemoryInfo = windows.NewLazySystemDLL("psapi.dll").NewProc("GetProcessMemoryInfo")

// Read returns this process's working set and private bytes.
func Read() (Stats, error) {
	if err := procGetProcessMemoryInfo.Find(); err != nil {
		return Stats{}, err
	}
	h, err := windows.GetCurrentProcess()
	if err != nil {
		return Stats{}, err
	}
	var c processMemoryCountersEx
	c.CB = uint32(unsafe.Sizeof(c))
	r, _, callErr := procGetProcessMemoryInfo.Call(uintptr(h), uintptr(unsafe.Pointer(&c)), uintptr(c.CB))
	if r == 0 {
		return Stats{}, callErr
	}
	return Stats{
		WorkingSet: uint64(c.WorkingSetSize),
		Private:    uint64(c.PrivateUsage),
		HasPrivate: true,
	}, nil
}
