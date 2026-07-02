import React, { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import {
  MessageSquare, RefreshCw, Trash2, AlertCircle, ArrowUp, ArrowDown,
  ChevronRight, ChevronDown, Search,
} from 'lucide-react'
import {
  getChatChannels, listChatConversations, getChatThread, getChatFeed,
  deleteChatPeer, clearChat,
} from '../services/api'
import type { ChatMessage, ChatConversation } from '../types/chat'
import { channelLabel } from '../types/chat'
import { useWebSocket } from '../hooks/useWebSocket'
import { WSEvent } from '../lib/wsEvents'
import { useEscapeToClose } from '../hooks/useEscapeToClose'
import MissingLogNotice from '../components/MissingLogNotice'
import BackfillLink from '../components/BackfillLink'

// Standard channels always shown in the dropdown (even before they have data),
// in this order; named/custom channels present are appended alphabetically.
const KNOWN_CHANNELS = ['tell', 'guild', 'raid', 'group', 'ooc', 'auction', 'shout']

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

// Convert a yyyy-mm-dd date input to a unix-seconds boundary (start or end of
// that local day). Empty string → 0 (unbounded).
function dateToUnix(d: string, endOfDay: boolean): number {
  if (!d) return 0
  const t = new Date(`${d}T${endOfDay ? '23:59:59' : '00:00:00'}`)
  if (isNaN(t.getTime())) return 0
  return Math.floor(t.getTime() / 1000)
}

export default function ChatHistoryPage(): React.ReactElement {
  const [characters, setCharacters] = useState<string[]>([])
  const [selectedChar, setSelectedChar] = useState<string>('')
  const [presentChannels, setPresentChannels] = useState<string[]>([])
  const [channel, setChannel] = useState<string>('tell')
  const [search, setSearch] = useState('')
  const [fromDate, setFromDate] = useState('')
  const [toDate, setToDate] = useState('')
  const [sortDir, setSortDir] = useState<'asc' | 'desc'>('desc')

  const [convos, setConvos] = useState<ChatConversation[]>([])
  const [feed, setFeed] = useState<ChatMessage[]>([])
  const [expanded, setExpanded] = useState<Set<string>>(new Set())
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  // Whether the character/channel metadata fetch has returned. Until it has,
  // the character tab bar's height is reserved so it doesn't pop in a moment
  // after the page renders and shove the content down (a jarring shift).
  const [metaLoaded, setMetaLoaded] = useState(false)
  const [confirmClearOpen, setConfirmClearOpen] = useState(false)

  // Scroll-preservation for live (background) reloads. The body list is fully
  // re-fetched on every chat:new, so without this the swap would jump the
  // viewport. We snapshot the scroll position just before the data swap and
  // restore it in a layout effect once the new rows are laid out — following
  // the newest edge if the user is already pinned there.
  const bodyRef = useRef<HTMLDivElement>(null)
  const pendingScroll = useRef<{ top: number; height: number; atTop: boolean; atBottom: boolean } | null>(null)
  // Monotonic token so a slow earlier fetch can't clobber a newer one.
  const seqRef = useRef(0)
  const snapshotScroll = useCallback(() => {
    const el = bodyRef.current
    if (!el) return
    pendingScroll.current = {
      top: el.scrollTop,
      height: el.scrollHeight,
      atTop: el.scrollTop < 40,
      atBottom: el.scrollHeight - el.scrollTop - el.clientHeight < 40,
    }
  }, [])

  // Load characters + present channels once (and refresh after backfills via a
  // manual refresh).
  const loadMeta = useCallback(() => {
    getChatChannels(selectedChar || undefined)
      .then((r) => {
        const chars = r.characters ?? []
        setCharacters(chars)
        setPresentChannels(r.channels ?? [])
        setSelectedChar((cur) => {
          if (cur && chars.includes(cur)) return cur
          if (r.active && chars.includes(r.active)) return r.active
          return chars[0] ?? r.active ?? ''
        })
        // Flip metaLoaded in the same batch as selectedChar so the feed effect
        // (gated on metaLoaded) fires exactly once with the resolved character,
        // instead of loading first for "" and again after the name resolves.
        setMetaLoaded(true)
      })
      .catch(() => { setMetaLoaded(true) })
  }, [selectedChar])

  useEffect(() => { loadMeta() }, []) // eslint-disable-line react-hooks/exhaustive-deps

  const channelOptions = useMemo(() => {
    const named = (presentChannels ?? []).filter((c) => !KNOWN_CHANNELS.includes(c)).sort()
    return [...KNOWN_CHANNELS, ...named]
  }, [presentChannels])

  // load(background): a foreground load shows the spinner and replaces the
  // view (used on mount, filter/character changes, and manual Refresh). A
  // background load is what live chat:new events trigger — it keeps the
  // current list mounted and visible (no spinner flash, no scroll reset) and
  // swallows transient fetch errors so a momentary failure never blanks the
  // view. Scroll is preserved across the silent data swap.
  const load = useCallback((background = false) => {
    // Monotonic token: bump per call so a slow earlier fetch (foreground or
    // background) can't overwrite a newer one's results out of order.
    const seq = ++seqRef.current
    if (!background) setLoading(true)
    setError(null)
    const from = dateToUnix(fromDate, false)
    const to = dateToUnix(toDate, true)
    if (channel === 'tell') {
      listChatConversations({ character: selectedChar || undefined, search, from, to, sort: sortDir, limit: 2000 })
        .then((r) => { if (seq !== seqRef.current) return; if (background) snapshotScroll(); setConvos(r.conversations); setFeed([]) })
        .catch((e: Error) => { if (seq === seqRef.current && !background) setError(e.message) })
        .finally(() => { if (seq === seqRef.current && !background) setLoading(false) })
    } else {
      getChatFeed({ character: selectedChar || undefined, channel, search, from, to, sort: sortDir, limit: 3000 })
        .then((r) => { if (seq !== seqRef.current) return; if (background) snapshotScroll(); setFeed(r.messages); setConvos([]) })
        .catch((e: Error) => { if (seq === seqRef.current && !background) setError(e.message) })
        .finally(() => { if (seq === seqRef.current && !background) setLoading(false) })
    }
  }, [channel, selectedChar, search, fromDate, toDate, sortDir, snapshotScroll])

  // Restore the snapshotted scroll position after a background data swap.
  // desc (newest first): new rows prepend at the top, so anchor by the height
  // delta — unless the user is pinned to the top, where we follow the newest.
  // asc (oldest first): new rows append at the bottom, so keep the position —
  // unless pinned to the bottom, where we follow.
  useLayoutEffect(() => {
    const snap = pendingScroll.current
    const el = bodyRef.current
    if (!snap || !el) return
    pendingScroll.current = null
    if (sortDir === 'desc') {
      el.scrollTop = snap.atTop ? 0 : snap.top + (el.scrollHeight - snap.height)
    } else {
      el.scrollTop = snap.atBottom ? el.scrollHeight : snap.top
    }
  }, [feed, convos, sortDir])

  // Wait for the character/channel metadata before the first fetch: otherwise
  // the feed loads once for the unresolved (empty) character and then again
  // once loadMeta sets the real selectedChar — a visible double spinner.
  useEffect(() => { if (metaLoaded) load() }, [load, metaLoaded])

  // Collapse threads when changing character or channel.
  useEffect(() => { setExpanded(new Set()) }, [selectedChar, channel])

  // Live updates (debounced) on new chat.
  const reloadTimer = useRef<ReturnType<typeof setTimeout> | null>(null)
  const onWs = useCallback((msg: { type: string }) => {
    if (msg.type !== WSEvent.ChatNew) return
    if (reloadTimer.current) clearTimeout(reloadTimer.current)
    reloadTimer.current = setTimeout(() => load(true), 500)
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

  async function doClear() {
    setConfirmClearOpen(false)
    try {
      await clearChat(selectedChar || undefined, channel)
      setExpanded(new Set())
      load()
      loadMeta()
    } catch (e) {
      setError((e as Error).message)
    }
  }

  const isTell = channel === 'tell'
  const count = isTell ? convos.length : feed.length

  return (
    <div className="flex h-full flex-col">
      {/* Header */}
      <div className="flex items-center gap-3 border-b px-4 py-3 shrink-0" style={{ borderColor: 'var(--color-border)' }}>
        <MessageSquare size={18} style={{ color: 'var(--color-primary)' }} />
        <span className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>Chat History</span>
        <span className="text-[11px]" style={{ color: 'var(--color-muted)' }}>
          {isTell ? `${count} conversation${count === 1 ? '' : 's'}` : `${count} message${count === 1 ? '' : 's'}`}
        </span>
        <div className="ml-auto flex items-center gap-2">
          <BackfillLink />
          <button
            onClick={() => { load(); loadMeta() }}
            className="flex items-center gap-1.5 text-xs px-2 py-1 rounded"
            style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-muted-foreground)', border: '1px solid var(--color-border)' }}
          >
            <RefreshCw size={11} />
            Refresh
          </button>
          <button
            onClick={() => setConfirmClearOpen(true)}
            disabled={count === 0}
            className="flex items-center gap-1.5 text-xs px-2 py-1 rounded disabled:opacity-50"
            style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-danger)', border: '1px solid var(--color-border)' }}
          >
            <Trash2 size={11} />
            Clear {channelLabel(channel)}
          </button>
        </div>
      </div>

      {/* Character tabs. While metadata is still loading we reserve the bar's
          height so a multi-character bar doesn't pop in late and shove the
          content down. Once loaded, the bar shows only when there's more than
          one character to switch between. The outer div carries the border;
          the inner scroll container overhangs it by 1px (instead of each tab
          carrying a negative margin) so the active tab's bottom border covers
          the row border without creating a 1px vertical overflow —
          overflow-x:auto forces overflow-y to auto too, and that overflow
          rendered as a stray scrollbar on the right of the row. */}
      {!metaLoaded ? (
        // Reserved-height placeholder: same markup as the real bar with one
        // invisible tab, so the row occupies the exact final height and the
        // content below never jumps when the tabs resolve.
        <div className="border-b shrink-0" style={{ borderColor: 'var(--color-border)' }} aria-hidden>
          <div className="px-3 pt-2 flex items-end gap-1 overflow-x-auto" style={{ marginBottom: -1 }}>
            <button
              className="rounded-t px-3 py-1.5 text-xs font-medium whitespace-nowrap"
              style={{ visibility: 'hidden', border: '1px solid transparent' }}
              tabIndex={-1}
            >
              &nbsp;
            </button>
          </div>
        </div>
      ) : characters.length > 1 ? (
        <div className="border-b shrink-0" style={{ borderColor: 'var(--color-border)' }}>
          <div className="px-3 pt-2 flex items-end gap-1 overflow-x-auto" style={{ marginBottom: -1 }}>
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
                  }}
                >
                  {name}
                </button>
              )
            })}
          </div>
        </div>
      ) : null}

      {/* Filters */}
      <div className="border-b px-4 py-2 shrink-0 flex items-center gap-2 flex-wrap" style={{ borderColor: 'var(--color-border)' }}>
        <select
          value={channel}
          onChange={(e) => setChannel(e.target.value)}
          className="text-xs rounded px-2 py-1 outline-none"
          style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)', color: 'var(--color-foreground)' }}
          title="Chat channel"
        >
          {channelOptions.map((c) => (
            <option key={c} value={c}>{channelLabel(c)}</option>
          ))}
        </select>
        <div className="relative">
          <Search size={13} style={{ position: 'absolute', left: 8, top: '50%', transform: 'translateY(-50%)', color: 'var(--color-muted-foreground)' }} />
          <input
            type="text"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            placeholder={isTell ? 'Search by player name…' : 'Search name or text…'}
            className="text-xs rounded px-2 py-1 pl-7 outline-none"
            style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)', color: 'var(--color-foreground)', minWidth: '14rem' }}
          />
        </div>
        <label className="flex items-center gap-1 text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>
          From
          <input type="date" value={fromDate} onChange={(e) => setFromDate(e.target.value)}
            className="text-xs rounded px-1.5 py-1 outline-none"
            style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)', color: 'var(--color-foreground)' }} />
        </label>
        <label className="flex items-center gap-1 text-[11px]" style={{ color: 'var(--color-muted-foreground)' }}>
          To
          <input type="date" value={toDate} onChange={(e) => setToDate(e.target.value)}
            className="text-xs rounded px-1.5 py-1 outline-none"
            style={{ backgroundColor: 'var(--color-surface-2)', border: '1px solid var(--color-border)', color: 'var(--color-foreground)' }} />
        </label>
        <button
          onClick={() => setSortDir((d) => (d === 'desc' ? 'asc' : 'desc'))}
          className="flex items-center gap-1.5 text-xs px-2 py-1 rounded"
          style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-muted-foreground)', border: '1px solid var(--color-border)' }}
          title="Toggle sort order"
        >
          {sortDir === 'desc' ? <ArrowDown size={11} /> : <ArrowUp size={11} />}
          {sortDir === 'desc' ? 'Newest first' : 'Oldest first'}
        </button>
        {(search || fromDate || toDate) && (
          <button
            onClick={() => { setSearch(''); setFromDate(''); setToDate('') }}
            className="text-xs px-2 py-1 rounded"
            style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-muted-foreground)', border: '1px solid var(--color-border)' }}
          >
            Clear filters
          </button>
        )}
      </div>

      {/* Body */}
      <div ref={bodyRef} className="flex-1 overflow-y-auto p-4 space-y-2">
        <MissingLogNotice />
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

        {!loading && !error && count === 0 && (
          <div className="flex flex-col items-center justify-center gap-2 py-12">
            <MessageSquare size={32} style={{ color: 'var(--color-muted)' }} />
            <p className="text-sm" style={{ color: 'var(--color-muted-foreground)' }}>
              No {channelLabel(channel)} chat {isTell ? 'tracked' : 'captured'} yet.
            </p>
            <p className="text-[11px] max-w-md text-center" style={{ color: 'var(--color-muted)' }}>
              Chat is captured live as you play. To backfill from a character's log, use the <span className="font-medium">Backfill</span> button above (Settings → Logs).
            </p>
          </div>
        )}

        {/* Tell conversations */}
        {!loading && !error && isTell && convos.map((c) => (
          <ConversationRow
            key={c.peer}
            convo={c}
            character={selectedChar}
            expanded={expanded.has(c.peer)}
            onToggle={() => toggle(c.peer)}
            onDeleted={() => { setExpanded((p) => { const n = new Set(p); n.delete(c.peer); return n }); load() }}
          />
        ))}

        {/* Channel flat feed */}
        {!loading && !error && !isTell && feed.length > 0 && (
          <div className="flex flex-col gap-1">
            {feed.map((m) => (
              <div key={m.id} className="flex items-baseline gap-2 rounded px-2 py-1" style={{ backgroundColor: 'var(--color-surface)' }}>
                <span className="text-[10px] tabular-nums shrink-0" style={{ color: 'var(--color-muted)', minWidth: '5.5rem' }}>
                  {formatTimestamp(m.ts)}
                </span>
                <span className="text-xs font-semibold shrink-0" style={{ color: m.direction === 'out' ? 'var(--color-primary)' : 'var(--color-foreground)', minWidth: '6rem' }}>
                  {m.direction === 'out' ? 'You' : m.peer}
                </span>
                <span className="text-xs" style={{ color: 'var(--color-foreground)', whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>
                  {m.message}
                </span>
              </div>
            ))}
          </div>
        )}
      </div>

      {confirmClearOpen && (
        <ConfirmModal
          title={`Clear ${channelLabel(channel)} history?`}
          body={`Delete all stored ${channelLabel(channel)} messages for ${selectedChar || 'the active character'}. This cannot be undone.`}
          confirmLabel="Clear"
          onCancel={() => setConfirmClearOpen(false)}
          onConfirm={doClear}
        />
      )}
    </div>
  )
}

