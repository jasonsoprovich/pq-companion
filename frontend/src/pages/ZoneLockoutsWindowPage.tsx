/**
 * ZoneLockoutsWindowPage — transparent always-on-top overlay showing the
 * active character's loot-lockout status for every raid-target boss in the
 * zone they're currently in. Renders in a dedicated frameless Electron window.
 */
import React, { useCallback, useEffect, useState } from 'react'
import { Lock, Circle } from 'lucide-react'
import { useWebSocket } from '../hooks/useWebSocket'
import { useLiveZone } from '../hooks/useLiveZone'
import { useActivePlayerName } from '../hooks/useActivePlayerName'
import { useOverlayOpacity } from '../hooks/useOverlayOpacity'
import { useOverlayChromeFade } from '../hooks/useOverlayChromeFade'
import { useOverlayLock } from '../hooks/useOverlayLock'
import { useWindowDrag } from '../hooks/useWindowDrag'
import OverlayLockButton from '../components/OverlayLockButton'
import { getZoneLockouts } from '../services/api'
import { openEntityInMain } from '../lib/overlayNav'
import type { ZoneLockoutBoss } from '../types/lockouts'
import { deriveZoneLockoutRow, sortZoneLockoutRows, ZoneLockoutRow } from '../components/overlays/zoneLockoutsShared'

export default function ZoneLockoutsWindowPage(): React.ReactElement {
  const opacity = useOverlayOpacity()
  const { locked, mode, toggleLocked, rootInteractionProps, headerInteractionProps } =
    useOverlayLock('zoneLockouts')
  const chrome = useOverlayChromeFade(mode === 'display-only')
  const onDragMouseDown = useWindowDrag()
  const { zone } = useLiveZone()
  const activePlayer = useActivePlayerName()
  const [bosses, setBosses] = useState<ZoneLockoutBoss[] | null>(null)
  const [nowMs, setNowMs] = useState(() => Date.now())

  const load = useCallback((z: string) => {
    getZoneLockouts(z).then((res) => setBosses(res.bosses ?? [])).catch(() => setBosses([]))
  }, [])

  useEffect(() => {
    if (!zone) {
      setBosses(null)
      return
    }
    load(zone)
  }, [zone, load])

  const handleMessage = useCallback((msg: { type: string; data: unknown }) => {
    if (msg.type !== 'lockouts.snapshot' || !zone) return
    load(zone)
  }, [zone, load])
  useWebSocket(handleMessage)

  useEffect(() => {
    const id = setInterval(() => setNowMs(Date.now()), 60_000)
    return () => clearInterval(id)
  }, [])

  const rows = bosses && activePlayer
    ? sortZoneLockoutRows(bosses.map((b) => deriveZoneLockoutRow(b, activePlayer, nowMs)))
    : []

  return (
    <div
      {...rootInteractionProps}
      style={{
        width: '100vw',
        height: '100vh',
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
          <Lock size={11} style={{ color: '#a855f7' }} />
          <span style={{ fontSize: 11, fontWeight: 700, color: 'rgba(255,255,255,0.8)' }}>
            Zone Lockouts
          </span>
          {zone && (
            <span style={{ fontSize: 10, color: 'rgba(255,255,255,0.35)', marginLeft: 2 }}>
              {zone}
            </span>
          )}
        </div>
        <div className="no-drag" style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
          <OverlayLockButton locked={locked} onToggle={toggleLocked} />
          <button
            onClick={() => window.electron?.overlay?.closeZoneLockouts()}
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

      {/* ── Boss list ────────────────────────────────────────────────────── */}
      <div style={{ flex: 1, overflow: 'auto', display: 'flex', flexDirection: 'column' }}>
        {!zone ? (
          <div
            style={{
              flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center',
              justifyContent: 'center', gap: 6, padding: 16,
              opacity: chrome ? 1 : 0, transition: 'opacity 0.4s ease',
            }}
          >
            <Circle size={22} style={{ opacity: 0.15 }} />
            <p style={{ fontSize: 11, color: 'rgba(255,255,255,0.25)', margin: 0, textAlign: 'center' }}>
              Waiting for zone…
            </p>
          </div>
        ) : bosses === null ? (
          <p style={{ padding: 12, fontSize: 11, color: 'rgba(255,255,255,0.3)', textAlign: 'center', margin: 0 }}>
            Loading…
          </p>
        ) : rows.length === 0 ? (
          <div
            style={{
              flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center',
              justifyContent: 'center', gap: 6, padding: 16,
              opacity: chrome ? 1 : 0, transition: 'opacity 0.4s ease',
            }}
          >
            <Lock size={22} style={{ opacity: 0.15 }} />
            <p style={{ fontSize: 11, color: 'rgba(255,255,255,0.25)', margin: 0, textAlign: 'center' }}>
              No raid targets tracked here
            </p>
          </div>
        ) : (
          rows.map((row) => (
            <ZoneLockoutRow
              key={row.targetName}
              row={row}
              variant="window"
              onNavigate={(npcId) => openEntityInMain('npc', npcId)}
            />
          ))
        )}
      </div>
    </div>
  )
}
