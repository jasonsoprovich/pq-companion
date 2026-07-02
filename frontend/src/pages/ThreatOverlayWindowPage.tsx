/**
 * ThreatOverlayWindowPage — transparent always-on-top overlay showing the
 * active character's ESTIMATED personal hate per mob. Renders in a dedicated
 * frameless Electron window. See internal/threat for the hate model.
 */
import React, { useCallback, useEffect, useRef, useState } from 'react'
import { Gauge, Trash2 } from 'lucide-react'
import { useWebSocket } from '../hooks/useWebSocket'
import { useRaidThreatEnabled } from '../hooks/useRaidThreatEnabled'
import { WSEvent } from '../lib/wsEvents'
import { useOverlayOpacity } from '../hooks/useOverlayOpacity'
import { useOverlayChromeFade } from '../hooks/useOverlayChromeFade'
import { useOverlayLock } from '../hooks/useOverlayLock'
import { useWindowDrag } from '../hooks/useWindowDrag'
import OverlayLockButton from '../components/OverlayLockButton'
import { getThreatState, getRaidThreatState, resetThreat } from '../services/api'
import {
  ThreatContent,
  RaidThreatContent,
  ThreatModeToggle,
  type ThreatMode,
} from '../components/overlays/threatShared'
import type { ThreatState, RaidThreatState } from '../types/overlay'

const MODE_KEY = 'threatOverlayMode'
function loadMode(): ThreatMode {
  return localStorage.getItem(MODE_KEY) === 'raid' ? 'raid' : 'personal'
}

export default function ThreatOverlayWindowPage(): React.ReactElement {
  const opacity = useOverlayOpacity()
  const chrome = useOverlayChromeFade()
  const { locked, toggleLocked, rootInteractionProps, headerInteractionProps } =
    useOverlayLock('threat')
  const onDragMouseDown = useWindowDrag()
  const [state, setState] = useState<ThreatState | null>(null)
  const [raidState, setRaidState] = useState<RaidThreatState | null>(null)
  const [mode, setMode] = useState<ThreatMode>(loadMode)
  const raidEnabled = useRaidThreatEnabled()
  const effMode: ThreatMode = raidEnabled ? mode : 'personal'

  const chooseMode = (m: ThreatMode): void => {
    setMode(m)
    localStorage.setItem(MODE_KEY, m)
  }

  // Skip an initial REST snapshot that resolves after a WS broadcast already
  // applied — otherwise the stale fetch clobbers a fresher live update.
  const wsAppliedRef = useRef(false)
  const wsRaidAppliedRef = useRef(false)
  useEffect(() => {
    getThreatState().then((s) => { if (!wsAppliedRef.current) setState(s) }).catch(() => {})
  }, [])

  useEffect(() => {
    if (raidEnabled) getRaidThreatState().then((s) => { if (!wsRaidAppliedRef.current) setRaidState(s) }).catch(() => {})
  }, [raidEnabled])

  const handleMessage = useCallback((msg: { type: string; data: unknown }) => {
    if (msg.type === WSEvent.OverlayThreat) { wsAppliedRef.current = true; setState(msg.data as ThreatState) }
    else if (msg.type === WSEvent.OverlayRaidThreat) { wsRaidAppliedRef.current = true; setRaidState(msg.data as RaidThreatState) }
  }, [])

  useWebSocket(handleMessage)

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
          <Gauge size={11} style={{ color: '#c9a84c' }} />
          <span style={{ fontSize: 11, fontWeight: 700, color: 'rgba(255,255,255,0.8)' }}>
            Threat
          </span>
        </div>
        <div className="no-drag" style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
          {raidEnabled && <ThreatModeToggle mode={effMode} onChange={chooseMode} />}
          {effMode === 'personal' && (
            <button
              onClick={() => resetThreat().catch(() => {})}
              title="Reset threat"
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
          )}
          <OverlayLockButton locked={locked} onToggle={toggleLocked} />
          <button
            onClick={() => window.electron?.overlay?.closeThreat()}
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

      {/* ── Threat body ──────────────────────────────────────────────────── */}
      {effMode === 'raid' ? <RaidThreatContent state={raidState} /> : <ThreatContent state={state} />}
    </div>
  )
}
