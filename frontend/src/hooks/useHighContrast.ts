import { useEffect } from 'react'
import { getConfig } from '../services/api'

const POLL_INTERVAL = 3000

// lastApplied tracks the contrast value currently on the document. It gates the
// initial load below so that, once a value is applied, the polling loop never
// re-applies the saved value and clobbers an unsaved preview from Settings.
let lastApplied: boolean | null = null

// applyContrast toggles the high-contrast theme by setting a data attribute on
// <html>; index.css overrides the muted text/border tokens when it's "high".
// Exported so the Settings toggle can apply it instantly (and revert).
export function applyContrast(high: boolean): void {
  lastApplied = high
  document.documentElement.setAttribute('data-contrast', high ? 'high' : 'normal')
}

// useHighContrast applies the saved high_contrast preference on first load.
// It retries on the poll interval only until the config is first read
// successfully, then stops — after that the Settings page owns live changes
// (preview on toggle, revert on cancel/leave), so there's no background poll to
// clobber an in-progress, not-yet-saved preview.
export function useHighContrast(): void {
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
          if (lastApplied === null) applyContrast(Boolean(c.preferences.high_contrast))
          stop() // config loaded — stop polling regardless of who applied it
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
