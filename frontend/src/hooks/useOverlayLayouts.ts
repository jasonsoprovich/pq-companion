/**
 * useOverlayLayouts — the user's overlay layouts (named sets of popout
 * windows, owned by the Electron main process) and the actions on them.
 * Follows 'layouts:changed', which main sends whenever the active layout
 * saves itself or a layout is switched, added, renamed or deleted.
 */
import { useCallback, useEffect, useState } from 'react'
import type { LayoutState } from '../types/electron'

export function useOverlayLayouts(): {
  state: LayoutState | null
  supported: boolean
  switchTo: (id: string) => void
  create: (name: string) => void
  rename: (id: string, name: string) => void
  remove: (id: string) => void
} {
  const o = window.electron?.overlay
  const supported = typeof o?.layoutsGet === 'function'
  const [state, setState] = useState<LayoutState | null>(null)

  useEffect(() => {
    if (!supported) return
    let cancelled = false
    window.electron.overlay.layoutsGet()
      .then((s) => { if (!cancelled) setState(s) })
      .catch(() => {})
    const off = window.electron.overlay.onLayoutsChanged((s) => {
      if (!cancelled) setState(s)
    })
    return () => { cancelled = true; off() }
  }, [supported])

  const run = useCallback((p: Promise<LayoutState> | undefined) => {
    p?.then(setState).catch(() => {})
  }, [])

  return {
    state,
    supported,
    switchTo: useCallback((id: string) => run(o?.layoutsSwitch(id)), [o, run]),
    create: useCallback((name: string) => run(o?.layoutsCreate(name)), [o, run]),
    rename: useCallback((id: string, name: string) => run(o?.layoutsRename(id, name)), [o, run]),
    remove: useCallback((id: string) => run(o?.layoutsDelete(id)), [o, run]),
  }
}
