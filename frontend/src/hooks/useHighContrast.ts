import { useEffect } from 'react'
import { getConfig } from '../services/api'

const POLL_INTERVAL = 3000

// applyContrast toggles the high-contrast theme by setting a data attribute on
// <html>; index.css overrides the muted text/border tokens when it's "high".
// Exported so the Settings toggle can apply it instantly (the hook below keeps
// it authoritative).
export function applyContrast(high: boolean): void {
  document.documentElement.setAttribute('data-contrast', high ? 'high' : 'normal')
}

// useHighContrast keeps the document's contrast mode in sync with the
// high_contrast preference. Polls like the other config-driven hooks
// (useOverlayOpacity, useMasterVolume) so a change saved in Settings takes
// effect without a reload.
export function useHighContrast(): void {
  useEffect(() => {
    let cancelled = false
    const fetch = (): void => {
      getConfig()
        .then((c) => {
          if (!cancelled) applyContrast(Boolean(c.preferences.high_contrast))
        })
        .catch(() => {})
    }
    fetch()
    const id = setInterval(fetch, POLL_INTERVAL)
    return () => {
      cancelled = true
      clearInterval(id)
    }
  }, [])
}
