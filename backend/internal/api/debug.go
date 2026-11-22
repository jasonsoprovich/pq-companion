package api

import (
	"net/http"
	"runtime"

	"github.com/jasonsoprovich/pq-companion/backend/internal/procmem"
)

// memoryResponse is the Go sidecar's half of the Developer tab memory report
// (the Electron half comes from app.getAppMetrics in the main process).
type memoryResponse struct {
	// OS view, comparable with Electron's per-process numbers.
	WorkingSetMB float64  `json:"working_set_mb"`
	PrivateMB    *float64 `json:"private_mb"` // null where the OS can't report it
	PeakRSS      bool     `json:"peak_rss"`   // true when WorkingSetMB is a peak, not current
	// Go runtime view, for telling heap growth from other memory.
	HeapInuseMB  float64 `json:"heap_inuse_mb"`
	HeapSysMB    float64 `json:"heap_sys_mb"`
	SysMB        float64 `json:"sys_mb"`
	NumGoroutine int     `json:"num_goroutine"`
	NumGC        uint32  `json:"num_gc"`
}

func toMB(b uint64) float64 {
	return float64(b*10/(1<<20)) / 10
}

// debugMemory handles GET /api/debug/memory.
func debugMemory(w http.ResponseWriter, _ *http.Request) {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	resp := memoryResponse{
		HeapInuseMB:  toMB(ms.HeapInuse),
		HeapSysMB:    toMB(ms.HeapSys),
		SysMB:        toMB(ms.Sys),
		NumGoroutine: runtime.NumGoroutine(),
		NumGC:        ms.NumGC,
		PeakRSS:      runtime.GOOS != "windows",
	}
	if st, err := procmem.Read(); err == nil {
		resp.WorkingSetMB = toMB(st.WorkingSet)
		if st.HasPrivate {
			p := toMB(st.Private)
			resp.PrivateMB = &p
		}
	}
	writeJSON(w, http.StatusOK, resp)
}
