import React, { useCallback, useEffect, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { Activity, Bell, Trash2, AlertTriangle, CheckCircle2, Circle } from 'lucide-react'
import { useWebSocket } from '../hooks/useWebSocket'
import { getLogStatus } from '../services/api'
import EventAlertsPanel from '../components/EventAlertsPanel'
import type { LogEvent, LogTailerStatus } from '../types/logEvent'

const MAX_EVENTS = 200

// ── Event badge colours ────────────────────────────────────────────────────────

const TYPE_META: Record<
  string,
  { label: string; color: string }
> = {
  'log:zone':            { label: 'Zone',      color: '#3b82f6' }, // blue
  'log:combat_hit':      { label: 'Hit',       color: '#ef4444' }, // red
  'log:combat_miss':     { label: 'Miss',      color: '#6b7280' }, // gray
  'log:spell_cast':      { label: 'Cast',      color: '#a855f7' }, // purple
  'log:spell_interrupt': { label: 'Interrupt', color: '#f97316' }, // orange
  'log:spell_resist':    { label: 'Resist',    color: '#f97316' }, // orange
  'log:spell_fade':      { label: 'Fade',      color: '#14b8a6' }, // teal
  'log:death':           { label: 'Death',     color: '#dc2626' }, // dark red
}

function EventBadge({ type }: { type: string }): React.ReactElement {
  const meta = TYPE_META[type] ?? { label: type, color: '#6b7280' }
  return (
    <span
      className="shrink-0 rounded px-1.5 py-0.5 text-[10px] font-semibold uppercase tracking-wider text-white"
      style={{ backgroundColor: meta.color }}
    >
      {meta.label}
    </span>
  )
}

// ── Status bar ─────────────────────────────────────────────────────────────────

function StatusBar({ status }: { status: LogTailerStatus | null }): React.ReactElement {
  if (!status) {
    return (
      <div
        className="flex items-center gap-2 rounded px-3 py-2 text-xs"
        style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-muted)' }}
      >
        <Circle size={10} />
        Loading status…
      </div>
    )
  }

  if (!status.enabled) {
    return (
      <div
        className="flex items-center gap-2 rounded px-3 py-2 text-xs"
        style={{ backgroundColor: 'var(--color-surface-2)', color: '#f97316' }}
      >
        <AlertTriangle size={12} />
        Log parsing is disabled. Enable it in{' '}
        <Link to="/settings" className="underline" style={{ color: 'var(--color-primary)' }}>
          Settings
        </Link>
        .
      </div>
    )
  }

  if (!status.file_exists) {
    return (
      <div
        className="flex items-center gap-2 rounded px-3 py-2 text-xs"
        style={{ backgroundColor: 'var(--color-surface-2)', color: '#f97316' }}
      >
        <AlertTriangle size={12} />
        Log file not found:{' '}
        <span className="font-mono" style={{ color: 'var(--color-muted-foreground)' }}>
          {status.file_path || '(not configured)'}
        </span>
      </div>
    )
  }

  return (
    <div
      className="flex items-center gap-2 rounded px-3 py-2 text-xs"
      style={{ backgroundColor: 'var(--color-surface-2)', color: '#22c55e' }}
    >
      <CheckCircle2 size={12} />
      <span>Tailing</span>
      <span className="font-mono" style={{ color: 'var(--color-muted)' }}>
        {status.file_path}
      </span>
    </div>
  )
}

// ── Connection pill ────────────────────────────────────────────────────────────

function ConnPill({
  state,
  status,
}: {
  state: string
  status: LogTailerStatus | null
}): React.ReactElement {
  let color: string
  let label: string
  if (state !== 'open') {
    color = state === 'connecting' ? '#f97316' : '#6b7280'
    label = state === 'connecting' ? 'Connecting…' : 'Disconnected'
  } else if (!status || !status.enabled || !status.file_exists) {
    color = '#f97316'
    label = 'No Log'
  } else {
    color = '#22c55e'
    label = 'Live'
  }
  return (
    <span className="flex items-center gap-1.5 text-xs" style={{ color }}>
      <span
        className="inline-block h-2 w-2 rounded-full"
        style={{ backgroundColor: color }}
      />
      {label}
    </span>
  )
}

// ── Page ───────────────────────────────────────────────────────────────────────

