import { useCallback, useEffect } from 'react'

/**
 * Enables click-through on the overlay window body while keeping the drag
 * header interactive. Call enableInteraction on header mouseenter and
 * enableClickThrough on header mouseleave / window mouseleave.
 *
 * Relies on Electron's setIgnoreMouseEvents with { forward: true } so that
 * mouse-move events are still forwarded to the renderer even in pass-through
 * mode, allowing mouseenter/mouseleave to fire on the drag region.
 */
export function useOverlayClickThrough() {
  useEffect(() => {
    window.electron?.overlay?.setIgnoreMouseEvents(true)
  }, [])

  const enableInteraction = useCallback(() => {
    window.electron?.overlay?.setIgnoreMouseEvents(false)
  }, [])

  const enableClickThrough = useCallback(() => {
    window.electron?.overlay?.setIgnoreMouseEvents(true)
  }, [])

  return { enableInteraction, enableClickThrough }
}
