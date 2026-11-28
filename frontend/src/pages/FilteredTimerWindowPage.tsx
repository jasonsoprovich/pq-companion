/**
 * FilteredTimerWindowPage — transparent always-on-top overlay showing a
 * user-filtered slice of the spell timers (by spell, player or mob), next to
 * the unfiltered Buff/Detrimental overlays. Renders in a dedicated frameless
 * Electron window. The filter bar hides while the overlay is locked so it
 * stays a clean HUD in game.
 */
import React, { useRef } from 'react'
import { Filter, SlidersHorizontal } from 'lucide-react'
import FilteredTimerView, { filterSummary } from '../components/overlays/FilteredTimerView'
import { useFilteredTimers } from '../hooks/useFilteredTimers'
import { useTimerAppearance } from '../hooks/useTimerAppearance'
import { useOverlayOpacity } from '../hooks/useOverlayOpacity'
import { CollapseToggle, useOverlayCollapse } from '../hooks/useOverlayCollapse'
import { useIdleCollapse } from '../hooks/useIdleCollapse'
import { useFitHeight } from '../hooks/useFitHeight'
import { timerSummary } from '../lib/overlaySummaries'
import { useOverlayChromeFade } from '../hooks/useOverlayChromeFade'
import { useOverlayLock } from '../hooks/useOverlayLock'
import { useWindowDrag } from '../hooks/useWindowDrag'
import OverlayLockButton from '../components/OverlayLockButton'

export default function FilteredTimerWindowPage(): React.ReactElement {
  const opacity = useOverlayOpacity('filteredTimer')
  const { locked, mode, toggleLocked, rootInteractionProps, headerInteractionProps, rowsInteractive } =
    useOverlayLock('filteredTimer')
  const chromeFade = useOverlayChromeFade('filteredTimer', mode === 'display-only')
  // Collapse to title bar.
  const { collapsed, toggleCollapsed, setCollapsed, headerRef, ready: collapseReady } =
    useOverlayCollapse(mode !== 'display-only')
  const onDragMouseDown = useWindowDrag()
  const appearance = useTimerAppearance()
  const f = useFilteredTimers()

  const collapsedSummary = timerSummary(f.timers ?? [], (n) => `${n} match`)

  const headerBtn: React.CSSProperties = {
    display: 'flex',
    alignItems: 'center',
    padding: '1px 5px',
    borderRadius: 3,
    border: '1px solid rgba(255,255,255,0.1)',
    backgroundColor: 'transparent',
    color: 'rgba(255,255,255,0.4)',
    cursor: 'pointer',
    lineHeight: 1,
  }

  useIdleCollapse({
    key: 'filteredTimer',
    active: collapsedSummary !== null,
    collapsed,
    ready: collapseReady,
    setCollapsed,
    allowed: mode !== 'display-only',
  })
  // Fit height to rows: the rows' own height, for the window to shrink to.
  const rootRef = useRef<HTMLDivElement>(null)
  const listRef = useRef<HTMLDivElement>(null)
  const rowsRef = useRef<HTMLDivElement>(null)
  useFitHeight({
    key: 'filteredTimer',
    collapsed,
    collapseReady,
    rows: f.groups.length,
    rootRef,
    listRef,
    contentRef: rowsRef,
  })
  // A collapsed window is only its title bar: keep it through the inactive
  // fade while there's something to show, but let an idle one fade too.
  const chrome = chromeFade || (collapsed && collapsedSummary !== null)

  return (
    <div
      ref={rootRef}
      {...rootInteractionProps}
      style={{
        width: '100%',
        height: '100%',
        backgroundColor: `rgba(10,10,12,${chrome ? opacity : 0})`,
        border: `1px solid rgba(255,255,255,${chrome ? 0.12 : 0})`,
        transition: 'background-color 0.4s ease, border-color 0.4s ease',
        borderRadius: 8,
        display: 'flex',
        flexDirection: 'column',
        overflow: 'hidden',
        fontFamily: 'system-ui, -apple-system, sans-serif',
        color: 'rgba(255,255,255,0.9)',
      }}
    >
      {/* ── Drag handle / title bar ─────────────────────────────────────── */}
      <div
        ref={headerRef}
        {...headerInteractionProps}
        onMouseDown={onDragMouseDown}
        className={`overlay-header ${locked ? 'no-drag' : 'drag-region'}`}
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          padding: '5px 8px',
          borderBottom: '1px solid rgba(255,255,255,0.1)',
          backgroundColor: 'rgba(255,255,255,0.04)',
          flexShrink: 0,
          userSelect: 'none',
          opacity: chrome ? 1 : 0,
          pointerEvents: chrome ? 'auto' : 'none',
          transition: 'opacity 0.4s ease',
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: 5, minWidth: 0 }}>
          <Filter size={11} style={{ color: '#38bdf8', flexShrink: 0 }} />
          <span
            style={{
              fontSize: 11,
              fontWeight: 700,
              color: 'rgba(255,255,255,0.8)',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
            }}
          >
            {filterSummary(f.config)}
          </span>
        </div>
        <div className="no-drag" style={{ display: 'flex', alignItems: 'center', gap: 6, flexShrink: 0 }}>
          <CollapseToggle collapsed={collapsed} onToggle={toggleCollapsed} summary={collapsedSummary} />
          {!locked && (
            <button
              onClick={f.toggleCollapsed}
              title={f.collapsed ? 'Show filters' : 'Hide filters'}
              style={{ ...headerBtn, color: f.collapsed ? 'rgba(255,255,255,0.4)' : 'rgba(255,255,255,0.85)' }}
            >
              <SlidersHorizontal size={11} />
            </button>
          )}
          <OverlayLockButton locked={locked} onToggle={toggleLocked} />
          <button
            onClick={() => window.electron?.overlay?.closeFilteredTimer()}
            style={{ ...headerBtn, fontSize: 11 }}
            title="Close overlay"
          >
            ×
          </button>
        </div>
      </div>

      <FilteredTimerView
        theme="overlay"
        config={f.config}
        update={f.update}
        showFilters={!f.collapsed && !locked && chrome}
        timers={f.timers}
        groups={f.groups}
        activePlayer={f.activePlayer}
        roster={f.roster}
        zealConnected={f.zealConnected}
        appearance={appearance}
        rowsInteractive={rowsInteractive}
        hideEmptyState={!chrome}
        listRef={listRef}
        rowsRef={rowsRef}
      />
    </div>
  )
}
