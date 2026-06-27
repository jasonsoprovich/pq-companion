import React, { useCallback, useEffect, useState } from 'react'
import { Gauge, ExternalLink, Trash2 } from 'lucide-react'
import { useWebSocket } from '../../hooks/useWebSocket'
import { useRaidThreatEnabled } from '../../hooks/useRaidThreatEnabled'
import { WSEvent } from '../../lib/wsEvents'
import { getThreatState, getRaidThreatState, resetThreat } from '../../services/api'
import OverlayWindow from '../OverlayWindow'
import type { ThreatState, RaidThreatState } from '../../types/overlay'
import {
  ThreatContent,
  RaidThreatContent,
  ThreatModeToggle,
  type ThreatMode,
} from './threatShared'

interface ThreatPanelProps {
  defaultX?: number
  defaultY?: number
  defaultWidth?: number
  defaultHeight?: number
  snapGridSize?: number
  onLayoutChange?: (b: { x: number; y: number; width: number; height: number }) => void
}

// Mode is persisted so the overlay reopens in whichever view the user last
// chose. Shared key with the popped-out window.
const MODE_KEY = 'threatOverlayMode'
function loadMode(): ThreatMode {
  return localStorage.getItem(MODE_KEY) === 'raid' ? 'raid' : 'personal'
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

export default function ThreatPanel({
  defaultX = 24,
  defaultY = 24,
  defaultWidth = 280,
  defaultHeight = 360,
  snapGridSize,
  onLayoutChange,
}: ThreatPanelProps): React.ReactElement {
  const [state, setState] = useState<ThreatState | null>(null)
  const [raidState, setRaidState] = useState<RaidThreatState | null>(null)
  const [mode, setMode] = useState<ThreatMode>(loadMode)
  const raidEnabled = useRaidThreatEnabled()
  // Fall back to personal if raid mode was persisted but later disabled.
  const effMode: ThreatMode = raidEnabled ? mode : 'personal'

  const chooseMode = (m: ThreatMode): void => {
    setMode(m)
    localStorage.setItem(MODE_KEY, m)
  }

  useEffect(() => {
    getThreatState().then(setState).catch(() => {})
  }, [])

  useEffect(() => {
    if (raidEnabled) getRaidThreatState().then(setRaidState).catch(() => {})
  }, [raidEnabled])

  const handleMessage = useCallback((msg: { type: string; data: unknown }) => {
    if (msg.type === WSEvent.OverlayThreat) setState(msg.data as ThreatState)
    else if (msg.type === WSEvent.OverlayRaidThreat) setRaidState(msg.data as RaidThreatState)
  }, [])

  const wsState = useWebSocket(handleMessage)

  return (
    <OverlayWindow
      title={
        <span style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
          <Gauge size={13} style={{ color: '#c9a84c' }} />
          Threat
        </span>
      }
      headerRight={
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          {raidEnabled && <ThreatModeToggle mode={effMode} onChange={chooseMode} />}
          {effMode === 'personal' && (
            <button
              onClick={() => resetThreat().catch(() => {})}
              title="Reset threat"
              style={{ background: 'none', border: 'none', cursor: 'pointer', padding: '1px 3px', color: 'var(--color-muted)', display: 'flex', alignItems: 'center' }}
            >
              <Trash2 size={12} />
            </button>
          )}
          {window.electron?.overlay && (
            <button
              onClick={() => window.electron.overlay.toggleThreat()}
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
      minWidth={200}
      minHeight={150}
      snapGridSize={snapGridSize}
      onLayoutChange={onLayoutChange}
    >
      {effMode === 'raid' ? <RaidThreatContent state={raidState} /> : <ThreatContent state={state} />}
    </OverlayWindow>
  )
}
