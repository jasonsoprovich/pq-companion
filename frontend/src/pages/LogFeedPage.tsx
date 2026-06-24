import React, { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { Link } from 'react-router-dom'
import { Activity, Trash2, AlertTriangle, CheckCircle2, Circle, Search, Film, Play, Pause, Square, FolderOpen, Loader2, FileText } from 'lucide-react'
import { useWebSocket } from '../hooks/useWebSocket'
import { useLogFeed, clearLogFeed, LOG_FEED_MAX } from '../hooks/useLogFeed'
import { useReplayPrefs, type ReplayPrefs } from '../hooks/useReplayPrefs'
import {
  getLogStatus,
  listReplayFiles,
  getReplayInfo,
  getReplayStatus,
  startReplay,
  pauseReplay,
  resumeReplay,
  stopReplay,
  setLogRawFeed,
  browseLog,
  type ReplayFile,
  type ReplayStatus,
  type LogBrowseLine,
} from '../services/api'
import type { LogTailerStatus } from '../types/logEvent'

// Page size for the out-of-game log browser.
const BROWSE_LIMIT = 300

// Event-type options for the browse filter. Values are exact backend event
// types; 'log:raw' is the catch-all for lines that match no known pattern.
const BROWSE_TYPE_OPTIONS: { value: string; label: string }[] = [
  { value: '', label: 'All types' },
  { value: 'log:combat_hit', label: 'Hits' },
  { value: 'log:combat_miss', label: 'Misses' },
  { value: 'log:spell_cast', label: 'Casts' },
  { value: 'log:spell_landed', label: 'Spell landed' },
  { value: 'log:spell_resist', label: 'Resists' },
  { value: 'log:spell_fade', label: 'Fades' },
  { value: 'log:heal', label: 'Heals' },
  { value: 'log:death', label: 'Deaths' },
  { value: 'log:zone', label: 'Zones' },
  { value: 'log:raw', label: 'Other / chat' },
]

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
  'log:spell_landed':    { label: 'Landed',    color: '#8b5cf6' }, // violet
  'log:raw':             { label: 'Log',       color: '#475569' }, // slate
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
function ReplayPanel({
  status,
  prefs,
  onPrefsChange,
}: {
  status: ReplayStatus
  prefs: ReplayPrefs
  onPrefsChange: (patch: Partial<ReplayPrefs>) => void
}): React.ReactElement {
  const [files, setFiles] = useState<ReplayFile[]>([])
  const [error, setError] = useState<string | null>(null)
  const { file, fromStr, toStr, speed } = prefs

  useEffect(() => {
    listReplayFiles()
      .then(setFiles)
      .catch((err: Error) => setError(err.message))
  }, [])

  // Selecting a file probes its first/last timestamps and pre-fills the range.
  const handleFileChange = (name: string): void => {
    onPrefsChange({ file: name })
    setError(null)
    if (!name) return
    getReplayInfo(name)
      .then((info) => {
        onPrefsChange({ fromStr: toLocalInput(info.first), toStr: toLocalInput(info.last) })
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
          onChange={(e) => onPrefsChange({ fromStr: e.target.value })}
          disabled={active || !file}
          title="Replay start point"
          style={inputStyle}
        />
        <span className="text-xs" style={{ color: 'var(--color-muted)' }}>→</span>
        <input
          type="datetime-local"
          step={1}
          value={toStr}
          onChange={(e) => onPrefsChange({ toStr: e.target.value })}
          disabled={active || !file}
          title="Replay end point"
          style={inputStyle}
        />
        <select
          value={speed}
          onChange={(e) => onPrefsChange({ speed: Number(e.target.value) })}
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

// ── Browse panel ─────────────────────────────────────────────────────────────

/**
 * BrowsePanel — file + event-type selectors for the out-of-game log viewer.
 * The header search box drives the text filter; this strip picks which log
 * file to read and (optionally) narrows to one event type. Read-only: it never
 * touches the live pipeline or the file.
 */
function BrowsePanel({
  files,
  file,
  onFileChange,
  type,
  onTypeChange,
  count,
  loading,
}: {
  files: ReplayFile[]
  file: string
  onFileChange: (name: string) => void
  type: string
  onTypeChange: (t: string) => void
  count: number
  loading: boolean
}): React.ReactElement {
  const inputStyle: React.CSSProperties = {
    background: 'var(--color-background)',
    border: '1px solid var(--color-border)',
    borderRadius: 4,
    color: 'var(--color-foreground)',
    fontSize: 11,
    padding: '3px 6px',
    outline: 'none',
  }
  return (
    <div
      className="shrink-0 border-b px-4 py-2"
      style={{ borderColor: 'var(--color-border)', backgroundColor: 'var(--color-surface)' }}
    >
      <div className="flex flex-wrap items-center gap-2">
        <FolderOpen size={12} style={{ color: 'var(--color-primary)' }} />
        <span className="text-xs font-semibold" style={{ color: 'var(--color-foreground)' }}>
          Browse
        </span>
        <select
          value={file}
          onChange={(e) => onFileChange(e.target.value)}
          style={{ ...inputStyle, maxWidth: 260 }}
        >
          <option value="">Select a character log…</option>
          {files.map((f) => (
            <option key={f.name} value={f.name}>
              {f.character} ({fmtBytes(f.size_bytes)})
            </option>
          ))}
        </select>
        <select
          value={type}
          onChange={(e) => onTypeChange(e.target.value)}
          disabled={!file}
          title="Filter by event type"
          style={inputStyle}
        >
          {BROWSE_TYPE_OPTIONS.map((o) => (
            <option key={o.value} value={o.value}>{o.label}</option>
          ))}
        </select>
        {loading && <Loader2 size={12} className="animate-spin" style={{ color: 'var(--color-muted)' }} />}
        {file && !loading && (
          <span className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
            {count} line{count === 1 ? '' : 's'} loaded
          </span>
        )}
      </div>
      <p className="mt-1 text-[10px]" style={{ color: 'var(--color-muted)' }}>
        Read any saved character log while the game is closed. Type in the search box to filter, scroll to load older lines. Files are never modified.
      </p>
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
  const [replayPrefs, patchReplayPrefs] = useReplayPrefs()
  const feedRef = useRef<HTMLDivElement>(null)
  const atBottomRef = useRef(true)

  // Live feed vs. out-of-game browser. Browse reads a chosen saved log file on
  // demand via /api/log/browse; the live feed is fed only by the WebSocket.
  const [mode, setMode] = useState<'live' | 'browse'>('live')
  const [browseFiles, setBrowseFiles] = useState<ReplayFile[]>([])
  const [browseFile, setBrowseFile] = useState('')
  const [browseType, setBrowseType] = useState('')
  const [browseLines, setBrowseLines] = useState<LogBrowseLine[]>([])
  const [browseNext, setBrowseNext] = useState<number | null>(null)
  const [browseLoading, setBrowseLoading] = useState(false)
  const [browseError, setBrowseError] = useState<string | null>(null)
  // Debounced search term so typing doesn't fire a request per keystroke.
  const [debouncedSearch, setDebouncedSearch] = useState('')

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

  // Auto-scroll to top when new events arrive (live feed is newest-first).
  useEffect(() => {
    if (mode === 'live' && feedRef.current && atBottomRef.current) {
      feedRef.current.scrollTop = 0
    }
  }, [events, mode])

  // Debounce the search box (drives the browse query and live filter).
  useEffect(() => {
    const id = setTimeout(() => setDebouncedSearch(search.trim()), 250)
    return () => clearTimeout(id)
  }, [search])

  // Load the candidate file list the first time Browse is opened, defaulting
  // to the most recently played character.
  useEffect(() => {
    if (mode !== 'browse' || browseFiles.length > 0) return
    listReplayFiles()
      .then((fs) => {
        setBrowseFiles(fs)
        setBrowseFile((cur) => cur || (fs[0]?.name ?? ''))
      })
      .catch((err: Error) => setBrowseError(err.message))
  }, [mode, browseFiles.length])

  // (Re)load the first page whenever the file, type filter, or search changes.
  useEffect(() => {
    if (mode !== 'browse' || !browseFile) {
      setBrowseLines([])
      setBrowseNext(null)
      return
    }
    let cancelled = false
    setBrowseLoading(true)
    setBrowseError(null)
    browseLog({
      file: browseFile,
      q: debouncedSearch || undefined,
      type: browseType || undefined,
      limit: BROWSE_LIMIT,
    })
      .then((res) => {
        if (cancelled) return
        setBrowseLines(res.lines)
        setBrowseNext(res.next_offset)
        if (feedRef.current) feedRef.current.scrollTop = 0
      })
      .catch((err: Error) => {
        if (!cancelled) setBrowseError(err.message)
      })
      .finally(() => {
        if (!cancelled) setBrowseLoading(false)
      })
    return () => {
      cancelled = true
    }
  }, [mode, browseFile, browseType, debouncedSearch])

  // Fetch the next (older) page and append. Called on scroll near the bottom.
  const loadMoreBrowse = useCallback(() => {
    if (browseLoading || browseNext == null || !browseFile) return
    setBrowseLoading(true)
    browseLog({
      file: browseFile,
      q: debouncedSearch || undefined,
      type: browseType || undefined,
      beforeOffset: browseNext,
      limit: BROWSE_LIMIT,
    })
      .then((res) => {
        setBrowseLines((prev) => [...prev, ...res.lines])
        setBrowseNext(res.next_offset)
      })
      .catch((err: Error) => setBrowseError(err.message))
      .finally(() => setBrowseLoading(false))
  }, [browseLoading, browseNext, browseFile, debouncedSearch, browseType])

  function handleScroll(): void {
    if (!feedRef.current) return
    if (mode === 'browse') {
      // Older lines load at the bottom; fetch more as the user nears it.
      const el = feedRef.current
      if (el.scrollHeight - el.scrollTop - el.clientHeight < 200) {
        loadMoreBrowse()
      }
      return
    }
    atBottomRef.current = feedRef.current.scrollTop < 40
  }

  // Opt-in raw passthrough: when on, the backend also pushes unrecognised
  // lines (chat, system messages, "X is no longer mezzed") to the live feed so
  // they're visible and searchable. The flag lives on the backend; optimistic
  // toggle, revert on failure.
  const rawFeed = status?.raw_feed ?? false
  const handleToggleRawFeed = useCallback(() => {
    const next = !(status?.raw_feed ?? false)
    setStatus((prev) => (prev ? { ...prev, raw_feed: next } : prev))
    setLogRawFeed(next).catch(() => {
      setStatus((prev) => (prev ? { ...prev, raw_feed: !next } : prev))
    })
  }, [status?.raw_feed])

  // Stage the clicked Browse row as the replay start (and probe the file's end
  // for a sensible range), open the Replay panel, and auto-start when idle. If
  // a replay is already running we only stage the selection so the user can
  // stop and re-play from here without losing their place.
  //
  // Browse-only by design: replay drives the full pipeline (triggers, timers,
  // overlays) and pauses live tailing, so it must never be triggered from the
  // live feed while the game is running.
  const handlePlayFrom = useCallback((timestamp: string) => {
    if (!browseFile) return
    const fromStr = toLocalInput(timestamp)
    setMode('live')
    setShowReplay(true)
    patchReplayPrefs({ file: browseFile, fromStr })
    getReplayInfo(browseFile)
      .then((info) => patchReplayPrefs({ toStr: toLocalInput(info.last) }))
      .catch(() => {})
    if (replayStatus.state === 'idle') {
      startReplay({ file: browseFile, from: new Date(fromStr).toISOString(), speed: replayPrefs.speed })
        .catch(() => {})
    }
  }, [browseFile, patchReplayPrefs, replayStatus.state, replayPrefs.speed])

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
            {mode === 'browse'
              ? `${browseLines.length}${browseNext != null ? '+' : ''} lines`
              : search
              ? `${visibleEvents.length} / ${events.length}`
              : `${events.length} / ${LOG_FEED_MAX}`}
          </span>
        </div>
        <div className="flex items-center gap-2">
          {/* Live / Browse mode toggle */}
          <div
            className="flex items-center rounded text-xs"
            style={{ border: '1px solid var(--color-border)', overflow: 'hidden' }}
          >
            {(['live', 'browse'] as const).map((m) => (
              <button
                key={m}
                onClick={() => setMode(m)}
                className="px-2 py-1 transition-colors"
                style={{
                  backgroundColor: mode === m ? 'var(--color-primary)' : 'transparent',
                  color: mode === m ? 'var(--color-background)' : 'var(--color-muted)',
                }}
                title={m === 'live' ? 'Live log feed (game running)' : 'Browse a saved log file (game closed)'}
              >
                {m === 'live' ? 'Live' : 'Browse'}
              </button>
            ))}
          </div>
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
              placeholder={mode === 'browse' ? 'Search log…' : 'Filter events…'}
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
          {mode === 'live' && <ConnPill state={wsState} status={status} />}
          {mode === 'live' && (
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
          )}
          {mode === 'live' && (
            <button
              onClick={() => { clearLogFeed(); setSearch('') }}
              className="flex items-center gap-1.5 rounded px-2 py-1 text-xs transition-colors"
              style={{ color: 'var(--color-muted)', border: '1px solid var(--color-border)' }}
              title="Clear events"
            >
              <Trash2 size={12} />
              Clear
            </button>
          )}
        </div>
      </div>

      {/* Tailer status + raw-lines toggle (live only) */}
      {mode === 'live' && (
        <div
          className="flex shrink-0 items-center gap-2 border-b px-4 py-2"
          style={{ borderColor: 'var(--color-border)' }}
        >
          <div className="min-w-0 flex-1">
            <StatusBar status={status} />
          </div>
          <button
            onClick={handleToggleRawFeed}
            className="flex shrink-0 items-center gap-1.5 rounded px-2 py-1 text-xs transition-colors"
            style={{
              color: rawFeed ? 'var(--color-primary)' : 'var(--color-muted)',
              backgroundColor: rawFeed ? 'var(--color-surface-2)' : 'transparent',
              border: '1px solid var(--color-border)',
            }}
            title="Also show raw, unrecognised lines (chat, system messages, e.g. “… is no longer mezzed”) in the feed so they can be searched here"
          >
            <FileText size={12} />
            Raw lines{rawFeed ? ': on' : ''}
          </button>
        </div>
      )}

      {/* Replay controls (live only) */}
      {mode === 'live' && showReplay && (
        <ReplayPanel
          status={replayStatus}
          prefs={replayPrefs}
          onPrefsChange={patchReplayPrefs}
        />
      )}

      {/* Browse controls */}
      {mode === 'browse' && (
        <BrowsePanel
          files={browseFiles}
          file={browseFile}
          onFileChange={setBrowseFile}
          type={browseType}
          onTypeChange={setBrowseType}
          count={browseLines.length}
          loading={browseLoading}
        />
      )}

      {/* Event feed */}
      <div
        ref={feedRef}
        className="flex-1 overflow-y-auto"
        onScroll={handleScroll}
        style={{ backgroundColor: 'var(--color-background)' }}
      >
        {mode === 'browse' ? (
          <BrowseBody
            file={browseFile}
            lines={browseLines}
            loading={browseLoading}
            error={browseError}
            hasMore={browseNext != null}
            search={debouncedSearch}
            onPickLive={() => setMode('live')}
            onPlayFrom={browseFile ? handlePlayFrom : undefined}
          />
        ) : events.length === 0 ? (
          <div className="flex flex-col items-center justify-center gap-3 py-20">
            <Activity size={32} style={{ color: 'var(--color-muted)' }} />
            <p className="text-sm" style={{ color: 'var(--color-muted)' }}>
              Waiting for log events…
            </p>
            <p className="max-w-xs text-center text-xs" style={{ color: 'var(--color-muted)' }}>
              Make sure <strong>Parse Combat Log</strong> is enabled in Settings and EQ is running.
            </p>
            <button
              onClick={() => setMode('browse')}
              className="mt-1 flex items-center gap-1.5 rounded px-3 py-1.5 text-xs"
              style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-foreground)', border: '1px solid var(--color-border)' }}
            >
              <FolderOpen size={12} />
              Browse a saved log instead
            </button>
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

// BrowseBody renders the out-of-game browse results, empty states, and the
// end-of-file / loading indicators.
function BrowseBody({
  file,
  lines,
  loading,
  error,
  hasMore,
  search,
  onPickLive,
  onPlayFrom,
}: {
  file: string
  lines: LogBrowseLine[]
  loading: boolean
  error: string | null
  hasMore: boolean
  search: string
  onPickLive: () => void
  onPlayFrom?: (timestamp: string) => void
}): React.ReactElement {
  if (error) {
    return (
      <div className="flex flex-col items-center justify-center gap-2 py-20">
        <AlertTriangle size={28} style={{ color: 'var(--color-danger)' }} />
        <p className="text-sm" style={{ color: 'var(--color-danger)' }}>{error}</p>
      </div>
    )
  }
  if (!file) {
    return (
      <div className="flex flex-col items-center justify-center gap-3 py-20">
        <FolderOpen size={32} style={{ color: 'var(--color-muted)' }} />
        <p className="text-sm" style={{ color: 'var(--color-muted)' }}>
          Pick a character log above to start browsing.
        </p>
        <button onClick={onPickLive} className="text-xs underline" style={{ color: 'var(--color-primary)' }}>
          Back to live feed
        </button>
      </div>
    )
  }
  if (lines.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center gap-3 py-20">
        {loading ? (
          <>
            <Loader2 size={28} className="animate-spin" style={{ color: 'var(--color-muted)' }} />
            <p className="text-sm" style={{ color: 'var(--color-muted)' }}>Reading log…</p>
          </>
        ) : (
          <>
            <Search size={28} style={{ color: 'var(--color-muted)' }} />
            <p className="text-sm" style={{ color: 'var(--color-muted)' }}>
              {search ? `No lines match "${search}"` : 'No readable lines in this log.'}
            </p>
          </>
        )}
      </div>
    )
  }
  return (
    <>
      <table className="w-full text-xs" style={{ borderCollapse: 'collapse' }}>
        <tbody>
          {lines.map((ev, i) => (
            <EventRow key={i} ev={ev} onPlayFrom={onPlayFrom} />
          ))}
        </tbody>
      </table>
      <div className="flex items-center justify-center gap-2 py-3 text-[11px]" style={{ color: 'var(--color-muted)' }}>
        {loading ? (
          <>
            <Loader2 size={12} className="animate-spin" />
            Loading older lines…
          </>
        ) : hasMore ? (
          'Scroll for older lines'
        ) : (
          'Start of log reached'
        )}
      </div>
    </>
  )
}

// EventRow renders one line for both the live feed and the browse view, so it
// accepts the minimal shared shape rather than the narrow LogEvent union.
// onPlayFrom, when provided, shows a hover play button that replays from this
// line's timestamp.
function EventRow({
  ev,
  onPlayFrom,
}: {
  ev: { type: string; timestamp: string; message: string }
  onPlayFrom?: (timestamp: string) => void
}): React.ReactElement {
  const ts = new Date(ev.timestamp)
  const timeStr = ts.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })

  return (
    <tr
      className="group border-b"
      style={{ borderColor: 'var(--color-border)' }}
    >
      {/* Play-from-here (appears on row hover) */}
      {onPlayFrom && (
        <td className="w-7 pl-2" style={{ verticalAlign: 'middle' }}>
          <button
            onClick={() => onPlayFrom(ev.timestamp)}
            title="Replay from this line"
            className="flex items-center justify-center rounded opacity-0 transition-opacity group-hover:opacity-100"
            style={{ width: 18, height: 18, color: 'var(--color-primary)' }}
            onMouseEnter={(e) => (e.currentTarget.style.background = 'var(--color-surface-2)')}
            onMouseLeave={(e) => (e.currentTarget.style.background = 'transparent')}
          >
            <Play size={12} />
          </button>
        </td>
      )}

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
