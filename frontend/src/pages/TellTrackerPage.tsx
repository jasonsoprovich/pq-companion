import React, { useCallback, useEffect, useRef, useState } from 'react'
import {
  MessageSquare, RefreshCw, Trash2, AlertCircle, X, ArrowUp, ArrowDown,
  ChevronRight, ChevronDown, ScanLine, Search,
} from 'lucide-react'
import {
  listTellConversations, getTellThread, deleteTellPeer, clearTells, scanTells,
  listTellCharacters,
} from '../services/api'
import type { Tell, TellConversation } from '../types/tell'
import { useWebSocket } from '../hooks/useWebSocket'
import { WSEvent } from '../lib/wsEvents'
import { useEscapeToClose } from '../hooks/useEscapeToClose'

function formatRelative(unix: number): string {
  if (!unix) return '—'
  const diffMs = Date.now() - unix * 1000
  if (diffMs < 60_000) return 'just now'
  const mins = Math.floor(diffMs / 60_000)
  if (mins < 60) return `${mins}m ago`
  const hours = Math.floor(mins / 60)
  if (hours < 24) return `${hours}h ago`
  const days = Math.floor(hours / 24)
  if (days < 30) return `${days}d ago`
  return new Date(unix * 1000).toLocaleDateString()
}

function formatTimestamp(unix: number): string {
  if (!unix) return ''
  return new Date(unix * 1000).toLocaleString([], {
    month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit',
  })
}

