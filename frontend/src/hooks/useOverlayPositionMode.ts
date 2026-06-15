import { useEffect, useState } from 'react'

/**
 * Tracks the global "Position overlays" mode — a transient, app-wide state
 * (owned by the Electron main process, off on every launch) that temporarily
 * makes every popout overlay fully interactive so they can be dragged into
 * place, regardless of each overlay's locked mode. Turning it off restores
 * each overlay to its configured behaviour.
 *
 * This is the recovery path for "display-only" overlays, which otherwise never
 * capture the mouse (their title bar is hidden, so they can't be grabbed or
 * unlocked directly).
 *
 * Consumed by the shared overlay hooks (useWindowDrag, useOverlayChromeFade,
 * useOverlayLock) and by the Settings toggle. Each overlay window and the main
 * window receive the broadcast, so this stays in sync everywhere live.
 */
export function useOverlayPositionMode(): boolean {
  const [active, setActive] = useState(false)

  useEffect(() => {
    let cancelled = false
    window.electron?.overlay?.getPositionMode?.().then((value) => {
      if (!cancelled) setActive(value)
    }).catch(() => {})
    const off = window.electron?.overlay?.onPositionModeChanged?.((value) => {
      if (!cancelled) setActive(value)
    })
    return () => {
      cancelled = true
      off?.()
    }
  }, [])

  return active
}
