/**
 * CHChainOverlayWindowPage — transparent always-on-top overlay showing the
 * active Complete-Heal chain as a countdown bar per chain position. Renders in
 * a dedicated frameless Electron window. Shows timers with category ===
 * 'ch_chain', sorted by chain position. Each timer's label is "#N  Target  ←
 * Caster", produced by the backend CH-chain matcher.
 */
import React, { useCallback, useEffect, useState } from 'react'
import { HeartPulse, Trash2, X } from 'lucide-react'
import { useWebSocket } from '../hooks/useWebSocket'
import { WSEvent } from '../lib/wsEvents'
import { useOverlayOpacity } from '../hooks/useOverlayOpacity'
import { useOverlayLock } from '../hooks/useOverlayLock'
import { useWindowDrag } from '../hooks/useWindowDrag'
import OverlayLockButton from '../components/OverlayLockButton'
import { clearTimers, getTimerState } from '../services/api'
import type { ActiveTimer, TimerState } from '../types/timer'

function fmtRemaining(secs: number): string {
  if (secs <= 0) return '0s'
  return `${Math.ceil(secs)}s`
}

// parseLabel splits the matcher's "#N  Target  ← Caster" label into a numeric
// chain position (for sorting + a badge) and the remaining display text.
function parseLabel(label: string): { position: number; text: string } {
  const m = /^#(\d+)\s+(.*)$/.exec(label)
  if (!m) return { position: Number.MAX_SAFE_INTEGER, text: label }
  return { position: parseInt(m[1], 10), text: m[2] }
}

// computeCadence derives the chain's live spacing from the gaps between
// consecutive callout start times — so it follows the raid leader speeding
// up / slowing down the chain mid-fight, rather than a static config value.
// Returns null when there aren't yet two callouts to measure. `stalled` is
// true when the most recent gap ran notably longer than the running median,
// i.e. the chain skipped a beat and a spot-heal may be needed.
function computeCadence(
  timers: ActiveTimer[],
): { cadence: number; stalled: boolean } | null {
  const starts = timers
    .map((t) => Date.parse(t.starts_at))
    .filter((ms) => !Number.isNaN(ms))
    .sort((a, b) => a - b)
  if (starts.length < 2) return null
  const gaps: number[] = []
  for (let i = 1; i < starts.length; i++) {
    gaps.push((starts[i] - starts[i - 1]) / 1000)
  }
  const sorted = [...gaps].sort((a, b) => a - b)
  const median = sorted[Math.floor(sorted.length / 2)]
  const last = gaps[gaps.length - 1]
  return { cadence: median, stalled: gaps.length >= 2 && last > median * 1.5 }
}

