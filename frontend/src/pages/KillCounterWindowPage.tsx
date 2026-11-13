/**
 * KillCounterWindowPage — transparent always-on-top overlay showing the
 * enabled kill counters (e.g. the Bertoxxulous event preset). Renders in a
 * dedicated frameless Electron window. The +/− buttons only render while the
 * rows are interactive, so a locked overlay stays a clean HUD in game.
 */
import React from 'react'
import { Crosshair } from 'lucide-react'
import KillCounterView from '../components/overlays/KillCounterView'
import { useKillCounters } from '../hooks/useKillCounters'
import { useOverlayOpacity } from '../hooks/useOverlayOpacity'
import { CollapseToggle, useOverlayCollapse } from '../hooks/useOverlayCollapse'
import { useOverlayChromeFade } from '../hooks/useOverlayChromeFade'
import { useOverlayLock } from '../hooks/useOverlayLock'
import { useWindowDrag } from '../hooks/useWindowDrag'
import OverlayLockButton from '../components/OverlayLockButton'

export default function KillCounterWindowPage(): React.ReactElement {
  const opacity = useOverlayOpacity('killCounter')
  const { locked, mode, toggleLocked, rootInteractionProps, headerInteractionProps, rowsInteractive } =
    useOverlayLock('killCounter')
  const chromeFade = useOverlayChromeFade('killCounter', mode === 'display-only')
  // Collapse to title bar. A collapsed window is only its title bar, so keep
  // that visible through the inactive fade.
  const { collapsed, toggleCollapsed, headerRef } = useOverlayCollapse(mode !== 'display-only')
  const chrome = chromeFade || collapsed
  const onDragMouseDown = useWindowDrag()
  const k = useKillCounters()

  // "Bertox 42/60" for the first enabled counter.
  const collapsedSummary = (() => {
    const c = k.counters.find((x) => x.def.enabled)
    if (!c) return null
    return c.def.goal ? `${c.def.name} ${c.count}/${c.def.goal}` : `${c.def.name} ${c.count}`
  })()

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

  return (
    <div
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
        <div style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
          <Crosshair size={11} style={{ color: '#f59e0b' }} />
          <span style={{ fontSize: 11, fontWeight: 700, color: 'rgba(255,255,255,0.8)' }}>Kill Counters</span>
        </div>
        <div className="no-drag" style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
          <CollapseToggle collapsed={collapsed} onToggle={toggleCollapsed} summary={collapsedSummary} />
          <OverlayLockButton locked={locked} onToggle={toggleLocked} />
          <button
            onClick={() => window.electron?.overlay?.closeKillCounter()}
            style={{ ...headerBtn, fontSize: 11 }}
            title="Close overlay"
          >
            ×
          </button>
        </div>
      </div>

      <KillCounterView
        theme="overlay"
        counters={k.counters}
        skewSecs={k.skewSecs}
        onAction={rowsInteractive && !locked ? k.act : undefined}
        hideEmptyState={!chrome}
      />
    </div>
  )
}
