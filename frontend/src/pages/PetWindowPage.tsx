/**
 * PetWindowPage — transparent always-on-top Pet overlay: the active
 * character's charmed/summoned pet card, the charm-break card, or the compact
 * charm finder when there is no pet. Renders in a dedicated frameless Electron
 * window. The finder's dropdown/filters and the view toggles only render while
 * the overlay is interactive, so a locked overlay stays a clean HUD in game.
 */
import React from 'react'
import { PawPrint } from 'lucide-react'
import PetView from '../components/overlays/PetView'
import { usePetState, usePetTimers } from '../hooks/usePetState'
import { useOverlayOpacity } from '../hooks/useOverlayOpacity'
import { CollapseToggle, useOverlayCollapse } from '../hooks/useOverlayCollapse'
import { useIdleCollapse } from '../hooks/useIdleCollapse'
import { fmtRemaining } from '../lib/timeFormat'
import { useOverlayChromeFade } from '../hooks/useOverlayChromeFade'
import { useOverlayLock } from '../hooks/useOverlayLock'
import { useWindowDrag } from '../hooks/useWindowDrag'
import { AppNavigateContext } from '../hooks/useAppNavigate'
import { openRouteInMain } from '../lib/overlayNav'
import OverlayLockButton from '../components/OverlayLockButton'

export default function PetWindowPage(): React.ReactElement {
  const opacity = useOverlayOpacity('pet')
  const { locked, mode, toggleLocked, rootInteractionProps, headerInteractionProps, rowsInteractive } =
    useOverlayLock('pet')
  const chromeFade = useOverlayChromeFade('pet', mode === 'display-only')
  // Collapse to title bar. A collapsed window is only its title bar, so keep
  // that visible through the inactive fade.
  const { collapsed, toggleCollapsed, setCollapsed, headerRef, ready: collapseReady } =
    useOverlayCollapse(mode !== 'display-only')
  const chrome = chromeFade || collapsed
  const onDragMouseDown = useWindowDrag()
  const state = usePetState()

  // "74% · Charm 1m 12s" — charm time only while charmed.
  const { charm } = usePetTimers(state.name)
  const collapsedSummary = (() => {
    if (!state.name) return null
    const parts: string[] = []
    if (state.hp_percent != null && state.hp_percent >= 0) parts.push(`${Math.round(state.hp_percent)}%`)
    else parts.push(state.name)
    if (charm && !charm.expired) {
      parts.push(`Charm ${fmtRemaining((Date.parse(charm.expires_at) - Date.now()) / 1000, true)}`)
    }
    return parts.join(' · ')
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

  useIdleCollapse({
    key: 'pet',
    active: collapsedSummary !== null,
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
          <PawPrint size={11} style={{ color: '#a855f7' }} />
          <span style={{ fontSize: 11, fontWeight: 700, color: 'rgba(255,255,255,0.8)' }}>Pet</span>
        </div>
        <div className="no-drag" style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
          <CollapseToggle collapsed={collapsed} onToggle={toggleCollapsed} summary={collapsedSummary} />
          <OverlayLockButton locked={locked} onToggle={toggleLocked} />
          <button
            onClick={() => window.electron?.overlay?.closePet()}
            style={{ ...headerBtn, fontSize: 11 }}
            title="Close overlay"
          >
            ×
          </button>
        </div>
      </div>

      <div style={{ flex: 1, minHeight: 0 }}>
        <AppNavigateContext.Provider value={openRouteInMain}>
          <PetView state={state} interactive={rowsInteractive && !locked} />
        </AppNavigateContext.Provider>
      </div>
    </div>
  )
}
