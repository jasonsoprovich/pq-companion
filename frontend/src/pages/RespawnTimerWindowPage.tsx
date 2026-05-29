/**
 * RespawnTimerWindowPage — transparent always-on-top overlay showing NPC
 * respawn ("death") timers. Renders in a dedicated frameless Electron window.
 */
import React, { useCallback, useEffect, useState } from 'react'
import { Hourglass, Skull, Trash2 } from 'lucide-react'
import { useWebSocket } from '../hooks/useWebSocket'
import { WSEvent } from '../lib/wsEvents'
import { useOverlayOpacity } from '../hooks/useOverlayOpacity'
import { useOverlayLock } from '../hooks/useOverlayLock'
import { useWindowDrag } from '../hooks/useWindowDrag'
import OverlayLockButton from '../components/OverlayLockButton'
import { clearRespawns, getRespawnState } from '../services/api'
import { RespawnRow } from '../components/overlays/respawnShared'
import type { RespawnState } from '../types/respawn'

export default function RespawnTimerWindowPage(): React.ReactElement {
  const opacity = useOverlayOpacity()
  const { locked, toggleLocked, enableInteraction, enableClickThrough } = useOverlayLock()
  const onDragMouseDown = useWindowDrag()
  const [state, setState] = useState<RespawnState | null>(null)

  useEffect(() => {
    getRespawnState().then(setState).catch(() => {})
  }, [])

  const handleMessage = useCallback((msg: { type: string; data: unknown }) => {
    if (msg.type === WSEvent.OverlayRespawns) {
      setState(msg.data as RespawnState)
    }
  }, [])

  useWebSocket(handleMessage)

  const timers = state?.timers ?? []
  const currentZone = state?.current_zone ?? ''

  return (
    <div
      style={{
        width: '100vw',
        height: '100vh',
        backgroundColor: `rgba(10,10,12,${opacity})`,
        border: '1px solid rgba(255,255,255,0.12)',
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
        onMouseDown={onDragMouseDown}
        className={locked ? 'no-drag' : 'drag-region'}
        onMouseEnter={enableInteraction}
        onMouseLeave={enableClickThrough}
        style={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          padding: '5px 8px',
          borderBottom: '1px solid rgba(255,255,255,0.1)',
          backgroundColor: 'rgba(255,255,255,0.04)',
          flexShrink: 0,
          userSelect: 'none',
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
          <Hourglass size={11} style={{ color: '#a855f7' }} />
          <span style={{ fontSize: 11, fontWeight: 700, color: 'rgba(255,255,255,0.8)' }}>
            Respawns
          </span>
          {timers.length > 0 && (
            <span style={{ fontSize: 10, color: 'rgba(255,255,255,0.35)', marginLeft: 2 }}>
              {timers.length}
            </span>
          )}
        </div>
        <div className="no-drag" style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
          <button
            onClick={() => clearRespawns().catch(() => {})}
            title="Clear all respawn timers"
            style={{
              display: 'flex',
              alignItems: 'center',
              padding: '1px 5px',
              borderRadius: 3,
              border: '1px solid rgba(255,255,255,0.1)',
              backgroundColor: 'transparent',
              color: 'rgba(255,255,255,0.4)',
              cursor: 'pointer',
              lineHeight: 1,
            }}
          >
            <Trash2 size={11} />
          </button>
          <OverlayLockButton locked={locked} onToggle={toggleLocked} />
          <button
            onClick={() => window.electron?.overlay?.closeRespawnTimer()}
            style={{
              fontSize: 11,
              lineHeight: 1,
              padding: '1px 5px',
              borderRadius: 3,
              border: '1px solid rgba(255,255,255,0.1)',
              backgroundColor: 'transparent',
              color: 'rgba(255,255,255,0.4)',
              cursor: 'pointer',
            }}
            title="Close overlay"
          >
            ×
          </button>
        </div>
      </div>

      {/* ── Timer list ───────────────────────────────────────────────────── */}
      <div style={{ flex: 1, overflow: 'auto', display: 'flex', flexDirection: 'column' }}>
        {state === null ? (
          <p style={{ padding: 12, fontSize: 11, color: 'rgba(255,255,255,0.3)', textAlign: 'center', margin: 0 }}>
            Connecting…
          </p>
        ) : timers.length === 0 ? (
          <div
            style={{
              flex: 1,
              display: 'flex',
              flexDirection: 'column',
              alignItems: 'center',
              justifyContent: 'center',
              gap: 6,
              padding: 16,
            }}
          >
            <Skull size={22} style={{ opacity: 0.15 }} />
            <p style={{ fontSize: 11, color: 'rgba(255,255,255,0.25)', margin: 0 }}>
              No respawn timers
            </p>
          </div>
        ) : (
          timers.map((t) => (
            <RespawnRow key={t.id} timer={t} currentZone={currentZone} variant="window" />
          ))
        )}
      </div>
    </div>
  )
}
