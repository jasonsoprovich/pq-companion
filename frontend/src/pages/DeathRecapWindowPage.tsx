/**
 * DeathRecapWindowPage — transparent always-on-top overlay showing the
 * recap of the most recent death: killer, damage and healing taken, CHs and
 * the last 20 s of incoming events. Renders in a dedicated frameless
 * Electron window.
 */
import React, { useEffect, useState } from 'react'
import { Skull } from 'lucide-react'
import DeathRecapView from '../components/overlays/DeathRecapView'
import { useLatestDeath } from '../hooks/useLatestDeath'
import { useOverlayOpacity } from '../hooks/useOverlayOpacity'
import { CollapseToggle, useOverlayCollapse } from '../hooks/useOverlayCollapse'
import { useIdleCollapse } from '../hooks/useIdleCollapse'
import { useOverlayChromeFade } from '../hooks/useOverlayChromeFade'
import { useOverlayLock } from '../hooks/useOverlayLock'
import { useWindowDrag } from '../hooks/useWindowDrag'
import OverlayLockButton from '../components/OverlayLockButton'
import type { DeathRecord } from '../types/combat'

const RECENT_DEATH_MS = 5 * 60 * 1000

function DeathRecapHeader({ death }: { death: DeathRecord }): React.ReactElement {
  const at = new Date(death.timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
  return (
    <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
      <div style={{ fontSize: 10, color: 'rgba(255,255,255,0.55)' }}>
        {at}{death.zone ? ` · ${death.zone}` : ''}
      </div>
      <DeathRecapView death={death} theme="overlay" />
    </div>
  )
}

export default function DeathRecapWindowPage(): React.ReactElement {
  const opacity = useOverlayOpacity('deathRecap')
  const { locked, mode, toggleLocked, rootInteractionProps, headerInteractionProps, rowsInteractive } =
    useOverlayLock('deathRecap')
  const chromeFade = useOverlayChromeFade('deathRecap', mode === 'display-only')
  // Collapse to title bar. A collapsed window is only its title bar, so keep
  // that visible through the inactive fade.
  const { collapsed, toggleCollapsed, setCollapsed, headerRef, ready: collapseReady } =
    useOverlayCollapse(mode !== 'display-only')
  const chrome = chromeFade || collapsed
  const onDragMouseDown = useWindowDrag()
  const death = useLatestDeath()
  // Idle-collapse counts a death as activity for a few minutes.
  const [recentDeath, setRecentDeath] = useState(false)
  useEffect(() => {
    const left = death ? Date.parse(death.timestamp) + RECENT_DEATH_MS - Date.now() : 0
    setRecentDeath(left > 0)
    if (left <= 0) return
    const t = setTimeout(() => setRecentDeath(false), left)
    return () => clearTimeout(t)
  }, [death])

  // Who killed you last, plus the biggest hit in the recap window.
  const collapsedSummary = death
    ? death.recap?.biggest_hit
      ? `${death.slain_by} · max hit ${death.recap.biggest_hit}`
      : death.slain_by
    : null

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
    key: 'deathRecap',
    active: recentDeath,
    collapsed,
    ready: collapseReady,
    setCollapsed,
    allowed: mode !== 'display-only',
  })

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
          <Skull size={11} style={{ color: '#ef4444' }} />
          <span style={{ fontSize: 11, fontWeight: 700, color: 'rgba(255,255,255,0.8)' }}>Death Recap</span>
        </div>
        <div className="no-drag" style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
          <CollapseToggle collapsed={collapsed} onToggle={toggleCollapsed} summary={collapsedSummary} />
          <OverlayLockButton locked={locked} onToggle={toggleLocked} />
          <button
            onClick={() => window.electron?.overlay?.closeDeathRecap()}
            style={{ ...headerBtn, fontSize: 11 }}
            title="Close overlay"
          >
            ×
          </button>
        </div>
      </div>

      <div style={{ flex: 1, overflowY: 'auto', padding: '6px 8px' }}>
        {death ? (
          <DeathRecapHeader death={death} />
        ) : (
          chrome && (
            <div style={{ fontSize: 11, color: 'rgba(255,255,255,0.55)' }}>No deaths yet.</div>
          )
        )}
      </div>
    </div>
  )
}
