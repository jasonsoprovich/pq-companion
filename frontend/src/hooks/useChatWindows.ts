/**
 * useChatWindows — live Chat Windows state for the popout windows and the
 * hub. Seeds from the REST snapshot, then follows three WS events:
 *   chatwindows:state  — everything, replaced
 *   chatwindows:window — one window, replaced (pin change, re-filter, refill)
 *   chatwindows:lines  — batched appends tagged (epoch, seq)
 * A batch from an older epoch is dropped (a replacement already superseded
 * it); one from a newer epoch, or with a seq gap, means we missed something,
 * so the snapshot is refetched. Lines a snapshot already had (a batch racing
 * the fetch) are skipped by seq. Refetches whenever the socket (re)opens.
 */
import { useCallback, useEffect, useRef, useState } from 'react'
import { useWebSocket } from './useWebSocket'
import { WSEvent } from '../lib/wsEvents'
import { chatWindowAction, getChatWindows } from '../services/api'
import type {
  ChatLinesEvent,
  ChatWindow,
  ChatWindowAction,
  ChatWindowsState,
} from '../types/chatWindow'

export interface ChatWindowsHook {
  windows: ChatWindow[]
  // Milliseconds to add to Date.now() to get server time (pin expiry).
  skewMs: number
  loaded: boolean
  error: string
  refresh: () => void
  act: (id: string, body: ChatWindowAction) => Promise<void>
}

// applyBatch merges one window's appends, or returns null when the client
// must refetch.
function applyBatch(
  w: ChatWindow,
  b: ChatLinesEvent['windows'][number],
): ChatWindow | null | 'skip' {
  if (b.epoch < w.state.epoch) return 'skip'
  if (b.epoch > w.state.epoch) return null
  const fresh = b.lines.filter((l) => l.seq > w.state.seq)
  if (fresh.length && fresh[0].seq !== w.state.seq + 1) return null
  const max = w.def.max_lines > 0 ? w.def.max_lines : 300
  let lines = fresh.length ? [...w.state.lines, ...fresh] : w.state.lines
  if (lines.length > max) lines = lines.slice(lines.length - max)
  return {
    ...w,
    state: {
      ...w.state,
      seq: fresh.length ? fresh[fresh.length - 1].seq : w.state.seq,
      lines,
      pins: b.pins ?? [],
    },
  }
}

export function useChatWindows(): ChatWindowsHook {
  const [windows, setWindows] = useState<ChatWindow[]>([])
  const [skewMs, setSkewMs] = useState(0)
  const [loaded, setLoaded] = useState(false)
  const [error, setError] = useState('')
  const windowsRef = useRef<ChatWindow[]>([])
  const fetching = useRef(false)

  const set = useCallback((ws: ChatWindow[]) => {
    windowsRef.current = ws
    setWindows(ws)
  }, [])

  const applyState = useCallback((s: ChatWindowsState) => {
    set(s.windows ?? [])
    if (s.now) setSkewMs(s.now - Date.now())
    setLoaded(true)
  }, [set])

  const refresh = useCallback(() => {
    if (fetching.current) return
    fetching.current = true
    getChatWindows()
      .then((s) => {
        applyState(s)
        setError('')
      })
      .catch((e: Error) => setError(e.message))
      .finally(() => {
        fetching.current = false
      })
  }, [applyState])

  const ready = useWebSocket((msg) => {
    if (msg.type === WSEvent.ChatWindowsState) {
      applyState(msg.data as ChatWindowsState)
    } else if (msg.type === WSEvent.ChatWindowsWindow) {
      const w = msg.data as ChatWindow
      const cur = windowsRef.current
      const i = cur.findIndex((x) => x.def.id === w.def.id)
      if (i < 0) {
        refresh()
        return
      }
      if (w.state.epoch < cur[i].state.epoch) return
      const next = [...cur]
      next[i] = w
      set(next)
    } else if (msg.type === WSEvent.ChatWindowsLines) {
      const ev = msg.data as ChatLinesEvent
      if (ev.now) setSkewMs(ev.now - Date.now())
      let next = windowsRef.current
      let stale = false
      for (const b of ev.windows ?? []) {
        const i = next.findIndex((x) => x.def.id === b.id)
        if (i < 0) {
          stale = true
          continue
        }
        const merged = applyBatch(next[i], b)
        if (merged === 'skip') continue
        if (merged === null) {
          stale = true
          continue
        }
        if (next === windowsRef.current) next = [...next]
        next[i] = merged
      }
      if (next !== windowsRef.current) set(next)
      if (stale) refresh()
    }
  })

  useEffect(() => {
    if (ready !== 'open' && loaded) return
    refresh()
  }, [ready]) // eslint-disable-line react-hooks/exhaustive-deps

  const act = useCallback(async (id: string, body: ChatWindowAction) => {
    try {
      await chatWindowAction(id, body)
      setError('')
    } catch (e) {
      setError((e as Error).message)
    }
  }, [])

  return { windows, skewMs, loaded, error, refresh, act }
}
