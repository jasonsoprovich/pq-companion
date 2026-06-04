/**
 * CHChainPanel — in-dashboard version of the Complete-Heal chain overlay. Shows
 * the active CH chain as a countdown bar per chain position (category ===
 * 'ch_chain'), sorted by position. Mirrors CHChainOverlayWindowPage but renders
 * inside the draggable/resizable OverlayWindow used by the Overlays dashboard,
 * themed to the app surface tokens. The pop-out button toggles the standalone
 * floating window.
 */
import React, { useCallback, useEffect, useState } from 'react'
import { HeartPulse, Trash2, ExternalLink } from 'lucide-react'
import { useWebSocket } from '../../hooks/useWebSocket'
import { WSEvent } from '../../lib/wsEvents'
import { clearTimers, getTimerState } from '../../services/api'
import OverlayWindow from '../OverlayWindow'
import type { ActiveTimer, TimerState } from '../../types/timer'

interface CHChainPanelProps {
  defaultX?: number
  defaultY?: number
  defaultWidth?: number
  defaultHeight?: number
  snapGridSize?: number
  onLayoutChange?: (b: { x: number; y: number; width: number; height: number }) => void
}

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
// consecutive callout start times, so it follows the raid leader speeding up /
// slowing down the chain mid-fight. Returns null until there are two callouts
// to measure. `stalled` is true when the most recent gap ran notably longer
// than the running median, i.e. the chain skipped a beat.
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
  // The bar is the 10s CH cast counting down to the heal landing, so a
  // near-empty bar means "heal incoming" — highlight green, not red.
  const landing = pct < 0.34

  return (
    <div style={{ position: 'relative', padding: '3px 10px', borderBottom: '1px solid var(--color-border)', overflow: 'hidden', flexShrink: 0 }}>
      <div
        style={{
          position: 'absolute', left: 0, top: 0, bottom: 0,
          width: `${pct * 100}%`, backgroundColor: landing ? '#22c55e' : '#3b82f6',
          opacity: 0.18, pointerEvents: 'none', transition: 'width 1s linear',
        }}
      />
      <div style={{ position: 'relative', display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 8 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 6, minWidth: 0, flex: 1 }}>
          {position !== Number.MAX_SAFE_INTEGER && (
            <span
              style={{
                fontSize: 10, fontWeight: 700, color: '#fff',
                backgroundColor: 'rgba(59,130,246,0.6)', borderRadius: 3,
                padding: '0 4px', flexShrink: 0, fontVariantNumeric: 'tabular-nums',
              }}
            >
              {position}
            </span>
          )}
          <span style={{ fontSize: 12, color: 'var(--color-foreground)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', fontWeight: landing ? 600 : 400 }}>
            {text}
          </span>
        </div>
        <span style={{ fontSize: 11, color: landing ? '#22c55e' : '#60a5fa', fontVariantNumeric: 'tabular-nums', flexShrink: 0, fontWeight: landing ? 700 : 500 }}>
          {fmtRemaining(timer.remaining_seconds)}
        </span>
      </div>
    </div>
  )
}

export default function CHChainPanel({
  defaultX = 24,
  defaultY = 24,
  defaultWidth = 304,
  defaultHeight = 336,
  snapGridSize,
  onLayoutChange,
}: CHChainPanelProps): React.ReactElement {
  const [state, setState] = useState<TimerState | null>(null)

  useEffect(() => {
    getTimerState().then(setState).catch(() => {})
  }, [])

  const handleMessage = useCallback((msg: { type: string; data: unknown }) => {
    if (msg.type === WSEvent.OverlayTimers) setState(msg.data as TimerState)
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
    <OverlayWindow
      title={
        <span style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
          <HeartPulse size={13} style={{ color: '#3b82f6' }} />
          CH Chain
          {chain.length > 0 && (
            <span style={{ fontSize: 10, color: 'var(--color-muted)', fontWeight: 400 }}>{chain.length}</span>
          )}
          {cadence && (
            <span
              title={
                cadence.stalled
                  ? 'Chain slowed — last gap ran long; a spot-heal may be needed'
                  : 'Live chain cadence (measured between callouts)'
              }
              style={{
                fontSize: 10, fontWeight: 700, marginLeft: 2,
                color: cadence.stalled ? '#f87171' : '#60a5fa',
                fontVariantNumeric: 'tabular-nums',
              }}
            >
              {cadence.stalled ? '⚠ ' : ''}
              {cadence.cadence.toFixed(1)}s
            </span>
          )}
        </span>
      }
      headerRight={
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <button
            onClick={() => clearTimers('ch_chain').catch(() => {})}
            title="Clear the current chain"
            style={{ background: 'none', border: 'none', cursor: 'pointer', padding: '1px 3px', color: 'var(--color-muted)', display: 'flex', alignItems: 'center' }}
          >
            <Trash2 size={12} />
          </button>
          {window.electron?.overlay && (
            <button
              onClick={() => window.electron.overlay.toggleCHChain()}
              title="Pop out as floating overlay"
              style={{ background: 'none', border: 'none', cursor: 'pointer', padding: '1px 3px', color: 'var(--color-muted)', display: 'flex', alignItems: 'center' }}
            >
              <ExternalLink size={12} />
            </button>
          )}
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
      <div style={{ flex: 1, minHeight: 0, overflow: 'auto', display: 'flex', flexDirection: 'column' }}>
        {state === null ? (
          <div style={{ flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', gap: 8, color: 'var(--color-muted)', padding: 16 }}>
            <HeartPulse size={28} style={{ opacity: 0.2, color: '#3b82f6' }} />
            <p style={{ fontSize: 12, margin: 0 }}>Connecting…</p>
          </div>
        ) : chain.length === 0 ? (
          <div style={{ flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', gap: 8, color: 'var(--color-muted)', padding: 16 }}>
            <HeartPulse size={28} style={{ opacity: 0.2, color: '#3b82f6' }} />
            <p style={{ fontSize: 12, margin: 0 }}>Waiting for a CH chain…</p>
          </div>
        ) : (
          chain.map((t) => <ChainRow key={t.id} timer={t} />)
        )}
      </div>
    </OverlayWindow>
  )
}
