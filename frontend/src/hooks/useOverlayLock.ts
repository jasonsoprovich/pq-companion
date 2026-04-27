import { useCallback, useEffect, useState } from 'react'

/**
 * Manages an overlay popout window's lock state.
 *
 * Unlocked (default): the window is fully interactive — drag the header to
 * move, drag edges to resize, all controls clickable.
 *
 * Locked: the window passes mouse events through to the game underneath via
 * Electron's setIgnoreMouseEvents. Header buttons remain clickable because
 * forward:true still delivers mouseenter/mouseleave events to the renderer —
 * callers should wire enableInteraction/enableClickThrough to the buttons
 * cluster so hovering temporarily disables passthrough.
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
