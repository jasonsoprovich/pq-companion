import { useCallback, useEffect, useState } from 'react'

/**
 * Manages an overlay popout window's lock state.
 *
 * Unlocked (default): the window is fully interactive — drag the header to
 * move, drag edges to resize, all controls clickable.
 *
 * Locked: the window passes mouse events through to the game underneath via
 * Electron's setIgnoreMouseEvents. Hover anywhere in the window to
 * temporarily disable passthrough (so the user can scroll the timer list,
 * click the per-row X, or grab a button); leave the window and passthrough
 * resumes. forward:true keeps mouseenter/mouseleave flowing to the renderer
 * even while passthrough is on, which is what makes the auto-toggle work.
 *
 * Lock state is persisted per overlay in the main process.
 */
export function useOverlayLock(): {
  locked: boolean
  toggleLocked: () => void
  enableInteraction: () => void
  enableClickThrough: () => void
} {
  const [locked, setLocked] = useState(false)

  useEffect(() => {
    let cancelled = false
    window.electron?.overlay?.getLocked().then((value) => {
      if (!cancelled) setLocked(value)
    }).catch(() => {})
    return () => {
      cancelled = true
    }
  }, [])

  const toggleLocked = useCallback(() => {
    setLocked((prev) => {
      const next = !prev
      window.electron?.overlay?.setLocked(next)
      return next
    })
  }, [])

  // While locked, hovering a no-drag region (e.g. the header buttons) should
  // temporarily disable passthrough so clicks register.
  const enableInteraction = useCallback(() => {
    if (!locked) return
    window.electron?.overlay?.setIgnoreMouseEvents(false)
  }, [locked])

  const enableClickThrough = useCallback(() => {
    if (!locked) return
    window.electron?.overlay?.setIgnoreMouseEvents(true)
  }, [locked])

  return { locked, toggleLocked, enableInteraction, enableClickThrough }
}
