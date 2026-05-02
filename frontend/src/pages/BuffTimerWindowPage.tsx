/**
 * BuffTimerWindowPage — transparent always-on-top overlay showing active buff timers.
 * Renders in a dedicated frameless Electron window; no sidebar or title bar.
 * Shows timers with category === 'buff'.
 */
import React, { useCallback, useEffect, useState } from 'react'
import { Shield, Eraser } from 'lucide-react'
import { useWebSocket } from '../hooks/useWebSocket'
import { useActivePlayerName, targetSuffix } from '../hooks/useActivePlayerName'
import { useDisplayThresholds, passesThreshold } from '../hooks/useDisplayThresholds'
import { useOverlayOpacity } from '../hooks/useOverlayOpacity'
import { useOverlayLock } from '../hooks/useOverlayLock'
import OverlayLockButton from '../components/OverlayLockButton'
import { clearTimers, getTimerState } from '../services/api'
import type { ActiveTimer, TimerState } from '../types/timer'

// ── Helpers ────────────────────────────────────────────────────────────────────

function fmtRemaining(secs: number): string {
  if (secs <= 0) return '0s'
  if (secs < 60) return `${Math.ceil(secs)}s`
  const m = Math.floor(secs / 60)
  const s = Math.ceil(secs % 60)
  return s > 0 ? `${m}m ${s}s` : `${m}m`
}

function barColor(remaining: number, total: number): string {
  if (total <= 0) return '#22c55e'
  const pct = remaining / total
  if (pct > 0.5) return '#22c55e'
  if (pct > 0.2) return '#f97316'
  return '#ef4444'
}

// ── Timer row ──────────────────────────────────────────────────────────────────

function TimerRow({ timer, activePlayer }: { timer: ActiveTimer; activePlayer: string }): React.ReactElement {
  const pct =
    timer.duration_seconds > 0
      ? Math.max(0, Math.min(1, timer.remaining_seconds / timer.duration_seconds))
      : 0
  const color = barColor(timer.remaining_seconds, timer.duration_seconds)
  const urgent = pct < 0.2
  const onTarget = targetSuffix(timer.target_name, activePlayer)

  return (
    <div
      style={{
        position: 'relative',
        padding: '5px 8px',
        borderBottom: '1px solid rgba(255,255,255,0.06)',
        overflow: 'hidden',
      }}
    >
      {/* depleting progress bar */}
      <div
        style={{
          position: 'absolute',
          left: 0,
          top: 0,
          bottom: 0,
          width: `${pct * 100}%`,
          backgroundColor: color,
          opacity: 0.18,
          pointerEvents: 'none',
          transition: 'width 1s linear',
        }}
      />
      <div
        style={{
          position: 'relative',
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          gap: 8,
        }}
      >
        <span
          style={{
            fontSize: 12,
            color: urgent ? '#f87171' : 'rgba(255,255,255,0.9)',
            overflow: 'hidden',
            textOverflow: 'ellipsis',
            whiteSpace: 'nowrap',
            fontWeight: urgent ? 600 : 400,
          }}
        >
          {timer.spell_name}
          {onTarget && (
            <span style={{ color: 'rgba(255,255,255,0.45)', fontWeight: 400 }}>{onTarget}</span>
          )}
        </span>
        <span
          style={{
            fontSize: 11,
            color: urgent ? '#f87171' : color,
            fontVariantNumeric: 'tabular-nums',
            flexShrink: 0,
            fontWeight: urgent ? 700 : 400,
          }}
        >
          {fmtRemaining(timer.remaining_seconds)}
        </span>
      </div>
    </div>
  )
}

// ── Page ───────────────────────────────────────────────────────────────────────

export default function BuffTimerWindowPage(): React.ReactElement {
  const opacity = useOverlayOpacity()
  const { locked, toggleLocked, enableInteraction, enableClickThrough } = useOverlayLock()
  const [state, setState] = useState<TimerState | null>(null)
  const activePlayer = useActivePlayerName()
  const thresholds = useDisplayThresholds()

  useEffect(() => {
    getTimerState().then(setState).catch(() => {})
  }, [])

  const handleMessage = useCallback((msg: { type: string; data: unknown }) => {
    if (msg.type === 'overlay:timers') {
      const data = msg.data as TimerState
      // eslint-disable-next-line no-console
      console.log('[timer-debug] BuffTimerWindowPage state update, total=', data?.timers?.length ?? 0)
      setState(data)
    }
  }, [])

  useWebSocket(handleMessage)

  const buffs = (state?.timers ?? [])
    .filter((t) => t.category === 'buff')
    .filter((t) => passesThreshold(t, thresholds))

  return (
    <div
      onMouseLeave={enableClickThrough}
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
        className={locked ? 'no-drag' : 'drag-region'}
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
          <Shield size={11} style={{ color: '#22c55e' }} />
          <span style={{ fontSize: 11, fontWeight: 700, color: 'rgba(255,255,255,0.8)' }}>
            Buffs
          </span>
          {buffs.length > 0 && (
            <span style={{ fontSize: 10, color: 'rgba(255,255,255,0.35)', marginLeft: 2 }}>
              {buffs.length}
            </span>
          )}
        </div>
        <div
          className="no-drag"
          onMouseEnter={enableInteraction}
          onMouseLeave={enableClickThrough}
          style={{ display: 'flex', alignItems: 'center', gap: 6 }}
        >
          <button
            onClick={() => clearTimers('buff').catch(() => {})}
            title="Clear all active buff timers"
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
            <Eraser size={11} />
          </button>
          <OverlayLockButton locked={locked} onToggle={toggleLocked} />
          <button
            onClick={() => window.electron?.overlay?.closeBuffTimer()}
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
      <div style={{ flex: 1, overflow: 'auto' }}>
        {state === null ? (
          <p
            style={{
              padding: 12,
              fontSize: 11,
              color: 'rgba(255,255,255,0.3)',
              textAlign: 'center',
              margin: 0,
            }}
          >
            Connecting…
          </p>
        ) : buffs.length === 0 ? (
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
            <Shield size={22} style={{ opacity: 0.15, color: '#22c55e' }} />
            <p style={{ fontSize: 11, color: 'rgba(255,255,255,0.25)', margin: 0 }}>
              No active buffs
            </p>
          </div>
        ) : (
          buffs.map((t) => <TimerRow key={t.id} timer={t} activePlayer={activePlayer} />)
        )}
      </div>
    </div>
  )
}