export default function LogFeedPage(): React.ReactElement {
  const [events, setEvents] = useState<LogEvent[]>([])
  const [status, setStatus] = useState<LogTailerStatus | null>(null)
  const [showAlerts, setShowAlerts] = useState(false)
  const feedRef = useRef<HTMLDivElement>(null)
  const atBottomRef = useRef(true)

  // Load tailer status once on mount.
  useEffect(() => {
    getLogStatus()
      .then(setStatus)
      .catch(() => setStatus(null))
  }, [])

  const handleMessage = useCallback((msg: { type: string; data: unknown }) => {
    // Only handle log:* events.
    if (!msg.type.startsWith('log:')) return
    const ev = msg.data as LogEvent
    setEvents((prev) => {
      const next = [ev, ...prev]
      return next.length > MAX_EVENTS ? next.slice(0, MAX_EVENTS) : next
    })
  }, [])

  const wsState = useWebSocket(handleMessage)

  // Auto-scroll to top when new events arrive (feed is newest-first).
  useEffect(() => {
    if (feedRef.current && atBottomRef.current) {
      feedRef.current.scrollTop = 0
    }
  }, [events])

  function handleScroll(): void {
    if (!feedRef.current) return
    atBottomRef.current = feedRef.current.scrollTop < 40
  }

  return (
    <div className="flex h-full flex-col overflow-hidden" style={{ position: 'relative' }}>
      {/* Header */}
      <div
        className="flex shrink-0 items-center justify-between border-b px-4 py-3"
        style={{ borderColor: 'var(--color-border)' }}
      >
        <div className="flex items-center gap-2">
          <Activity size={18} style={{ color: 'var(--color-primary)' }} />
          <h1 className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>
            Log Feed
          </h1>
          <span
            className="rounded px-1.5 py-0.5 text-[10px]"
            style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-muted)' }}
          >
            {events.length} / {MAX_EVENTS}
          </span>
        </div>
        <div className="flex items-center gap-3">
          <ConnPill state={wsState} status={status} />
          <button
            onClick={() => setShowAlerts((v) => !v)}
            className="flex items-center gap-1.5 rounded px-2 py-1 text-xs transition-colors"
            style={{
              color: showAlerts ? 'var(--color-primary)' : 'var(--color-muted)',
              border: `1px solid ${showAlerts ? 'var(--color-primary)' : 'var(--color-border)'}`,
            }}
            title="Event audio alerts"
          >
            <Bell size={12} />
            Alerts
          </button>
          <button
            onClick={() => setEvents([])}
            className="flex items-center gap-1.5 rounded px-2 py-1 text-xs transition-colors"
            style={{ color: 'var(--color-muted)', border: '1px solid var(--color-border)' }}
            title="Clear events"
          >
            <Trash2 size={12} />
            Clear
          </button>
        </div>
      </div>

      {/* Tailer status */}
      <div className="shrink-0 border-b px-4 py-2" style={{ borderColor: 'var(--color-border)' }}>
        <StatusBar status={status} />
      </div>

      {/* Event feed */}
      <div
        ref={feedRef}
        className="flex-1 overflow-y-auto"
        onScroll={handleScroll}
        style={{ backgroundColor: 'var(--color-background)' }}
      >
        {events.length === 0 ? (
          <div className="flex flex-col items-center justify-center gap-3 py-20">
            <Activity size={32} style={{ color: 'var(--color-muted)' }} />
            <p className="text-sm" style={{ color: 'var(--color-muted)' }}>
              Waiting for log events…
            </p>
            <p className="max-w-xs text-center text-xs" style={{ color: 'var(--color-muted)' }}>
              Make sure <strong>Parse Combat Log</strong> is enabled in Settings and EQ is running.
            </p>
          </div>
        ) : (
          <table className="w-full text-xs" style={{ borderCollapse: 'collapse' }}>
            <tbody>
              {events.map((ev, i) => (
                <EventRow key={i} ev={ev} />
              ))}
            </tbody>
          </table>
        )}
      </div>

      {/* Event alerts config panel */}
      {showAlerts && <EventAlertsPanel onClose={() => setShowAlerts(false)} />}
    </div>
  )
}

function EventRow({ ev }: { ev: LogEvent }): React.ReactElement {
  const ts = new Date(ev.timestamp)
  const timeStr = ts.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })

  return (
    <tr
      className="border-b"
      style={{
        borderColor: 'var(--color-border)',
      }}
    >
      {/* Timestamp */}
      <td
        className="w-24 shrink-0 px-3 py-1.5 font-mono text-[10px] tabular-nums"
        style={{ color: 'var(--color-muted)', verticalAlign: 'middle' }}
      >
        {timeStr}
      </td>

      {/* Type badge */}
      <td className="w-20 px-1 py-1.5" style={{ verticalAlign: 'middle' }}>
        <EventBadge type={ev.type} />
      </td>

      {/* Raw log message */}
      <td
        className="px-3 py-1.5 font-mono"
        style={{ color: 'var(--color-foreground)', verticalAlign: 'middle', wordBreak: 'break-word' }}
      >
        {ev.message}
      </td>
    </tr>
  )
}
