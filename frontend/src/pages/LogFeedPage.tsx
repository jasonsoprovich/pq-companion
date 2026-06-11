import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { Activity, Trash2, AlertTriangle, CheckCircle2, Circle, Search, Film, Play, Pause, Square } from 'lucide-react'
import { useWebSocket } from '../hooks/useWebSocket'
import { useLogFeed, clearLogFeed, LOG_FEED_MAX } from '../hooks/useLogFeed'
import {
  getLogStatus,
  listReplayFiles,
  getReplayInfo,
  getReplayStatus,
  startReplay,
  pauseReplay,
  resumeReplay,
  stopReplay,
  type ReplayFile,
  type ReplayStatus,
} from '../services/api'
import type { LogEvent, LogTailerStatus } from '../types/logEvent'

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
  'log:heal':            { label: 'Heal',      color: '#22c55e' }, // green
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

// ── Replay panel ───────────────────────────────────────────────────────────────

/** Convert an ISO timestamp into a datetime-local input value (local time). */
function toLocalInput(iso: string): string {
  const d = new Date(iso)
  const pad = (n: number): string => String(n).padStart(2, '0')
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`
}

function fmtBytes(n: number): string {
  if (n >= 1 << 30) return `${(n / (1 << 30)).toFixed(1)} GB`
  if (n >= 1 << 20) return `${(n / (1 << 20)).toFixed(1)} MB`
  return `${Math.max(1, Math.round(n / 1024))} KB`
}

/**
 * ReplayPanel — pick a log file, a start/end point, and play the segment
 * through the app's full pipeline (triggers, timers, combat meter, overlays)
 * as if the session were live. Live tailing pauses for the duration; the
 * file is read strictly read-only.
 */
function ReplayPanel({ status }: { status: ReplayStatus }): React.ReactElement {
  const [files, setFiles] = useState<ReplayFile[]>([])
  const [file, setFile] = useState('')
  const [fromStr, setFromStr] = useState('')
  const [toStr, setToStr] = useState('')
  const [speed, setSpeed] = useState(1)
  const [error, setError] = useState<string | null>(null)

  useEffect(() => {
    listReplayFiles()
      .then(setFiles)
      .catch((err: Error) => setError(err.message))
  }, [])

  // Selecting a file probes its first/last timestamps and pre-fills the range.
  const handleFileChange = (name: string): void => {
    setFile(name)
    setError(null)
    if (!name) return
    getReplayInfo(name)
      .then((info) => {
        setFromStr(toLocalInput(info.first))
        setToStr(toLocalInput(info.last))
      })
      .catch((err: Error) => setError(err.message))
  }

  const handleStart = (): void => {
    if (!file) return
    setError(null)
    startReplay({
      file,
      from: fromStr ? new Date(fromStr).toISOString() : undefined,
      to: toStr ? new Date(toStr).toISOString() : undefined,
      speed,
    }).catch((err: Error) => setError(err.message))
  }

  const active = status.state !== 'idle'
  const inputStyle: React.CSSProperties = {
    background: 'var(--color-background)',
    border: '1px solid var(--color-border)',
    borderRadius: 4,
    color: 'var(--color-foreground)',
    fontSize: 11,
    padding: '3px 6px',
    outline: 'none',
  }

  // Progress within the requested window.
  let progress = 0
  if (active && status.from && status.to) {
    const fromMs = new Date(status.from).getTime()
    const toMs = new Date(status.to).getTime()
    const posMs = status.position ? new Date(status.position).getTime() : fromMs
    if (toMs > fromMs) progress = Math.max(0, Math.min(1, (posMs - fromMs) / (toMs - fromMs)))
  }

  return (
    <div
      className="shrink-0 border-b px-4 py-2 space-y-2"
      style={{ borderColor: 'var(--color-border)', backgroundColor: 'var(--color-surface)' }}
    >
      <div className="flex flex-wrap items-center gap-2">
        <Film size={12} style={{ color: 'var(--color-primary)' }} />
        <span className="text-xs font-semibold" style={{ color: 'var(--color-foreground)' }}>
          Replay
        </span>
        <select
          value={file}
          onChange={(e) => handleFileChange(e.target.value)}
          disabled={active}
          style={{ ...inputStyle, maxWidth: 260 }}
        >
          <option value="">Select a log file…</option>
          {files.map((f) => (
            <option key={f.name} value={f.name}>
              {f.character} ({fmtBytes(f.size_bytes)})
            </option>
          ))}
        </select>
        <input
          type="datetime-local"
          step={1}
          value={fromStr}
          onChange={(e) => setFromStr(e.target.value)}
          disabled={active || !file}
          title="Replay start point"
          style={inputStyle}
        />
        <span className="text-xs" style={{ color: 'var(--color-muted)' }}>→</span>
        <input
          type="datetime-local"
          step={1}
          value={toStr}
          onChange={(e) => setToStr(e.target.value)}
          disabled={active || !file}
          title="Replay end point"
          style={inputStyle}
        />
        <select
          value={speed}
          onChange={(e) => setSpeed(Number(e.target.value))}
          disabled={active}
          title="Playback speed"
          style={inputStyle}
        >
          {[1, 2, 5, 10, 25].map((s) => (
            <option key={s} value={s}>{s}×</option>
          ))}
        </select>
        {!active && (
          <button
            onClick={handleStart}
            disabled={!file}
            className="flex items-center gap-1 rounded px-2 py-1 text-xs"
            style={{
              backgroundColor: file ? 'var(--color-primary)' : 'var(--color-surface-2)',
              color: file ? 'var(--color-background)' : 'var(--color-muted)',
            }}
          >
            <Play size={11} />
            Play
          </button>
        )}
        {status.state === 'playing' && (
          <button
            onClick={() => pauseReplay().catch(() => {})}
            className="flex items-center gap-1 rounded px-2 py-1 text-xs"
            style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-foreground)', border: '1px solid var(--color-border)' }}
          >
            <Pause size={11} />
            Pause
          </button>
        )}
        {status.state === 'paused' && (
          <button
            onClick={() => resumeReplay().catch(() => {})}
            className="flex items-center gap-1 rounded px-2 py-1 text-xs"
            style={{ backgroundColor: 'var(--color-primary)', color: 'var(--color-background)' }}
          >
            <Play size={11} />
            Resume
          </button>
        )}
        {active && (
          <button
            onClick={() => stopReplay().catch(() => {})}
            className="flex items-center gap-1 rounded px-2 py-1 text-xs"
            style={{ backgroundColor: 'var(--color-surface-2)', color: '#f87171', border: '1px solid var(--color-border)' }}
          >
            <Square size={11} />
            Stop
          </button>
        )}
      </div>

      {active && (
        <div className="space-y-1">
          <div
            style={{
              height: 5,
              borderRadius: 3,
              backgroundColor: 'var(--color-surface-2)',
              overflow: 'hidden',
            }}
          >
            <div
              style={{
                height: '100%',
                width: `${progress * 100}%`,
                backgroundColor: 'var(--color-primary)',
                transition: 'width 1s linear',
              }}
            />
          </div>
          <p className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
            {status.state === 'paused' ? 'Paused' : 'Replaying'} {status.file}
            {status.position && ` — ${new Date(status.position).toLocaleString()}`}
            {` · ${status.lines_emitted} lines · ${status.speed ?? 1}× — live log parsing is paused`}
          </p>
        </div>
      )}
      {!active && (
        <p className="text-[10px]" style={{ color: 'var(--color-muted)' }}>
          Plays a historical log segment through the whole app — triggers, spell timers, combat
          meter, and overlays react as if live. Use it to test and debug triggers against real
          gameplay (best while not in game; live parsing pauses during playback). Files are never modified.
        </p>
      )}
      {error && (
        <p className="text-[11px]" style={{ color: 'var(--color-danger)' }}>{error}</p>
      )}
    </div>
  )
}

// ── Page ───────────────────────────────────────────────────────────────────────

export default function LogFeedPage(): React.ReactElement {
  // Events live in a module-level store that the top-level
  // useLogFeedSubscriber keeps populating; navigating tabs no longer clears
  // them. The page reads via useSyncExternalStore so we still re-render
  // when new events land.
  const events = useLogFeed()
  const [status, setStatus] = useState<LogTailerStatus | null>(null)
  const [search, setSearch] = useState('')
  const [showReplay, setShowReplay] = useState(false)
  const [replayStatus, setReplayStatus] = useState<ReplayStatus>({ state: 'idle', lines_emitted: 0 })
  const feedRef = useRef<HTMLDivElement>(null)
  const atBottomRef = useRef(true)

  // Load tailer + replay status once on mount.
  useEffect(() => {
    getLogStatus()
      .then(setStatus)
      .catch(() => setStatus(null))
    getReplayStatus()
      .then((st) => {
        setReplayStatus(st)
        if (st.state !== 'idle') setShowReplay(true)
      })
      .catch(() => {})
  }, [])

  // Log events themselves are handled by the top-level subscriber; this
  // handler only tracks replay status pushes (and the hook supplies the
  // connection-state pill).
  const handleMessage = useCallback((msg: { type: string; data: unknown }) => {
    if (msg.type === 'replay:status') {
      setReplayStatus(msg.data as ReplayStatus)
    }
  }, [])
  const wsState = useWebSocket(handleMessage)

  const visibleEvents = useMemo(() => {
    const q = search.trim().toLowerCase()
    if (!q) return events
    return events.filter(
      (ev) =>
        ev.message.toLowerCase().includes(q) ||
        ev.type.toLowerCase().includes(q)
    )
  }, [events, search])

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
            {search ? `${visibleEvents.length} / ${events.length}` : `${events.length} / ${LOG_FEED_MAX}`}
          </span>
        </div>
        <div className="flex items-center gap-2">
          {/* Search input */}
          <div style={{ position: 'relative' }}>
            <Search
              size={11}
              style={{
                position: 'absolute',
                left: 7,
                top: '50%',
                transform: 'translateY(-50%)',
                color: 'var(--color-muted)',
                pointerEvents: 'none',
              }}
            />
            <input
              type="text"
              placeholder="Filter events…"
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              style={{
                paddingLeft: 24,
                paddingRight: 8,
                paddingTop: 4,
                paddingBottom: 4,
                fontSize: 11,
                width: 160,
                background: 'var(--color-background)',
                border: '1px solid var(--color-border)',
                borderRadius: 4,
                color: 'var(--color-foreground)',
                outline: 'none',
              }}
            />
          </div>
          <ConnPill state={wsState} status={status} />
          <button
            onClick={() => setShowReplay((v) => !v)}
            className="flex items-center gap-1.5 rounded px-2 py-1 text-xs transition-colors"
            style={{
              color: showReplay || replayStatus.state !== 'idle' ? 'var(--color-primary)' : 'var(--color-muted)',
              border: '1px solid var(--color-border)',
            }}
            title="Replay a historical log segment through the whole app (triggers, timers, combat)"
          >
            <Film size={12} />
            Replay
          </button>
          <button
            onClick={() => { clearLogFeed(); setSearch('') }}
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

      {/* Replay controls */}
      {showReplay && <ReplayPanel status={replayStatus} />}

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
        ) : visibleEvents.length === 0 ? (
          <div className="flex flex-col items-center justify-center gap-3 py-20">
            <Search size={32} style={{ color: 'var(--color-muted)' }} />
            <p className="text-sm" style={{ color: 'var(--color-muted)' }}>
              No events match "{search}"
            </p>
          </div>
        ) : (
          <table className="w-full text-xs" style={{ borderCollapse: 'collapse' }}>
            <tbody>
              {visibleEvents.map((ev, i) => (
                <EventRow key={i} ev={ev} />
              ))}
            </tbody>
          </table>
        )}
      </div>

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
