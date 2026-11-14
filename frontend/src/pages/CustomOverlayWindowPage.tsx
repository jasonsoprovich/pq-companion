/**
 * CustomOverlayWindowPage — transparent always-on-top popout for one Custom
 * Overlay (route custom-overlay-window?id=<overlayId>&name=<name>). Main opens
 * one window per overlay, keyed `customOverlay:<id>`; inside a stack the page
 * gets the id as its groupId prop instead.
 *
 * Lock mode, opacity and fade are one shared 'customOverlay' setting for every
 * custom overlay (like Custom Timers windows). Reset sits in the title bar
 * and needs a second click to confirm; table rows remove from a right-click
 * menu while the rows are interactive.
 */
import React, { useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { LayoutTemplate, RotateCcw } from 'lucide-react'
import CustomOverlayView from '../components/overlays/CustomOverlayView'
import { useCustomOverlays, useNowMs } from '../hooks/useCustomOverlays'
import { useOverlayOpacity } from '../hooks/useOverlayOpacity'
import { CollapseToggle, useOverlayCollapse } from '../hooks/useOverlayCollapse'
import { useIdleCollapse } from '../hooks/useIdleCollapse'
import { useOverlayChromeFade } from '../hooks/useOverlayChromeFade'
import { useOverlayLock } from '../hooks/useOverlayLock'
import { useWindowDrag } from '../hooks/useWindowDrag'
import OverlayLockButton from '../components/OverlayLockButton'
import {
  customOverlayActive,
  customOverlaySummary,
  customOverlayWindowKey,
} from '../lib/customOverlay'

const RESET_CONFIRM_MS = 3000
// How often the idle check re-evaluates while nothing else re-renders.
const IDLE_TICK_MS = 10_000

export default function CustomOverlayWindowPage({ groupId }: { groupId?: string } = {}): React.ReactElement {
  const [searchParams] = useSearchParams()
  const id = groupId ?? searchParams.get('id') ?? ''
  const urlName = searchParams.get('name') ?? ''

  const opacity = useOverlayOpacity('customOverlay')
  const { locked, mode, toggleLocked, rootInteractionProps, headerInteractionProps, rowsInteractive } =
    useOverlayLock('customOverlay')
  const chromeFade = useOverlayChromeFade('customOverlay', mode === 'display-only')
  // Collapse to title bar. A collapsed window is only its title bar, so keep
  // that visible through the inactive fade.
  const { collapsed, toggleCollapsed, setCollapsed, headerRef, ready: collapseReady } =
    useOverlayCollapse(mode !== 'display-only')
  const chrome = chromeFade || collapsed
  const onDragMouseDown = useWindowDrag()

  const co = useCustomOverlays()
  const overlay = co.overlays.find((o) => o.def.id === id)
  const now = useNowMs(true, co.skewMs, IDLE_TICK_MS)
  const active = overlay ? customOverlayActive(overlay, now) : false
  const summary = overlay ? customOverlaySummary(overlay) : null

  useIdleCollapse({
    key: customOverlayWindowKey(id),
    active,
    collapsed,
    ready: collapseReady && co.loaded,
    setCollapsed,
    allowed: mode !== 'display-only',
  })

  const [resetArmed, setResetArmed] = useState(false)
  useEffect(() => {
    if (!resetArmed) return
    const t = setTimeout(() => setResetArmed(false), RESET_CONFIRM_MS)
    return () => clearTimeout(t)
  }, [resetArmed])
  const onReset = (): void => {
    if (!overlay) return
    if (!resetArmed) {
      setResetArmed(true)
      return
    }
    setResetArmed(false)
    void co.act(overlay.def.id, { action: 'reset' })
  }

  const headerBtn: React.CSSProperties = {
    display: 'flex',
    alignItems: 'center',
    gap: 3,
    padding: '1px 5px',
    borderRadius: 3,
    border: '1px solid rgba(255,255,255,0.1)',
    backgroundColor: 'transparent',
    color: 'rgba(255,255,255,0.4)',
    cursor: 'pointer',
    lineHeight: 1,
    fontSize: 10,
  }

  const canEditRows = rowsInteractive || !locked

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
          gap: 6,
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
        {/* The name keeps up to half the bar; the collapsed summary on the
            right is what truncates. */}
        <div style={{ display: 'flex', alignItems: 'center', gap: 5, minWidth: 0, flexShrink: 0, maxWidth: '50%' }}>
          <LayoutTemplate size={11} style={{ color: '#a78bfa', flexShrink: 0 }} />
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
            {overlay?.def.name || urlName || 'Custom Overlay'}
          </span>
        </div>
        <div className="no-drag" style={{ display: 'flex', alignItems: 'center', gap: 6, minWidth: 0 }}>
          <CollapseToggle collapsed={collapsed} onToggle={toggleCollapsed} summary={summary} />
          {overlay && (
            <button
              onClick={onReset}
              title={resetArmed ? 'Click again to clear everything in this overlay' : 'Reset this overlay'}
              style={{
                ...headerBtn,
                ...(resetArmed
                  ? { color: '#fca5a5', borderColor: 'rgba(248,113,113,0.6)', backgroundColor: 'rgba(248,113,113,0.12)' }
                  : {}),
              }}
            >
              <RotateCcw size={11} />
              {resetArmed && <span>Reset?</span>}
            </button>
          )}
          <OverlayLockButton locked={locked} onToggle={toggleLocked} />
          <button
            onClick={() => window.electron?.overlay?.closeCustomOverlay(id)}
            style={{ ...headerBtn, fontSize: 11 }}
            title="Close overlay"
          >
            ×
          </button>
        </div>
      </div>

      {overlay ? (
        <CustomOverlayView
          theme="overlay"
          overlay={overlay}
          skewMs={co.skewMs}
          onRemoveRow={canEditRows ? (widget, key) => void co.act(overlay.def.id, { action: 'remove_row', widget, key }) : undefined}
          hideEmptyState={!chrome}
        />
      ) : (
        co.loaded && chrome && (
          <div style={{ padding: '14px 10px', fontSize: 11, color: 'rgba(255,255,255,0.4)', textAlign: 'center' }}>
            This custom overlay no longer exists.
          </div>
        )
      )}
    </div>
  )
}