// One expandable conversation: a peer header that toggles open to reveal the
// full message thread (lazily loaded the first time it's opened).
function ConversationRow({
  convo,
  character,
  expanded,
  onToggle,
  onDeleted,
}: {
  convo: TellConversation
  character: string
  expanded: boolean
  onToggle: () => void
  onDeleted: () => void
}): React.ReactElement {
  const [thread, setThread] = useState<Tell[] | null>(null)
  const [err, setErr] = useState<string | null>(null)

  const loadThread = useCallback(() => {
    getTellThread(convo.peer, { sort: 'asc', character: character || undefined })
      .then((r) => setThread(r.messages))
      .catch((e: Error) => setErr(e.message))
  }, [convo.peer, character])

  useEffect(() => {
    if (expanded && thread === null) loadThread()
  }, [expanded, thread, loadThread])

  // When new tells arrive for an already-open thread, the parent bumps last_ts;
  // refetch so the open thread stays current.
  useEffect(() => {
    if (expanded) loadThread()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [convo.last_ts])

  async function handleDelete(e: React.MouseEvent) {
    e.stopPropagation()
    try {
      await deleteTellPeer(convo.peer, character || undefined)
      onDeleted()
    } catch {
      // best-effort
    }
  }

  return (
    <div
      className="rounded-lg border"
      style={{ backgroundColor: 'var(--color-surface)', borderColor: 'var(--color-border)' }}
    >
      <button
        onClick={onToggle}
        className="flex w-full items-center gap-2 px-3 py-2 text-left"
        style={{ background: 'transparent', border: 'none', cursor: 'pointer' }}
      >
        {expanded ? (
          <ChevronDown size={14} style={{ color: 'var(--color-muted)' }} />
        ) : (
          <ChevronRight size={14} style={{ color: 'var(--color-muted)' }} />
        )}
        <span className="text-sm font-semibold" style={{ color: 'var(--color-primary)' }}>
          {convo.peer}
        </span>
        <span className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
          {convo.count} message{convo.count === 1 ? '' : 's'}
        </span>
        <span className="mx-2 flex-1 truncate text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
          <span style={{ color: 'var(--color-muted)' }}>
            {convo.last_direction === 'out' ? 'You: ' : `${convo.peer}: `}
          </span>
          {convo.last_message}
        </span>
        <span className="text-[11px] tabular-nums" style={{ color: 'var(--color-muted)' }}>
          {formatRelative(convo.last_ts)}
        </span>
        <span
          onClick={handleDelete}
          title="Delete this conversation"
          className="rounded p-1"
          style={{ color: 'var(--color-danger)' }}
        >
          <Trash2 size={13} />
        </span>
      </button>

      {expanded && (
        <div
          className="border-t px-3 py-3 flex flex-col gap-2.5"
          style={{ borderColor: 'var(--color-border)', backgroundColor: 'var(--color-background)' }}
        >
          {err && (
            <p className="text-xs" style={{ color: 'var(--color-danger)' }}>{err}</p>
          )}
          {!err && thread === null && (
            <p className="text-xs" style={{ color: 'var(--color-muted)' }}>Loading…</p>
          )}
          {thread !== null && thread.length === 0 && (
            <p className="text-xs" style={{ color: 'var(--color-muted)' }}>No messages.</p>
          )}
          {thread !== null && thread.map((m) => {
            const out = m.direction === 'out'
            return (
              <div key={m.id} className="flex flex-col" style={{ alignItems: out ? 'flex-end' : 'flex-start' }}>
                {/* Sender label, sitting above the bubble like a chat header */}
                <span
                  className="mb-0.5 px-1 text-[10px] font-semibold uppercase tracking-wide"
                  style={{ color: out ? 'var(--color-primary)' : 'var(--color-muted-foreground)' }}
                >
                  {out ? 'You' : m.peer}
                </span>
                {/* Message bubble — indented to ~80% width and aligned by sender */}
                <div
                  className="rounded-2xl px-3 py-2"
                  style={{
                    maxWidth: '80%',
                    backgroundColor: out ? 'var(--color-primary)' : 'var(--color-surface-2)',
                    color: out ? '#fff' : 'var(--color-foreground)',
                    borderBottomRightRadius: out ? 4 : undefined,
                    borderBottomLeftRadius: out ? undefined : 4,
                  }}
                >
                  <span className="text-xs leading-relaxed" style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>
                    {m.message}
                  </span>
                </div>
                {/* Timestamp / zone footnote under the bubble */}
                <span className="mt-0.5 px-1 text-[10px] tabular-nums" style={{ color: 'var(--color-muted)' }}>
                  {formatTimestamp(m.ts)}{m.zone ? ` · ${m.zone}` : ''}
                </span>
              </div>
            )
          })}
        </div>
      )}
    </div>
  )
}

export default function TellTrackerPage(): React.ReactElement {
  const [convos, setConvos] = useState<TellConversation[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [search, setSearch] = useState('')
  const [sortDir, setSortDir] = useState<'asc' | 'desc'>('desc')
  const [expanded, setExpanded] = useState<Set<string>>(new Set())
  const [confirmClearOpen, setConfirmClearOpen] = useState(false)
  const [scanModalOpen, setScanModalOpen] = useState(false)
  const [scanning, setScanning] = useState(false)
  const [scanResult, setScanResult] = useState<string | null>(null)
  const [characters, setCharacters] = useState<string[]>([])
  const [selectedChar, setSelectedChar] = useState<string>('')

  // Each character has its own log file and so its own conversations. Load the
  // tab set once; default the selection to the active in-game character.
  useEffect(() => {
    listTellCharacters()
      .then((r) => {
        setCharacters(r.characters)
        setSelectedChar((cur) => {
          if (cur && r.characters.includes(cur)) return cur
          if (r.active && r.characters.includes(r.active)) return r.active
          return r.characters[0] ?? r.active ?? ''
        })
      })
      .catch(() => { /* tabs are best-effort; fall back to all-characters view */ })
  }, [])

  const load = useCallback(() => {
    listTellConversations({ search, sort: sortDir, character: selectedChar || undefined, limit: 1000 })
      .then((r) => setConvos(r.conversations))
      .catch((e: Error) => setError(e.message))
      .finally(() => setLoading(false))
  }, [search, sortDir, selectedChar])

  useEffect(() => {
    setLoading(true)
    setError(null)
    load()
  }, [load])

  // Collapse open threads when switching characters so we don't show one
  // character's expanded conversation under another's list.
  useEffect(() => {
    setExpanded(new Set())
  }, [selectedChar])

  // Live updates: a new tell bumps the list. Coalesce bursts with a short timer
  // so a rapid back-and-forth conversation doesn't refetch on every line.
  const reloadTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const onWs = useCallback((msg: { type: string }) => {
    if (msg.type !== WSEvent.TellsNew) return
    if (reloadTimer.current) clearTimeout(reloadTimer.current)
    reloadTimer.current = setTimeout(() => load(), 400)
  }, [load])
  useWebSocket(onWs)

  function toggle(peer: string) {
    setExpanded((prev) => {
      const next = new Set(prev)
      if (next.has(peer)) next.delete(peer)
      else next.add(peer)
      return next
    })
  }

  async function doClearAll() {
    setConfirmClearOpen(false)
    try {
      await clearTells(selectedChar || undefined)
      setExpanded(new Set())
      load()
    } catch (e) {
      setError((e as Error).message)
    }
  }

  async function doScan() {
    setScanning(true)
    setScanResult(null)
    try {
      const r = await scanTells(selectedChar || undefined)
      setScanResult(`Scanned ${r.character}'s log — added ${r.inserted} new tell${r.inserted === 1 ? '' : 's'}.`)
      load()
    } catch (e) {
      setScanResult(`Scan failed: ${(e as Error).message}`)
    } finally {
      setScanning(false)
    }
  }

  return (
    <div className="flex h-full flex-col">
      {/* Header */}
      <div className="flex items-center gap-3 border-b px-4 py-3 shrink-0" style={{ borderColor: 'var(--color-border)' }}>
        <MessageSquare size={18} style={{ color: 'var(--color-primary)' }} />
        <span className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>Tell Tracker</span>
        <span className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
          {convos.length} conversation{convos.length === 1 ? '' : 's'}
        </span>
        <div className="ml-auto flex items-center gap-2">
          <button
            onClick={() => { setScanResult(null); setScanModalOpen(true) }}
            className="flex items-center gap-1.5 text-xs px-2 py-1 rounded"
            style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-muted-foreground)', border: '1px solid var(--color-border)' }}
          >
            <ScanLine size={11} />
            Scan logs
          </button>
          <button
            onClick={() => setConfirmClearOpen(true)}
            disabled={convos.length === 0}
            className="flex items-center gap-1.5 text-xs px-2 py-1 rounded disabled:opacity-50"
            style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-danger)', border: '1px solid var(--color-border)' }}
          >
            <Trash2 size={11} />
            Clear all
          </button>
        </div>
      </div>

      {/* Character tabs — each character has its own log file & conversations */}
      {characters.length > 1 && (
        <div
          className="border-b px-3 pt-2 shrink-0 flex items-end gap-1 overflow-x-auto"
          style={{ borderColor: 'var(--color-border)' }}
        >
          {characters.map((name) => {
            const active = name === selectedChar
            return (
              <button
                key={name}
                onClick={() => setSelectedChar(name)}
                className="rounded-t px-3 py-1.5 text-xs font-medium whitespace-nowrap"
                style={{
                  backgroundColor: active ? 'var(--color-surface)' : 'transparent',
                  color: active ? 'var(--color-primary)' : 'var(--color-muted-foreground)',
                  border: '1px solid',
                  borderColor: active ? 'var(--color-border)' : 'transparent',
                  borderBottom: active ? '1px solid var(--color-surface)' : '1px solid transparent',
                  marginBottom: -1,
                }}
              >
                {name}
              </button>
            )
          })}
        </div>
      )}

      {/* Filters */}
      <div className="border-b px-4 py-2 shrink-0 flex items-center gap-2 flex-wrap" style={{ borderColor: 'var(--color-border)' }}>
        <div className="relative">
          <Search size={13} style={{ position: 'absolute', left: 8, top: '50%', transform: 'translateY(-50%)', color: 'var(--color-muted-foreground)' }} />
          <input
            type="text"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder="Search by player name…"
            className="text-xs rounded px-2 py-1 pl-7 outline-none"
            style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)', color: 'var(--color-foreground)', minWidth: '16rem' }}
          />
        </div>
        <button
          onClick={() => setSortDir((d) => (d === 'desc' ? 'asc' : 'desc'))}
          className="flex items-center gap-1.5 text-xs px-2 py-1 rounded"
          style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-muted-foreground)', border: '1px solid var(--color-border)' }}
          title="Toggle sort order by most recent activity"
        >
          {sortDir === 'desc' ? <ArrowDown size={11} /> : <ArrowUp size={11} />}
          {sortDir === 'desc' ? 'Newest first' : 'Oldest first'}
        </button>
      </div>

      {/* Body */}
      <div className="flex-1 overflow-y-auto p-4 space-y-2">
        {loading && (
          <div className="flex flex-1 items-center justify-center py-12">
            <RefreshCw size={18} className="animate-spin" style={{ color: 'var(--color-muted)' }} />
          </div>
        )}
        {error && !loading && (
          <div className="flex items-start gap-2 rounded p-3" style={{ backgroundColor: 'var(--color-surface-2)' }}>
            <AlertCircle size={14} style={{ color: 'var(--color-danger)' }} />
            <p className="text-xs" style={{ color: 'var(--color-danger)' }}>{error}</p>
          </div>
        )}
        {!loading && !error && convos.length === 0 && (
          <div className="flex flex-col items-center justify-center gap-2 py-12">
            <MessageSquare size={32} style={{ color: 'var(--color-muted)' }} />
            <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>No tells tracked yet.</p>
            <p className="text-[11px] max-w-md text-center" style={{ color: 'var(--color-muted)' }}>
              Direct tells you send and receive in-game appear here, grouped by player. Channel chatter and NPC merchant replies are filtered out. Use <span className="font-medium">Scan logs</span> to backfill past conversations from your current log file.
            </p>
          </div>
        )}
        {!loading && !error && convos.map((c) => (
          <ConversationRow
            key={c.peer}
            convo={c}
            character={selectedChar}
            expanded={expanded.has(c.peer)}
            onToggle={() => toggle(c.peer)}
            onDeleted={() => { setExpanded((p) => { const n = new Set(p); n.delete(c.peer); return n }); load() }}
          />
        ))}
      </div>

      {confirmClearOpen && (
        <ConfirmModal
          title="Clear tell tracker?"
          body="Delete every stored tell conversation for the active character. This cannot be undone."
          confirmLabel="Clear all"
          onCancel={() => setConfirmClearOpen(false)}
          onConfirm={doClearAll}
        />
      )}

      {scanModalOpen && (
        <ScanModal
          scanning={scanning}
          result={scanResult}
          onScan={doScan}
          onClose={() => setScanModalOpen(false)}
        />
      )}
    </div>
  )
}

