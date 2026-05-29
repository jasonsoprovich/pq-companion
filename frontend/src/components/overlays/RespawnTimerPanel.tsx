import React, { useCallback, useEffect, useState } from 'react'
import {
  Hourglass, Skull, ExternalLink, Trash2, Circle,
  CheckCircle2, AlertTriangle,
} from 'lucide-react'
import { useWebSocket } from '../../hooks/useWebSocket'
import { WSEvent } from '../../lib/wsEvents'
import { clearRespawns, getLogStatus, getRespawnState, removeRespawn } from '../../services/api'
import OverlayWindow from '../OverlayWindow'
import type { RespawnState, RespawnTimer } from '../../types/respawn'
import type { LogTailerStatus } from '../../types/logEvent'
import { RespawnRow } from './respawnShared'

interface RespawnTimerPanelProps {
  defaultX?: number
  defaultY?: number
  defaultWidth?: number
  defaultHeight?: number
  snapGridSize?: number
  onLayoutChange?: (b: { x: number; y: number; width: number; height: number }) => void
}

function StatusBar({ status }: { status: LogTailerStatus | null }): React.ReactElement {
  const style: React.CSSProperties = {
    display: 'flex', alignItems: 'center', gap: 6,
    padding: '6px 10px', fontSize: 11,
    borderBottom: '1px solid var(--color-border)', flexShrink: 0,
    backgroundColor: 'var(--color-surface-2)',
  }
  if (!status) return <div style={{ ...style, color: 'var(--color-muted)' }}><Circle size={10} /> Loading…</div>
  if (!status.enabled) return <div style={{ ...style, color: '#f97316' }}><AlertTriangle size={11} /> Log parsing disabled — enable in Settings</div>
  if (!status.file_exists) return <div style={{ ...style, color: '#f97316' }}><AlertTriangle size={11} /> Log file not found</div>
  return <div style={{ ...style, color: '#22c55e' }}><CheckCircle2 size={11} /> Tailing log</div>
}

function ConnPill({ state }: { state: string }): React.ReactElement {
  const color = state === 'open' ? '#22c55e' : state === 'connecting' ? '#f97316' : '#6b7280'
  return (
    <span style={{ display: 'flex', alignItems: 'center', gap: 4, fontSize: 11, color }}>
      <span style={{ width: 7, height: 7, borderRadius: '50%', backgroundColor: color, display: 'inline-block' }} />
      {state === 'open' ? 'Live' : state === 'connecting' ? 'Connecting' : 'Off'}
    </span>
  )
}

export default function RespawnTimerPanel({
  defaultX = 24,
  defaultY = 24,
  defaultWidth = 300,
  defaultHeight = 380,
  snapGridSize,
  onLayoutChange,
}: RespawnTimerPanelProps): React.ReactElement {
  const [state, setState] = useState<RespawnState | null>(null)
  const [status, setStatus] = useState<LogTailerStatus | null>(null)

  useEffect(() => {
    getRespawnState().then(setState).catch(() => {})
    getLogStatus().then(setStatus).catch(() => {})
  }, [])

  const handleMessage = useCallback((msg: { type: string; data: unknown }) => {
    if (msg.type === WSEvent.OverlayRespawns) setState(msg.data as RespawnState)
  }, [])

  const wsState = useWebSocket(handleMessage)

  const timers: RespawnTimer[] = state?.timers ?? []
  const currentZone = state?.current_zone ?? ''

  return (
    <OverlayWindow
      title={
        <span style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
          <Hourglass size={13} style={{ color: '#a855f7' }} />
          Respawns
        </span>
      }
      headerRight={
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <button
            onClick={() => clearRespawns().catch(() => {})}
            title="Clear all respawn timers"
            style={{ background: 'none', border: 'none', cursor: 'pointer', padding: '1px 3px', color: 'var(--color-muted)', display: 'flex', alignItems: 'center' }}
          >
            <Trash2 size={12} />
          </button>
          {window.electron?.overlay && (
            <button
              onClick={() => window.electron.overlay.toggleRespawnTimer()}
              title="Pop out as floating overlay"
              style={{ background: 'none', border: 'none', cursor: 'pointer', padding: '1px 3px', color: 'var(--color-muted)', display: 'flex', alignItems: 'center' }}
            >
              <ExternalLink size={12} />
            </button>
          )}
          <ConnPill state={wsState} />
        </div>
      }
      defaultWidth={defaultWidth}
      defaultHeight={defaultHeight}
      defaultX={defaultX}
      defaultY={defaultY}
      minWidth={220}
      minHeight={160}
      snapGridSize={snapGridSize}
      onLayoutChange={onLayoutChange}
    >
      <StatusBar status={status} />
      <div style={{ flex: 1, minHeight: 0, overflow: 'auto', display: 'flex', flexDirection: 'column' }}>
        {state === null ? (
          <div style={{ flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', gap: 8, color: 'var(--color-muted)', padding: 16 }}>
            <Skull size={28} style={{ opacity: 0.2 }} />
            <p style={{ fontSize: 12, margin: 0 }}>Loading…</p>
          </div>
        ) : timers.length === 0 ? (
          <div style={{ flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', gap: 8, color: 'var(--color-muted)', padding: 16 }}>
            <Skull size={28} style={{ opacity: 0.2 }} />
            <p style={{ fontSize: 12, margin: 0 }}>No respawn timers</p>
            <p style={{ fontSize: 11, margin: 0, opacity: 0.7, textAlign: 'center' }}>
              Kill a mob to start tracking its respawn.
            </p>
          </div>
        ) : (
          timers.map((t) => (
            <RespawnRow key={t.id} timer={t} currentZone={currentZone} variant="panel" />
          ))
        )}
      </div>
    </OverlayWindow>
  )
}
