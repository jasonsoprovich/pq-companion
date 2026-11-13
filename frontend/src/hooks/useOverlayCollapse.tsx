/**
 * useOverlayCollapse — folds a popout overlay window down to its title bar.
 *
 * The page attaches `headerRef` to its title bar and renders
 * <CollapseToggle> among its header controls. Toggling measures the header
 * and asks the main process to resize the window to it (top edge stays put;
 * the main process persists the state and the expanded height). The body
 * isn't unmounted — the shrunken window simply clips it — so timers and data
 * keep running and the window springs back instantly.
 *
 * `setCollapsed` sets it directly, for pages that fold themselves when they
 * have nothing to show.
 *
 * `enabled` is false in display-only lock mode, where the title bar itself is
 * hidden: a collapsed display-only window would show nothing at all, so a
 * stored collapse is undone instead.
 */
import React, { useCallback, useEffect, useRef, useState } from 'react'
import { ChevronDown, ChevronRight } from 'lucide-react'

export function useOverlayCollapse(enabled = true): {
  collapsed: boolean
  toggleCollapsed: () => void
  setCollapsed: (collapse: boolean) => void
  headerRef: React.RefObject<HTMLDivElement>
} {
  const headerRef = useRef<HTMLDivElement>(null)
  const [stored, setStored] = useState(false)

  useEffect(() => {
    let cancelled = false
    window.electron?.overlay?.collapseGet?.()
      .then((v) => { if (!cancelled) setStored(!!v) })
      .catch(() => {})
    const off = window.electron?.overlay?.onCollapseChanged?.((v) => {
      if (!cancelled) setStored(!!v)
    })
    return () => { cancelled = true; off?.() }
  }, [])

  const request = useCallback((collapse: boolean) => {
    const header = headerRef.current
    // +2 for the root's 1px top and bottom border around the header.
    const headerPx = header ? header.getBoundingClientRect().height + 2 : 0
    const fraction = window.innerHeight > 0 ? headerPx / window.innerHeight : 0
    window.electron?.overlay?.collapseSet?.(collapse, fraction)
      .then((v) => setStored(!!v))
      .catch(() => {})
  }, [])

  useEffect(() => {
    if (!enabled && stored) request(false)
  }, [enabled, stored, request])

  const collapsed = stored && enabled
  const toggleCollapsed = useCallback(() => request(!collapsed), [collapsed, request])

  return { collapsed, toggleCollapsed, setCollapsed: request, headerRef }
}

/**
 * CollapseToggle — the header chevron, plus the one-line summary shown while
 * collapsed. Goes first in the header's controls group.
 */
export function CollapseToggle({
  collapsed,
  onToggle,
  summary,
}: {
  collapsed: boolean
  onToggle: () => void
  summary?: string | null
}): React.ReactElement {
  return (
    <>
      {collapsed && summary && (
        <span
          style={{
            fontSize: 10,
            fontFamily: 'ui-monospace, Consolas, monospace',
            color: '#c9a84c',
            whiteSpace: 'nowrap',
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            minWidth: 0,
            maxWidth: 220,
          }}
          title={summary}
        >
          {summary}
        </span>
      )}
      <button
        onClick={onToggle}
        title={collapsed ? 'Expand' : 'Collapse to title bar'}
        aria-label={collapsed ? 'Expand overlay' : 'Collapse overlay to title bar'}
        style={{
          display: 'flex',
          alignItems: 'center',
          background: 'none',
          border: 'none',
          padding: '1px 2px',
          cursor: 'pointer',
          color: collapsed ? '#c9a84c' : 'rgba(255,255,255,0.4)',
          flexShrink: 0,
        }}
      >
        {collapsed ? <ChevronRight size={12} /> : <ChevronDown size={12} />}
      </button>
    </>
  )
}
