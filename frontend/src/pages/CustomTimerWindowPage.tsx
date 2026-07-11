/**
 * CustomTimerWindowPage — transparent always-on-top overlay showing generic
 * user timers (category === 'custom'): manual countdowns started from the
 * quick-add form below, and trigger-driven timers with timer_type "custom".
 * Renders in a dedicated frameless Electron window; no sidebar or title bar.
 */
import React, { useCallback, useEffect, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { Bell, BellOff, Hourglass, Pin, Trash2, X } from 'lucide-react'
import { useWebSocket } from '../hooks/useWebSocket'
import { useDisplayThresholds, passesThreshold } from '../hooks/useDisplayThresholds'
import { useCustomTimerAlertPref } from '../hooks/useCustomTimerAlertPref'
import { customAlertThresholds, withTimerAlertDefaults } from '../lib/timerAlerts'
import { WSEvent } from '../lib/wsEvents'
import { useOverlayOpacity } from '../hooks/useOverlayOpacity'
import { useOverlayChromeFade } from '../hooks/useOverlayChromeFade'
import { useOverlayLock } from '../hooks/useOverlayLock'
import { useWindowDrag } from '../hooks/useWindowDrag'
import OverlayLockButton from '../components/OverlayLockButton'
import { getTimerState, removeTimer, startCustomTimer } from '../services/api'
import type { ActiveTimer, TimerState } from '../types/timer'

// ── Helpers ────────────────────────────────────────────────────────────────────

function fmtRemaining(secs: number): string {
  if (secs <= 0) return '0s'
  const s = Math.ceil(secs)
  if (s < 60) return `${s}s`
  if (s < 3600) {
    const m = Math.floor(s / 60)
    const rem = s % 60
    return rem > 0 ? `${m}m${rem}s` : `${m}m`
  }
  return `${Math.floor(s / 3600)}h${Math.floor((s % 3600) / 60)}m`
}

function barColor(remaining: number, total: number): string {
  if (total <= 0) return '#38bdf8'
  const pct = remaining / total
  if (pct > 0.5) return '#38bdf8'
  if (pct > 0.2) return '#f97316'
  return '#ef4444'
}

/**
 * Parse a human duration into seconds. Mirrors the backend's
 * trigger.ParseDurationText: plain seconds ("400"), colon notation
 * ("6:40", "1:02:03"), unit notation ("6m40s", "2h", "90s"). 0 = unparseable.
 */
function parseDurationText(raw: string): number {
  const s = raw.trim().toLowerCase()
  if (!s) return 0
  if (/^\d+$/.test(s)) return parseInt(s, 10)
  if (s.includes(':')) {
    let total = 0
    for (const part of s.split(':')) {
      if (!/^\d+$/.test(part.trim())) return 0
      total = total * 60 + parseInt(part.trim(), 10)
    }
    return total
  }
  const m = s.match(/^(?:(\d+)h)?(?:(\d+)m)?(?:(\d+)s?)?$/)
  if (!m) return 0
  const [, h, min, sec] = m
  return (parseInt(h ?? '0', 10) || 0) * 3600 +
    (parseInt(min ?? '0', 10) || 0) * 60 +
    (parseInt(sec ?? '0', 10) || 0)
}

// ── Timer row ──────────────────────────────────────────────────────────────────

function TimerRow({ timer }: { timer: ActiveTimer }): React.ReactElement {
  const pct =
    timer.duration_seconds > 0
      ? Math.max(0, Math.min(1, timer.remaining_seconds / timer.duration_seconds))
      : 0
  const color = barColor(timer.remaining_seconds, timer.duration_seconds)
  const urgent = pct < 0.2

  return (
    <div
      style={{
        position: 'relative',
        padding: '3px 8px',
        borderBottom: '1px solid rgba(255,255,255,0.1)',
        overflow: 'hidden',
        flexShrink: 0,
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
          {timer.pinned ? (
            <Pin size={13} style={{ color, flexShrink: 0 }} fill="currentColor" />
          ) : (
            <Hourglass size={13} style={{ color, flexShrink: 0 }} />
          )}
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
            {timer.target_name && (
              <span style={{ color: 'rgba(255,255,255,0.6)', fontWeight: 400 }}>{` — ${timer.target_name}`}</span>
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

export default function CustomTimerWindowPage(): React.ReactElement {
  // Present (non-empty) only when the main process opened this window for a
  // named TimerGroup (see createCustomTimerGroupOverlay) — the default/
  // original window loads with no query string at all. Locked-mode
  // preference still resolves via the shared 'customTimer' key (a single
  // per-type setting, not per-window — see useOverlayLock).
  const [searchParams] = useSearchParams()
  const groupId = searchParams.get('group') ?? ''
  const groupName = searchParams.get('name') ?? ''

  const opacity = useOverlayOpacity()
  const chrome = useOverlayChromeFade()
  const { locked, toggleLocked, rootInteractionProps, headerInteractionProps } =
    useOverlayLock('customTimer')
  const onDragMouseDown = useWindowDrag()
  const [state, setState] = useState<TimerState | null>(null)
  const thresholds = useDisplayThresholds()
  const [newName, setNewName] = useState('')
  const [newDuration, setNewDuration] = useState('')
  const [addError, setAddError] = useState(false)
  // Per-add alert toggle. null = follow the global default; once the user
  // clicks the bell it holds their explicit choice for subsequent adds.
  const alertPref = useCustomTimerAlertPref()
  const [alertOverride, setAlertOverride] = useState<boolean | null>(null)
  const bellOn = alertOverride ?? (alertPref?.enabled ?? false)

  useEffect(() => {
    getTimerState().then(setState).catch(() => {})
  }, [])

  const handleMessage = useCallback((msg: { type: string; data: unknown }) => {
    if (msg.type === WSEvent.OverlayTimers) {
      setState(msg.data as TimerState)
    }
  }, [])

  useWebSocket(handleMessage)

  const timers = (state?.timers ?? [])
    .filter((t) => t.category === 'custom' && (groupId ? t.custom_group === groupId : !t.custom_group))
    .filter((t) => passesThreshold(t, thresholds))

  const handleAdd = (e: React.FormEvent): void => {
    e.preventDefault()
    const secs = parseDurationText(newDuration)
    if (!newName.trim() || secs <= 0) {
      setAddError(true)
      return
    }
    setAddError(false)
    // When the bell is lit, arm this timer with the global alert config (or a
    // sensible built-in default if none is configured); otherwise stay silent.
    const alerts = bellOn
      ? customAlertThresholds({ ...withTimerAlertDefaults(alertPref, 'custom'), enabled: true })
      : undefined
    startCustomTimer(newName.trim(), secs, alerts, undefined, groupId || undefined)
      .then(() => {
        setNewName('')
        setNewDuration('')
      })
      .catch(() => setAddError(true))
  }

  // clearTimers('custom') would wipe every custom timer across every window
  // (default + every named group), not just this one — so a per-window
  // "Clear" removes only the timers this window is currently showing.
  const handleClear = (): void => {
    Promise.all(timers.map((t) => removeTimer(t.id))).catch(() => {})
  }

  const quickInputStyle: React.CSSProperties = {
    backgroundColor: 'rgba(255,255,255,0.08)',
    border: `1px solid ${addError ? 'rgba(248,113,113,0.7)' : 'rgba(255,255,255,0.15)'}`,
    borderRadius: 3,
    color: 'rgba(255,255,255,0.9)',
    fontSize: 11,
    padding: '2px 6px',
    outline: 'none',
    minWidth: 0,
  }

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
          // Fade-when-inactive: hide the title bar with the rest of the
          // chrome; pointerEvents off so invisible buttons can't be clicked.
          opacity: chrome ? 1 : 0,
          pointerEvents: chrome ? 'auto' : 'none',
          transition: 'opacity 0.4s ease',
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
          <Hourglass size={11} style={{ color: '#38bdf8' }} />
          <span style={{ fontSize: 11, fontWeight: 700, color: 'rgba(255,255,255,0.8)' }}>
            {groupName || 'Timers'}
          </span>
          {timers.length > 0 && (
            <span style={{ fontSize: 10, color: 'rgba(255,255,255,0.35)', marginLeft: 2 }}>
              {timers.length}
            </span>
          )}
        </div>
        <div
          className="no-drag"
          style={{ display: 'flex', alignItems: 'center', gap: 6 }}
        >
          <button
            onClick={handleClear}
            title="Clear all timers in this window"
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
            onClick={() => {
              if (groupId) window.electron?.overlay?.closeCustomTimerGroup(groupId)
              else window.electron?.overlay?.closeCustomTimer()
            }}
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
              // Idle placeholder — fade with the chrome, it isn't live content.
              opacity: chrome ? 1 : 0,
              transition: 'opacity 0.4s ease',
            }}
          >
            <Hourglass size={22} style={{ opacity: 0.15, color: '#38bdf8' }} />
            <p style={{ fontSize: 11, color: 'rgba(255,255,255,0.25)', margin: 0 }}>
              No active timers
            </p>
          </div>
        ) : (
          timers.map((t) => <TimerRow key={t.id} timer={t} />)
        )}
      </div>

      {/* ── Quick-add form — default window only. Named group windows are
          fed exclusively by triggers assigned to them (see the trigger
          editor's "Custom Timers window" picker) — no manual add here. ── */}
      {!groupId && (
        <form
          onSubmit={handleAdd}
          className="no-drag"
          style={{
            display: 'flex',
            gap: 4,
            padding: '5px 8px',
            borderTop: '1px solid rgba(255,255,255,0.1)',
            backgroundColor: 'rgba(255,255,255,0.04)',
            flexShrink: 0,
            opacity: chrome ? 1 : 0,
            pointerEvents: chrome ? 'auto' : 'none',
            transition: 'opacity 0.4s ease',
          }}
        >
          <input
            type="text"
            value={newName}
            onChange={(e) => { setNewName(e.target.value); setAddError(false) }}
            placeholder="Timer name"
            style={{ ...quickInputStyle, flex: 1 }}
          />
          <input
            type="text"
            value={newDuration}
            onChange={(e) => { setNewDuration(e.target.value); setAddError(false) }}
            placeholder="5m / 300 / 6:40"
            title="Duration: seconds (300), colon notation (6:40), or units (6m40s)"
            style={{ ...quickInputStyle, width: 78 }}
          />
          <button
            type="button"
            onClick={() => setAlertOverride(!bellOn)}
            title={bellOn ? 'Alert when this timer finishes (click to mute)' : 'No alert for new timers (click to enable)'}
            aria-pressed={bellOn}
            style={{
              display: 'flex',
              alignItems: 'center',
              padding: '2px 6px',
              borderRadius: 3,
              border: '1px solid rgba(255,255,255,0.15)',
              backgroundColor: bellOn ? 'rgba(56,189,248,0.2)' : 'rgba(255,255,255,0.04)',
              color: bellOn ? 'rgba(255,255,255,0.85)' : 'rgba(255,255,255,0.4)',
              cursor: 'pointer',
              flexShrink: 0,
            }}
          >
            {bellOn ? <Bell size={12} /> : <BellOff size={12} />}
          </button>
          <button
            type="submit"
            style={{
              fontSize: 11,
              padding: '2px 8px',
              borderRadius: 3,
              border: '1px solid rgba(255,255,255,0.15)',
              backgroundColor: 'rgba(56,189,248,0.2)',
              color: 'rgba(255,255,255,0.85)',
              cursor: 'pointer',
              lineHeight: 1.4,
              flexShrink: 0,
            }}
          >
            Start
          </button>
        </form>
      )}
    </div>
  )
}
