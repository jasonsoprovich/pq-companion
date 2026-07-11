/**
 * useOverlayPopouts — shared "any overlay popped out?" state and the
 * open-all/close-all toggle, backed by the same overlay:popouts:* IPC calls
 * used by the Overlays Dashboard's "Manage overlays" menu. Lets any part of
 * the app (e.g. the sidebar's quick-toggle button) drive the same pop-out-all
 * behavior without duplicating the polling logic.
 */
import { useCallback, useEffect, useState } from 'react'
import { loadDashboardLayout, loadGroupPanelLayouts, VISIBLE_DASHBOARD_PANEL_KEYS } from '../services/dashboardLayout'
import { listTimerGroups } from '../services/api'
import type { TimerGroup } from '../types/trigger'

const POLL_MS = 1500

export function useOverlayPopouts(): {
  anyPopoutOpen: boolean
  supported: boolean
  toggle: () => void
  refresh: () => void
} {
  const [anyPopoutOpen, setAnyPopoutOpen] = useState(false)
  const supported = typeof window.electron?.overlay?.anyPopoutOpen === 'function'

  const refresh = useCallback(() => {
    window.electron?.overlay?.anyPopoutOpen?.().then(setAnyPopoutOpen).catch(() => {})
  }, [])

  useEffect(() => {
    if (!supported) return
    let cancelled = false
    const check = (): void => {
      window.electron.overlay
        .anyPopoutOpen()
        .then((v) => { if (!cancelled) setAnyPopoutOpen(v) })
        .catch(() => {})
    }
    check()
    const id = setInterval(check, POLL_MS)
    return () => { cancelled = true; clearInterval(id) }
  }, [supported])

  const toggle = useCallback(() => {
    const o = window.electron?.overlay
    if (!o) return
    if (anyPopoutOpen) {
      o.closeAllPopouts().catch(() => {})
      setAnyPopoutOpen(false)
      return
    }
    // Only pop out overlays the user has toggled visible in the dashboard —
    // a panel hidden there shouldn't open as a floating window. Trigger
    // Alerts has no dashboard toggle and is always included by the main
    // process.
    const layout = loadDashboardLayout()
    const panels: Array<string | { key: string; name: string }> =
      VISIBLE_DASHBOARD_PANEL_KEYS.filter((k) => layout[k].visible)
    const visibleGroupIds = Object.entries(loadGroupPanelLayouts())
      .filter(([, l]) => l.visible)
      .map(([id]) => id)
    // Group panels need each group's current display name, which only the
    // backend has — main process can't create the window without it.
    const openWithGroups = (groups: TimerGroup[]): void => {
      for (const id of visibleGroupIds) {
        const g = groups.find((x) => x.id === id)
        if (g) panels.push({ key: `customTimer:${id}`, name: g.name })
      }
      o.openAllPopouts(panels).catch(() => {})
    }
    if (visibleGroupIds.length > 0) {
      listTimerGroups().then(openWithGroups).catch(() => openWithGroups([]))
    } else {
      openWithGroups([])
    }
    setAnyPopoutOpen(true)
  }, [anyPopoutOpen])

  return { anyPopoutOpen, supported, toggle, refresh }
}
