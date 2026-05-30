import { useCallback, useEffect, useState } from 'react'

/**
 * Manages an overlay popout window's lock state.
 *
 * Unlocked (default): the window is fully interactive — drag the header to
 * move, drag edges to resize, all controls clickable.
 *
 * Locked: the window passes mouse events through to the game underneath via
 * Electron's setIgnoreMouseEvents. Hovering anywhere over the overlay
 * temporarily disables passthrough so the whole window — header buttons,
 * the scrollable timer list, and per-row controls — stays interactive while
 * the cursor is over it; moving the cursor off the overlay restores
 * click-through. forward:true keeps mouseenter/mouseleave flowing to the
 * renderer even while passthrough is on, which is what makes the auto-toggle
 * work. (The overlay pages wire enableInteraction/enableClickThrough to the
 * root container's onMouseEnter/onMouseLeave.) Move/resize stay disabled
 * while locked — drag is gated by the no-drag class and resize is turned off
 * in the main process. See issue #127.
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

  // While locked, hovering anywhere over the overlay should temporarily
  // disable passthrough so clicks and scroll register on the whole window.
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
