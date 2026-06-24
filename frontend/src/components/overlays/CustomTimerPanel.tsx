/**
 * CustomTimerPanel — in-dashboard version of the Custom Timers overlay. Shows
 * generic user timers (category === 'custom'): manual countdowns started from
 * the quick-add form below, and trigger-driven timers with timer_type
 * "custom". Mirrors CustomTimerWindowPage but renders inside the
 * draggable/resizable OverlayWindow used by the Overlays dashboard, themed to
 * the app surface tokens. The pop-out button toggles the standalone floating
 * window.
 */
import React, { useCallback, useEffect, useState } from 'react'
import { Bell, BellOff, Hourglass, Palette, Trash2, ExternalLink, X } from 'lucide-react'
import { useWebSocket } from '../../hooks/useWebSocket'
import { useDisplayThresholds, passesThreshold } from '../../hooks/useDisplayThresholds'
import { useTimerAppearance, type TimerAppearance } from '../../hooks/useTimerAppearance'
import { useCustomTimerAlertPref } from '../../hooks/useCustomTimerAlertPref'
import { customAlertThresholds, withTimerAlertDefaults } from '../../lib/timerAlerts'
import { WSEvent } from '../../lib/wsEvents'
import { clearTimers, getTimerState, removeTimer, startCustomTimer } from '../../services/api'
import OverlayWindow from '../OverlayWindow'
import type { ActiveTimer, TimerState } from '../../types/timer'

interface CustomTimerPanelProps {
  defaultX?: number
  defaultY?: number
  defaultWidth?: number
  defaultHeight?: number
  snapGridSize?: number
  onLayoutChange?: (b: { x: number; y: number; width: number; height: number }) => void
}

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

function TimerRow({ timer, appearance }: { timer: ActiveTimer; appearance: TimerAppearance }): React.ReactElement {
  const expired = timer.expired === true
  const overdue = expired ? -timer.remaining_seconds : 0
  const pct = expired
    ? 1
    : timer.duration_seconds > 0
      ? Math.max(0, Math.min(1, timer.remaining_seconds / timer.duration_seconds))
      : 0
  // Expired forces red; otherwise a per-timer bar_color overrides the auto color.
  const color = expired ? '#ef4444' : (timer.bar_color || barColor(timer.remaining_seconds, timer.duration_seconds))
  const fillOpacity = expired
    ? (appearance.fillOpacity === 0 ? 0 : Math.min(1, appearance.fillOpacity + 0.07))
    : appearance.fillOpacity
  const urgent = expired || pct < 0.2

  return (
    <div style={{ position: 'relative', padding: `${appearance.rowPadding}px 10px`, borderBottom: '1px solid var(--color-border)', overflow: 'hidden', flexShrink: 0 }}>
      <div
        style={{
          position: 'absolute', left: 0, top: 0, bottom: 0,
          width: `${pct * 100}%`, backgroundColor: color,
          opacity: fillOpacity, pointerEvents: 'none', transition: 'width 1s linear',
        }}
      />
      <div style={{ position: 'relative', display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 8 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 6, minWidth: 0, flex: 1 }}>
          <Hourglass size={12} style={{ color, flexShrink: 0 }} />
          <span style={{ fontSize: appearance.nameFontSize, color: urgent ? '#f87171' : 'var(--color-foreground)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', fontWeight: urgent ? 600 : 400 }}>
            {timer.spell_name}
            {timer.target_name && (
              <span style={{ color: 'var(--color-muted)', fontWeight: 400 }}>{` — ${timer.target_name}`}</span>
            )}
          </span>
        </div>
        <span
          title={expired ? 'Expired — restart or dismiss with ✕' : undefined}
          style={{ fontSize: appearance.timeFontSize, color: urgent ? '#f87171' : color, fontVariantNumeric: 'tabular-nums', flexShrink: 0, fontWeight: urgent ? 700 : 500 }}
        >
          {expired ? `+${Math.floor(overdue) < 60 ? Math.floor(overdue) + 's' : Math.floor(overdue / 60) + 'm'}` : fmtRemaining(timer.remaining_seconds)}
        </span>
        <button
          onClick={() => removeTimer(timer.id).catch(() => {})}
          title="Remove this timer"
          style={{
            background: 'none', border: 'none', cursor: 'pointer', padding: 0,
            color: 'var(--color-muted)', display: 'flex', alignItems: 'center',
            flexShrink: 0, lineHeight: 0,
          }}
        >
          <X size={11} />
        </button>
      </div>
    </div>
  )
}