// ConversationRow — an expandable tell conversation rendered as a chat thread.
function ConversationRow({
  convo, character, expanded, onToggle, onDeleted,
}: {
  convo: ChatConversation
  character: string
  expanded: boolean
  onToggle: () => void
  onDeleted: () => void
}): React.ReactElement {
  const [thread, setThread] = useState<ChatMessage[] | null>(null)
  const [err, setErr] = useState<string | null>(null)

  const loadThread = useCallback(() => {
    getChatThread(convo.peer, { sort: 'asc', character: character || undefined })
      .then((r) => setThread(r.messages))
      .catch((e: Error) => setErr(e.message))
  }, [convo.peer, character])

  useEffect(() => { if (expanded && thread === null) loadThread() }, [expanded, thread, loadThread])
  useEffect(() => { if (expanded) loadThread() }, [convo.last_ts]) // eslint-disable-line react-hooks/exhaustive-deps

  async function handleDelete(e: React.MouseEvent) {
    e.stopPropagation()
    try { await deleteChatPeer(convo.peer, character || undefined); onDeleted() } catch { /* best-effort */ }
  }

  return (
    <div className="rounded-lg border" style={{ backgroundColor: 'var(--color-surface)', borderColor: 'var(--color-border)' }}>
      <button onClick={onToggle} className="flex w-full items-center gap-2 px-3 py-1.5 text-left" style={{ background: 'transparent', border: 'none', cursor: 'pointer' }}>
        {expanded ? <ChevronDown size={14} style={{ color: 'var(--color-muted)' }} /> : <ChevronRight size={14} style={{ color: 'var(--color-muted)' }} />}
        <span className="text-sm font-semibold" style={{ color: 'var(--color-primary)' }}>{convo.peer}</span>
        <span className="text-[11px]" style={{ color: 'var(--color-muted)' }}>{convo.count} message{convo.count === 1 ? '' : 's'}</span>
        <span className="mx-2 flex-1 truncate text-xs" style={{ color: 'var(--color-muted-foreground)' }}>
          <span style={{ color: 'var(--color-muted)' }}>{convo.last_direction === 'out' ? 'You: ' : `${convo.peer}: `}</span>
          {convo.last_message}
        </span>
        <span className="text-[11px] tabular-nums" style={{ color: 'var(--color-muted)' }}>{formatRelative(convo.last_ts)}</span>
        <span onClick={handleDelete} title="Delete this conversation" className="rounded p-1" style={{ color: 'var(--color-danger)' }}>
          <Trash2 size={13} />
        </span>
      </button>

      {expanded && (
        <div className="border-t px-2.5 py-2 flex flex-col gap-1" style={{ borderColor: 'var(--color-border)', backgroundColor: 'var(--color-background)' }}>
          {err && <p className="text-xs" style={{ color: 'var(--color-danger)' }}>{err}</p>}
          {!err && thread === null && <p className="text-xs" style={{ color: 'var(--color-muted)' }}>Loading…</p>}
          {thread !== null && thread.length === 0 && <p className="text-xs" style={{ color: 'var(--color-muted)' }}>No messages.</p>}
          {thread !== null && thread.map((m) => {
            const out = m.direction === 'out'
            return (
              <div key={m.id} className="flex items-baseline gap-2 rounded px-2 py-1" style={{ backgroundColor: 'var(--color-surface)' }}>
                <span className="text-[10px] tabular-nums shrink-0" style={{ color: 'var(--color-muted)', minWidth: '5.5rem' }} title={m.zone || undefined}>
                  {formatTimestamp(m.ts)}
                </span>
                <span className="text-xs font-semibold shrink-0" style={{ color: out ? 'var(--color-primary)' : 'var(--color-foreground)', minWidth: '6rem' }}>
                  {out ? 'You' : m.peer}
                </span>
                <span className="text-xs" style={{ color: 'var(--color-foreground)', whiteSpace: 'pre-wrap', wordBreak: 'break-word' }}>
                  {m.message}
                </span>
              </div>
            )
          })}
        </div>
      )}
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
    <div onClick={onCancel} style={{ position: 'fixed', inset: 0, backgroundColor: 'rgba(0,0,0,0.6)', zIndex: 1000, display: 'flex', alignItems: 'center', justifyContent: 'center', padding: 16 }}>
      <div onClick={(e) => e.stopPropagation()} className="rounded-lg p-4 space-y-3" style={{ backgroundColor: 'var(--color-surface)', border: '1px solid var(--color-border)', width: '100%', maxWidth: 420 }}>
        <div className="flex items-center gap-2">
          <AlertCircle size={16} style={{ color: 'var(--color-danger)' }} />
          <p className="text-sm font-semibold" style={{ color: 'var(--color-foreground)' }}>{title}</p>
        </div>
        <p className="text-xs leading-relaxed" style={{ color: 'var(--color-muted-foreground)' }}>{body}</p>
        <div className="flex justify-end gap-2 pt-1">
          <button onClick={onCancel} className="text-xs px-3 py-1.5 rounded font-medium" style={{ backgroundColor: 'var(--color-surface-2)', color: 'var(--color-foreground)', border: '1px solid var(--color-border)' }}>Cancel</button>
          <button onClick={onConfirm} className="text-xs px-3 py-1.5 rounded font-medium" style={{ backgroundColor: 'var(--color-danger)', color: '#fff', border: '1px solid transparent' }}>{confirmLabel}</button>
        </div>
      </div>
    </div>
  )
}
