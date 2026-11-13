/**
 * useOverlayPopouts — shared popout visibility state and the hide/show-all
 * toggle used by the sidebar quick-toggle button and the Overlays
 * Dashboard's "Manage overlays" menu (and, via the main process, the global
 * show/hide hotkey).
 *
 * The toggle shows overlays hidden by "hide all" if there are any, otherwise
 * hides the open ones (they stay alive and in place), otherwise — nothing
 * open — opens the user's saved popout set. That last case is decided in the
 * main process, which asks the main window to run openSavedPopoutSelection()
 * because the saved set lives in this renderer's localStorage.
 */
import { useCallback, useEffect, useState } from 'react'
import { loadPopoutSelection } from '../services/overlayPopoutSelection'
import { listTimerGroups } from '../services/api'
import type { TimerGroup } from '../types/trigger'

const POLL_MS = 1500

export interface PopoutVisibility {
  open: number
  hidden: number
}

// Pops out exactly the overlays the user last individually popped out via
// Manage overlays — dashboard panel visibility is a separate concern and
// shouldn't drive which floating windows open. Trigger Alerts has no popout
// toggle of its own and is always included by the main process.
export function openSavedPopoutSelection(): void {
  const o = window.electron?.overlay
  if (!o) return
  const selection = loadPopoutSelection()
  const panels: Array<string | { key: string; name: string }> = [...selection.panels]
  const groupIds = selection.groups
  // Group panels need each group's current display name, which only the
  // backend has — main process can't create the window without it.
  const openWithGroups = (groups: TimerGroup[]): void => {
    for (const id of groupIds) {
      const g = groups.find((x) => x.id === id)
      if (g) panels.push({ key: `customTimer:${id}`, name: g.name })
    }
    o.openAllPopouts(panels).catch(() => {})
  }
  if (groupIds.length > 0) {
    listTimerGroups().then(openWithGroups).catch(() => openWithGroups([]))
  } else {
    openWithGroups([])
  }
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
  const supported = typeof window.electron?.overlay?.togglePopoutVisibility === 'function'

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
      ?.togglePopoutVisibility()
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
