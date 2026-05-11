import { useCallback, useEffect, useState } from 'react'

/**
 * Manages an overlay popout window's lock state.
 *
 * Unlocked (default): the window is fully interactive — drag the header to
 * move, drag edges to resize, all controls clickable.
 *
 * Locked: the window passes mouse events through to the game underneath via
 * Electron's setIgnoreMouseEvents. Hovering the header strip temporarily
 * disables passthrough so the header buttons (clear, lock, close, etc.)
 * stay clickable; the body of the overlay remains click-through even when
 * the cursor is over it. forward:true keeps mouseenter/mouseleave flowing
 * to the renderer even while passthrough is on, which is what makes the
 * auto-toggle work.
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
      // setLocked in the main process applies setIgnoreMouseEvents(next) to
      // sync the window state. When locking, the user just clicked the lock
      // button (which lives in the header), so the cursor is over the header
      // — re-assert interactive mode after the round-trip so the header stays
      // clickable until the cursor actually leaves it.
      const p = window.electron?.overlay?.setLocked(next)
      if (next && p) {
        p.then(() => window.electron?.overlay?.setIgnoreMouseEvents(false))
         .catch(() => {})
      }
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