function ScanModal({
  scanning,
  result,
  onScan,
  onClose,
}: {
  scanning: boolean
  result: string | null
  onScan: () => void
  onClose: () => void
}): React.ReactElement {
  useEscapeToClose(onClose)
  return (
    <div
      onClick={onClose}
      style={{ position: 'fixed', inset: 0, backgroundColor: 'rgba(0,0,0,0.6)', zIndex: 1000, display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 16 }}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        className="rounded-lg p-4 space-y-3"
        style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)', width: '100%', maxWidth: 460 }}
      >
        <div className="flex items-center gap-2">
          <ScanLine size={16} style={{ color: 'var(--color-primary)' }} />
          <p className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>Scan existing logs?</p>
        </div>
        <p className="text-xs leading-relaxed" style={{ color: 'var(--color-muted-foreground)' }}>
          This reads your active character's entire log file to capture past tell conversations. Large logs can take a while to process. Going forward, new tells are tracked automatically — you only need this once to backfill history. Re-scanning is safe and won't create duplicates.
        </p>
        {result && (
          <p className="text-xs rounded px-2 py-1.5" style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-foreground)' }}>
            {result}
          </p>
        )}
        <div className="flex justify-end gap-2 pt-1">
          <button
            onClick={onClose}
            className="text-xs px-3 py-1.5 rounded font-medium"
            style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-foreground)', border: '1px solid var(--color-border)' }}
          >
            Close
          </button>
          <button
            onClick={onScan}
            disabled={scanning}
            className="flex items-center gap-1.5 text-xs px-3 py-1.5 rounded font-medium disabled:opacity-60"
            style={{ backgroundColor: 'var(--color-primary)', color: '#fff', border: '1px solid transparent' }}
          >
            {scanning ? <RefreshCw size={12} className="animate-spin" /> : <ScanLine size={12} />}
            {scanning ? 'Scanning…' : 'Scan now'}
          </button>
        </div>
      </div>
    </div>
  )
}