function ChainRow({ timer }: { timer: ActiveTimer }): React.ReactElement {
  const pct =
    timer.duration_seconds > 0
      ? Math.max(0, Math.min(1, timer.remaining_seconds / timer.duration_seconds))
      : 0
  const { position, text } = parseLabel(timer.spell_name)
  // Each bar is the 10s CH cast counting down to the heal landing, so a
  // near-empty bar means "heal incoming" — a good thing. Highlight it green,
  // not red (red is reserved for the header stall warning).
  const landing = pct < 0.34

  return (
    <div
      style={{
        position: 'relative',
        padding: '3px 8px',
        borderBottom: '1px solid rgba(255,255,255,0.1)',
        overflow: 'hidden',
      }}
    >
      <div
        style={{
          position: 'absolute',
          left: 0,
          top: 0,
          bottom: 0,
          width: `${pct * 100}%`,
          backgroundColor: landing ? '#22c55e' : '#3b82f6',
          opacity: 0.5,
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
        <div style={{ display: 'flex', alignItems: 'center', gap: 6, minWidth: 0, flex: 1 }}>
          {position !== Number.MAX_SAFE_INTEGER && (
            <span
              style={{
                fontSize: 10,
                fontWeight: 700,
                color: '#fff',
                backgroundColor: 'rgba(59,130,246,0.6)',
                borderRadius: 3,
                padding: '0 4px',
                flexShrink: 0,
                fontVariantNumeric: 'tabular-nums',
                textShadow: '0 1px 2px rgba(0,0,0,0.9)',
              }}
            >
              {position}
            </span>
          )}
          <span
            style={{
              fontSize: 12,
              color: 'rgba(255,255,255,1)',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
              fontWeight: landing ? 600 : 500,
              textShadow: '0 1px 2px rgba(0,0,0,0.9)',
            }}
          >
            {text}
          </span>
        </div>
        <span
          style={{
            fontSize: 11,
            color: landing ? '#86efac' : '#93c5fd',
            fontVariantNumeric: 'tabular-nums',
            flexShrink: 0,
            fontWeight: landing ? 700 : 600,
            textShadow: '0 1px 2px rgba(0,0,0,0.9)',
          }}
        >
          {fmtRemaining(timer.remaining_seconds)}
        </span>
      </div>
    </div>
  )
}

export default function CHChainOverlayWindowPage(): React.ReactElement {
  const opacity = useOverlayOpacity()
  const { locked, toggleLocked, rootInteractionProps, headerInteractionProps } =
    useOverlayLock('chChain')
  const onDragMouseDown = useWindowDrag()
  const [state, setState] = useState<TimerState | null>(null)

  useEffect(() => {
    getTimerState().then(setState).catch(() => {})
  }, [])

  const handleMessage = useCallback((msg: { type: string; data: unknown }) => {
    if (msg.type === WSEvent.OverlayTimers) {
      setState(msg.data as TimerState)
    }
  }, [])
  useWebSocket(handleMessage)

  const chain = (state?.timers ?? [])
    .filter((t) => t.category === 'ch_chain')
    .sort((a, b) => {
      const pa = parseLabel(a.spell_name).position
      const pb = parseLabel(b.spell_name).position
      if (pa !== pb) return pa - pb
      return a.remaining_seconds - b.remaining_seconds
    })
  const cadence = computeCadence(chain)

  return (
    <div
      {...rootInteractionProps}
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
      <div
        {...headerInteractionProps}
        onMouseDown={onDragMouseDown}
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
          <HeartPulse size={11} style={{ color: '#3b82f6' }} />
          <span style={{ fontSize: 11, fontWeight: 700, color: 'rgba(255,255,255,0.8)' }}>
            CH Chain
          </span>
          {chain.length > 0 && (
            <span style={{ fontSize: 10, color: 'rgba(255,255,255,0.35)', marginLeft: 2 }}>
              {chain.length}
            </span>
          )}
          {cadence && (
            <span
              title={
                cadence.stalled
                  ? 'Chain slowed — last gap ran long; a spot-heal may be needed'
                  : 'Live chain cadence (measured between callouts)'
              }
              style={{
                fontSize: 10,
                fontWeight: 700,
                marginLeft: 4,
                color: cadence.stalled ? '#fca5a5' : 'rgba(147,197,253,0.9)',
                fontVariantNumeric: 'tabular-nums',
              }}
            >
              {cadence.stalled ? '⚠ ' : ''}
              {cadence.cadence.toFixed(1)}s
            </span>
          )}
        </div>
        <div className="no-drag" style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
          <button
            onClick={() => clearTimers('ch_chain').catch(() => {})}
            title="Clear the current chain"
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
            onClick={() => window.electron?.overlay?.closeCHChain()}
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
            <X size={11} />
          </button>
        </div>
      </div>

      <div style={{ flex: 1, overflow: 'auto', display: 'flex', flexDirection: 'column' }}>
        {state === null ? (
          <p style={{ padding: 12, fontSize: 11, color: 'rgba(255,255,255,0.3)', textAlign: 'center', margin: 0 }}>
            Connecting…
          </p>
        ) : chain.length === 0 ? (
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
            <HeartPulse size={22} style={{ opacity: 0.15, color: '#3b82f6' }} />
            <p style={{ fontSize: 11, color: 'rgba(255,255,255,0.25)', margin: 0, textAlign: 'center' }}>
              Waiting for a CH chain…
            </p>
          </div>
        ) : (
          chain.map((t) => <ChainRow key={t.id} timer={t} />)
        )}
      </div>
    </div>
  )
}
