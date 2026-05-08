/**
 * DetrimTimerWindowPage — transparent always-on-top overlay showing active
 * detrimental spell timers (debuffs, DoTs, mez, stuns cast on enemies).
 * Renders in a dedicated frameless Electron window; no sidebar or title bar.
 * Shows timers with category in: 'debuff' | 'dot' | 'mez' | 'stun'.
 */
import React, { useCallback, useEffect, useState } from 'react'
import { Skull, Eraser, X } from 'lucide-react'
import { useWebSocket } from '../hooks/useWebSocket'
import { useActivePlayerName } from '../hooks/useActivePlayerName'
import { useDisplayThresholds, passesThreshold } from '../hooks/useDisplayThresholds'
import { useOverlayOpacity } from '../hooks/useOverlayOpacity'
import { useOverlayLock } from '../hooks/useOverlayLock'
import OverlayLockButton from '../components/OverlayLockButton'
import { clearTimers, getTimerState, removeTimer } from '../services/api'
import { SpellIcon } from '../components/Icon'
import type { ActiveTimer, TimerCategory, TimerState } from '../types/timer'

// ── Constants ──────────────────────────────────────────────────────────────────

const DETRIM_CATEGORIES = new Set<TimerCategory>(['debuff', 'dot', 'mez', 'stun'])

const CATEGORY_COLORS: Record<TimerCategory, string> = {
  buff: '#22c55e',
  debuff: '#f97316',
  dot: '#ef4444',
  mez: '#a855f7',
  stun: '#eab308',
}

// ── Helpers ────────────────────────────────────────────────────────────────────

/**
 * The detrim target — almost always an NPC — is the most useful identifier
 * after the spell name itself, so it's shown inline rather than tucked into
 * an "on …" suffix. Returns an empty string for self-targeted entries (rare
 * for detrimentals) and trigger-driven timers with no target.
 */
function detrimTarget(targetName: string, activePlayer: string): string {
  if (!targetName) return ''
  if (targetName === activePlayer) return ''
  if (targetName === 'You') return ''
  return targetName
}

function fmtRemaining(secs: number): string {
  if (secs <= 0) return '0s'
  if (secs < 60) return `${Math.ceil(secs)}s`
  return `${Math.ceil(secs / 60)}m`
}

function barColor(remaining: number, total: number, category: TimerCategory): string {
  if (total <= 0) return CATEGORY_COLORS[category]
  const pct = remaining / total
  if (pct > 0.2) return CATEGORY_COLORS[category]
  return '#ef4444'
}

// ── Timer row ──────────────────────────────────────────────────────────────────

function TimerRow({ timer, activePlayer }: { timer: ActiveTimer; activePlayer: string }): React.ReactElement {
  const pct =
    timer.duration_seconds > 0
      ? Math.max(0, Math.min(1, timer.remaining_seconds / timer.duration_seconds))
      : 0
  const color = barColor(timer.remaining_seconds, timer.duration_seconds, timer.category)
  const urgent = pct < 0.2
  const catColor = CATEGORY_COLORS[timer.category]
  const target = detrimTarget(timer.target_name, activePlayer)

  return (
    <div
      style={{
        position: 'relative',
        padding: '3px 8px',
        borderBottom: '1px solid rgba(255,255,255,0.1)',
        overflow: 'hidden',
      }}
    >
      {/* depleting progress bar — kept at high alpha so it stays readable even when the window opacity is low */}
      <div
        style={{
          position: 'absolute',
          left: 0,
          top: 0,
          bottom: 0,
          width: `${pct * 100}%`,
          backgroundColor: color,
          opacity: 0.55,
          pointerEvents: 'none',
          transition: 'width 1s linear',
        }}
      />
      {/* left accent line */}
      <div
        style={{
          position: 'absolute',
          left: 0,
          top: 0,
          bottom: 0,
          width: 2,
          backgroundColor: catColor,
          opacity: 0.85,
        }}
      />
      <div
        style={{
          position: 'relative',
          display: 'flex',
          justifyContent: 'space-between',
          alignItems: 'center',
          gap: 6,
          paddingLeft: 6,
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: 5, minWidth: 0, flex: 1 }}>
          <SpellIcon id={timer.icon} name={timer.spell_name} size={16} loading="eager" />
          <span
            style={{
              fontSize: 12,
              color: urgent ? '#f87171' : 'rgba(255,255,255,1)',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
              fontWeight: urgent ? 600 : 500,
              textShadow: '0 1px 2px rgba(0,0,0,0.9)',
            }}
          >
            {timer.spell_name}
            {target && (
              <span style={{ color: 'rgba(255,255,255,0.7)', fontWeight: 400 }}>{` — ${target}`}</span>
            )}
          </span>
        </div>
        <span
          style={{
            fontSize: 11,
            color: urgent ? '#f87171' : color,
            fontVariantNumeric: 'tabular-nums',
            flexShrink: 0,
            fontWeight: urgent ? 700 : 600,
            textShadow: '0 1px 2px rgba(0,0,0,0.9)',
          }}
        >
          {fmtRemaining(timer.remaining_seconds)}
        </span>
        <button
          onClick={() => removeTimer(timer.id).catch(() => {})}
          title="Remove this timer"
          style={{
            background: 'none',
            border: 'none',
            cursor: 'pointer',
            padding: 0,
            color: 'rgba(255,255,255,0.55)',
            display: 'flex',
            alignItems: 'center',
            flexShrink: 0,
            lineHeight: 0,
          }}
        >
          <X size={11} />
        </button>
      </div>
    </div>
  )
}

// ── Page ───────────────────────────────────────────────────────────────────────

export default function DetrimTimerWindowPage(): React.ReactElement {
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
      setState(msg.data as TimerState)
    }
  }, [])

  useWebSocket(handleMessage)

  const detrims = (state?.timers ?? [])
    .filter((t) => DETRIM_CATEGORIES.has(t.category))
    .filter((t) => passesThreshold(t, thresholds))

  return (
    <div
      onMouseEnter={enableInteraction}
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
          <Skull size={11} style={{ color: '#ef4444' }} />
          <span style={{ fontSize: 11, fontWeight: 700, color: 'rgba(255,255,255,0.8)' }}>
            Detrimental
          </span>
          {detrims.length > 0 && (
            <span style={{ fontSize: 10, color: 'rgba(255,255,255,0.35)', marginLeft: 2 }}>
              {detrims.length}
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
            onClick={() => clearTimers('detrimental').catch(() => {})}
            title="Clear all active detrimental timers"
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
            onClick={() => window.electron?.overlay?.closeDetrimTimer()}
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
        ) : detrims.length === 0 ? (
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
            <Skull size={22} style={{ opacity: 0.15, color: '#ef4444' }} />
            <p style={{ fontSize: 11, color: 'rgba(255,255,255,0.25)', margin: 0 }}>
              No active detrimentals
            </p>
          </div>
        ) : (
          detrims.map((t) => <TimerRow key={t.id} timer={t} activePlayer={activePlayer} />)
        )}
      </div>
    </div>
  )
}