function ConfirmModal({
  title, body, confirmLabel, onCancel, onConfirm,
}: {
  title: string
  body: string
  confirmLabel: string
  onCancel: () => void
  onConfirm: () => void
}): React.ReactElement {
  useEscapeToClose(onCancel)
  return (
    <div
      onClick={onCancel}
      style={{ position: 'fixed', inset: 0, backgroundColor: 'rgba(0,0,0,0.6)', zIndex: 1000, display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 16 }}
    >
      <div
        onClick={(e) => e.stopPropagation()}
        className="rounded-lg p-4 space-y-3"
        style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)', width: '100%', maxWidth: 420 }}
      >
        <div className="flex items-center gap-2">
          <AlertCircle size={16} style={{ color: 'var(--color-danger)' }} />
          <p className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>{title}</p>
        </div>
        <p className="text-xs leading-relaxed" style={{ color: 'var(--color-muted-foreground)' }}>{body}</p>
        <div className="flex justify-end gap-2 pt-1">
          <button
            onClick={onCancel}
            className="text-xs px-3 py-1.5 rounded font-medium"
            style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-foreground)', border: '1px solid var(--color-border)' }}
          >
            Cancel
          </button>
          <button
            onClick={onConfirm}
            className="text-xs px-3 py-1.5 rounded font-medium"
            style={{ backgroundColor: 'var(--color-danger)', color: '#fff', border: '1px solid transparent' }}
          >
            {confirmLabel}
          </button>
        </div>
      </div>
    </div>
  )
}
