import React, { useCallback, useEffect, useState } from 'react'
import { Lock, ExternalLink, Circle } from 'lucide-react'
import { useNavigate } from 'react-router-dom'
import { useWebSocket } from '../../hooks/useWebSocket'
import { useLiveZone } from '../../hooks/useLiveZone'
import { useActivePlayerName } from '../../hooks/useActivePlayerName'
import { getZoneLockouts } from '../../services/api'
import OverlayWindow from '../OverlayWindow'
import type { ZoneLockoutBoss } from '../../types/lockouts'
import { deriveZoneLockoutRow, sortZoneLockoutRows, ZoneLockoutRow } from './zoneLockoutsShared'

interface ZoneLockoutsPanelProps {
  defaultX?: number
  defaultY?: number
  defaultWidth?: number
  defaultHeight?: number
  snapGridSize?: number
  onLayoutChange?: (b: { x: number; y: number; width: number; height: number }) => void
}

export default function ZoneLockoutsPanel({
  defaultX = 24,
  defaultY = 24,
  defaultWidth = 300,
  defaultHeight = 320,
  snapGridSize,
  onLayoutChange,
}: ZoneLockoutsPanelProps): React.ReactElement {
  const navigate = useNavigate()
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
    <OverlayWindow
      title={
        <span style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
          <Lock size={13} style={{ color: '#a855f7' }} />
          Zone Lockouts
        </span>
      }
      headerRight={
        window.electron?.overlay && (
          <button
            onClick={() => window.electron.overlay.toggleZoneLockouts()}
            title="Pop out as floating overlay"
            style={{ background: 'none', border: 'none', cursor: 'pointer', padding: '1px 3px', color: 'var(--color-muted)', display: 'flex', alignItems: 'center' }}
          >
            <ExternalLink size={12} />
          </button>
        )
      }
      defaultWidth={defaultWidth}
      defaultHeight={defaultHeight}
      defaultX={defaultX}
      defaultY={defaultY}
      minWidth={220}
      minHeight={140}
      snapGridSize={snapGridSize}
      onLayoutChange={onLayoutChange}
    >
      <div style={{ display: 'flex', alignItems: 'center', gap: 6, padding: '6px 10px', fontSize: 11, borderBottom: '1px solid var(--color-border)', flexShrink: 0, backgroundColor: 'var(--color-surface-2)', color: 'var(--color-muted)' }}>
        <Circle size={10} style={{ color: zone ? '#22c55e' : '#6b7280' }} />
        {zone ? zone : 'Zone unknown — connect Zeal to detect it'}
      </div>
      <div style={{ flex: 1, minHeight: 0, overflow: 'auto', display: 'flex', flexDirection: 'column' }}>
        {!zone ? (
          <div style={{ flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', gap: 8, color: 'var(--color-muted)', padding: 16 }}>
            <Lock size={28} style={{ opacity: 0.2 }} />
            <p style={{ fontSize: 12, margin: 0, textAlign: 'center' }}>Waiting for zone…</p>
          </div>
        ) : bosses === null ? (
          <div style={{ flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', gap: 8, color: 'var(--color-muted)', padding: 16 }}>
            <p style={{ fontSize: 12, margin: 0 }}>Loading…</p>
          </div>
        ) : rows.length === 0 ? (
          <div style={{ flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', gap: 8, color: 'var(--color-muted)', padding: 16 }}>
            <Lock size={28} style={{ opacity: 0.2 }} />
            <p style={{ fontSize: 12, margin: 0 }}>No raid targets tracked in this zone.</p>
          </div>
        ) : (
          rows.map((row) => (
            <ZoneLockoutRow
              key={row.targetName}
              row={row}
              variant="panel"
              onNavigate={(npcId) => navigate(`/npcs?select=${npcId}`)}
            />
          ))
        )}
      </div>
    </OverlayWindow>
  )
}