export default function CustomTimerPanel({
  defaultX = 24,
  defaultY = 24,
  defaultWidth = 304,
  defaultHeight = 336,
  snapGridSize,
  onLayoutChange,
}: CustomTimerPanelProps): React.ReactElement {
  const [state, setState] = useState<TimerState | null>(null)
  const thresholds = useDisplayThresholds()
  const appearance = useTimerAppearance()
  const [newName, setNewName] = useState('')
  const [newDuration, setNewDuration] = useState('')
  const [addError, setAddError] = useState(false)
  // '' = automatic (urgency cyan→orange→red shift); a hex value pins the bar.
  const [newColor, setNewColor] = useState('')
  // Per-add alert toggle. null = follow the global default; once the user
  // clicks the bell it holds their explicit choice for subsequent adds.
  const alertPref = useCustomTimerAlertPref()
  const [alertOverride, setAlertOverride] = useState<boolean | null>(null)
  const bellOn = alertOverride ?? (alertPref?.enabled ?? false)

  useEffect(() => {
    getTimerState().then(setState).catch(() => {})
  }, [])

  const handleMessage = useCallback((msg: { type: string; data: unknown }) => {
    if (msg.type === WSEvent.OverlayTimers) setState(msg.data as TimerState)
  }, [])
  useWebSocket(handleMessage)

  const timers = (state?.timers ?? [])
    .filter((t) => t.category === 'custom')
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
    startCustomTimer(newName.trim(), secs, alerts, newColor || undefined)
      .then(() => {
        setNewName('')
        setNewDuration('')
      })
      .catch(() => setAddError(true))
  }

  const quickInputStyle: React.CSSProperties = {
    backgroundColor: 'var(--color-surface)',
    border: `1px solid ${addError ? 'rgba(248,113,113,0.7)' : 'var(--color-border)'}`,
    borderRadius: 3,
    color: 'var(--color-foreground)',
    fontSize: 11,
    padding: '2px 6px',
    outline: 'none',
    minWidth: 0,
  }

  return (
    <OverlayWindow
      title={
        <span style={{ display: 'flex', alignItems: 'center', gap: 5 }}>
          <Hourglass size={13} style={{ color: '#38bdf8' }} />
          Custom Timers
          {timers.length > 0 && (
            <span style={{ fontSize: 10, color: 'var(--color-muted)', fontWeight: 400 }}>{timers.length}</span>
          )}
        </span>
      }
      headerRight={
        <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
          <button
            onClick={() => clearTimers('custom').catch(() => {})}
            title="Clear all custom timers"
            style={{ background: 'none', border: 'none', cursor: 'pointer', padding: '1px 3px', color: 'var(--color-muted)', display: 'flex', alignItems: 'center' }}
          >
            <Trash2 size={12} />
          </button>
          {window.electron?.overlay && (
            <button
              onClick={() => window.electron.overlay.toggleCustomTimer()}
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
            <Hourglass size={28} style={{ opacity: 0.2, color: '#38bdf8' }} />
            <p style={{ fontSize: 12, margin: 0 }}>Connecting…</p>
          </div>
        ) : timers.length === 0 ? (
          <div style={{ flex: 1, display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', gap: 8, color: 'var(--color-muted)', padding: 16 }}>
            <Hourglass size={28} style={{ opacity: 0.2, color: '#38bdf8' }} />
            <p style={{ fontSize: 12, margin: 0 }}>No active timers</p>
          </div>
        ) : (
          timers.map((t) => <TimerRow key={t.id} timer={t} appearance={appearance} />)
        )}
      </div>

      {/* Quick-add form — start a manual countdown by name + duration */}
      <form
        onSubmit={handleAdd}
        style={{
          display: 'flex',
          gap: 4,
          padding: '5px 8px',
          borderTop: '1px solid var(--color-border)',
          backgroundColor: 'var(--color-surface-2)',
          flexShrink: 0,
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
        {newColor !== '' && (
          <input
            type="color"
            value={newColor}
            onChange={(e) => setNewColor(e.target.value)}
            title="Bar color for this timer"
            style={{ width: 26, height: 24, padding: 0, borderRadius: 3, border: '1px solid var(--color-border)', background: 'transparent', cursor: 'pointer', flexShrink: 0 }}
          />
        )}
        <button
          type="button"
          onClick={() => setNewColor(newColor ? '' : '#38bdf8')}
          title={newColor ? 'Custom bar color (click for automatic)' : 'Automatic bar color (click to pick a color)'}
          aria-pressed={newColor !== ''}
          style={{
            display: 'flex',
            alignItems: 'center',
            padding: '2px 6px',
            borderRadius: 3,
            border: '1px solid var(--color-border)',
            backgroundColor: newColor ? 'rgba(56,189,248,0.2)' : 'var(--color-surface)',
            color: newColor ? 'var(--color-foreground)' : 'var(--color-muted)',
            cursor: 'pointer',
            flexShrink: 0,
          }}
        >
          <Palette size={12} />
        </button>
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
            border: '1px solid var(--color-border)',
            backgroundColor: bellOn ? 'rgba(56,189,248,0.2)' : 'var(--color-surface)',
            color: bellOn ? 'var(--color-foreground)' : 'var(--color-muted)',
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
            border: '1px solid var(--color-border)',
            backgroundColor: 'rgba(56,189,248,0.2)',
            color: 'var(--color-foreground)',
            cursor: 'pointer',
            lineHeight: 1.4,
            flexShrink: 0,
          }}
        >
          Start
        </button>
      </form>
    </OverlayWindow>
  )
}
