import { useEffect } from 'react'
import { getConfig } from '../services/api'

const POLL_INTERVAL = 3000

// lastApplied gates the initial load so the poll never re-applies the saved
// value over an unsaved preview from Settings (same approach as useHighContrast).
let lastApplied: number | null = null

// applyZoom scales the whole main window via Electron's zoom (like a browser
// Ctrl+/Ctrl-). 0/invalid is treated as 1.0 (100%). Exported so the Settings
// slider can preview instantly and revert.
export function applyZoom(factor: number): void {
  const f = factor && factor > 0 ? factor : 1
  lastApplied = f
  void window.electron?.window?.setZoom?.(f)
}

// useZoom applies the saved zoom_factor on first successful config load, then
// stops polling — after that the Settings page owns live changes (preview on
// change, revert on cancel/leave), so nothing clobbers an unsaved preview.
export function useZoom(): void {
  useEffect(() => {
    let cancelled = false
    let id: ReturnType<typeof setInterval> | null = null
    const stop = (): void => {
      if (id) {
        clearInterval(id)
        id = null
      }
    }
    const tryApply = (): void => {
      getConfig()
        .then((c) => {
          if (cancelled) return
          if (lastApplied === null) applyZoom(c.preferences.zoom_factor)
          stop()
        })
        .catch(() => {})
    }
    tryApply()
    id = setInterval(tryApply, POLL_INTERVAL)
    return () => {
      cancelled = true
      stop()
    }
  }, [])
}
