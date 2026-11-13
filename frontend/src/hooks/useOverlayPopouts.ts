/**
 * useOverlayPopouts — shared popout visibility state and the open/close-all
 * toggle used by the sidebar quick-toggle button and the Overlays hub's
 * ⋯ menu.
 *
 * The toggle shows overlays hidden by the show/hide-all hotkey if there are
 * any, otherwise closes the open ones (they stay in the layout), otherwise —
 * nothing open — opens the active overlay layout. The hotkey itself hides
 * rather than closes; see toggleAllPopoutsVisibility in main.
 */
import { useCallback, useEffect, useState } from 'react'

const POLL_MS = 1500

export interface PopoutVisibility {
  open: number
  hidden: number
}

export function useOverlayPopouts(): {
  visibility: PopoutVisibility
  anyPopoutOpen: boolean
  anyHidden: boolean
  supported: boolean
  toggle: () => void
  closeAll: () => void
  refresh: () => void
} {
  const [visibility, setVisibility] = useState<PopoutVisibility>({ open: 0, hidden: 0 })
  const supported = typeof window.electron?.overlay?.togglePopoutsOpen === 'function'

  const refresh = useCallback(() => {
    window.electron?.overlay?.popoutVisibility?.().then(setVisibility).catch(() => {})
  }, [])

  useEffect(() => {
    if (!supported) return
    let cancelled = false
    const check = (): void => {
      window.electron.overlay
        .popoutVisibility()
        .then((v) => { if (!cancelled) setVisibility(v) })
        .catch(() => {})
    }
    check()
    const id = setInterval(check, POLL_MS)
    return () => { cancelled = true; clearInterval(id) }
  }, [supported])

  const toggle = useCallback(() => {
    window.electron?.overlay
      ?.togglePopoutsOpen()
      // Re-read shortly after: an open-selection round trip creates windows
      // asynchronously.
      .then(() => { refresh(); setTimeout(refresh, 400) })
      .catch(() => {})
  }, [refresh])

  const closeAll = useCallback(() => {
    window.electron?.overlay?.closeAllPopouts().then(refresh).catch(() => {})
  }, [refresh])

  return {
    visibility,
    anyPopoutOpen: visibility.open > 0,
    anyHidden: visibility.hidden > 0,
    supported,
    toggle,
    closeAll,
    refresh,
  }
}
