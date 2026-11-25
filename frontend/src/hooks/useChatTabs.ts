/**
 * useChatTabs — the tabs layout of one chat window: its tabs, the active
 * one, unread counts and tell conversations.
 *
 * Unread counts only come from live appends (onAppend, fed by
 * useChatWindows), never from snapshots, so a history merge, refetch or
 * replay swap can't make anything look new. Lines you sent and lines on
 * screen (the active tab; any tell while the all-tells tab is active)
 * don't count. A count is capped at the lines its tab holds, which covers
 * Clear and scrollback eviction. With tell tabs on, the Tells tab's count
 * is the sum of the conversation counts: opening it reads them all.
 *
 * Tell conversations (and the active tab) persist per window in
 * localStorage. While a Log Feed replay runs they live in memory only,
 * starting blank, and the saved ones come back when it ends; counts are
 * cleared whenever replay starts or stops.
 *
 * persistKey null (the editor preview) keeps everything in memory.
 */
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { ChatLine, ChatWindowDef } from '../types/chatWindow'
import {
  DEFAULT_MAX_TELL_TABS,
  TELLS_TAB,
  channelTabs,
  closeConv,
  convTabId,
  convTabs,
  countTabFor,
  discoverConvs,
  enforceCap,
  isConvTab,
  lineInTab,
  openConv,
  tellTabsOn,
  type ChatTab,
  type Convs,
} from '../lib/chatTabs'

interface Saved {
  active?: string
  convs?: Convs
}

function storageKey(id: string): string {
  return `pq.chatTabs.${id}`
}

function load(key: string | null): Saved {
  if (!key) return {}
  try {
    const raw = localStorage.getItem(storageKey(key))
    return raw ? (JSON.parse(raw) as Saved) : {}
  } catch {
    return {}
  }
}

export interface ChatTabsState {
  enabled: boolean
  tabs: ChatTab[]
  active: string
  select: (id: string) => void
  close: (id: string) => void
  closeAllRead: () => void
  unread: (id: string) => number
  // The active tab's lines (all lines when tabs are off).
  lines: ChatLine[]
  onAppend: (lines: ChatLine[], replay: boolean) => void
}

export function useChatTabs({
  def,
  lines,
  replay,
  persistKey,
}: {
  def: ChatWindowDef | null
  lines: ChatLine[]
  replay: boolean
  persistKey: string | null
}): ChatTabsState {
  const enabled = def?.appearance.layout === 'tabs'
  const tellTabs = !!def && tellTabsOn(def)
  const cap = def?.appearance.max_tell_tabs || DEFAULT_MAX_TELL_TABS

  const [saved] = useState(() => (replay ? {} : load(persistKey)))
  const [convs, setConvs] = useState<Convs>(() => (replay ? {} : saved.convs ?? {}))
  const [activeRaw, setActive] = useState<string>(saved.active ?? '')
  const [counts, setCounts] = useState<Record<string, number>>({})

  // Replay start/stop: clear counts; swap to blank or saved conversations.
  const replayRef = useRef(replay)
  useEffect(() => {
    if (replayRef.current === replay) return
    replayRef.current = replay
    setCounts({})
    setConvs(replay ? {} : load(persistKey).convs ?? {})
  }, [replay, persistKey])

  const tabs = useMemo<ChatTab[]>(() => {
    if (!def || !enabled) return []
    return [...channelTabs(def, lines), ...(tellTabs ? convTabs(def, convs) : [])]
  }, [def, enabled, tellTabs, lines, convs])

  const active = tabs.some((t) => t.id === activeRaw) ? activeRaw : tabs[0]?.id ?? ''
  const activeRef = useRef(active)
  activeRef.current = active

  // Lines per tab, for capping counts.
  const tabLineCounts = useMemo(() => {
    const out: Record<string, number> = {}
    if (!enabled) return out
    for (const l of lines) {
      const ch = `ch:${l.channel}`
      out[ch] = (out[ch] ?? 0) + 1
      if (tellTabs && l.channel === 'tell') {
        const id = countTabFor(l, true)
        if (id !== ch) out[id] = (out[id] ?? 0) + 1
      }
    }
    return out
  }, [lines, enabled, tellTabs])

  const unread = useCallback(
    (id: string): number => {
      if (id === TELLS_TAB && tellTabs) {
        let sum = 0
        for (const [k, n] of Object.entries(counts)) {
          if (isConvTab(k)) sum += Math.min(n, tabLineCounts[k] ?? 0)
        }
        return sum
      }
      return Math.min(counts[id] ?? 0, tabLineCounts[id] ?? 0)
    },
    [counts, tabLineCounts, tellTabs],
  )

  // Snapshot discovery of tell conversations.
  useEffect(() => {
    if (!tellTabs) return
    setConvs((c) => discoverConvs(c, lines))
  }, [lines, tellTabs])

  // Keep open conversations within the cap (after opens, reads, loads and
  // cap changes).
  useEffect(() => {
    if (!tellTabs) return
    const keep = isConvTab(active) ? active.slice('tell:'.length) : null
    setConvs((c) => enforceCap(c, cap, (p) => unread(convTabId(p)), keep, lines))
  }, [convs, cap, unread, tellTabs, active, lines])

  // Persist (never during replay or in the preview).
  useEffect(() => {
    if (!persistKey || replay) return
    try {
      localStorage.setItem(storageKey(persistKey), JSON.stringify({ active: activeRaw, convs }))
    } catch {
      // storage full or unavailable: tabs still work for this session
    }
  }, [persistKey, replay, activeRaw, convs])

  const select = useCallback(
    (id: string) => {
      setActive(id)
      setCounts((c) => {
        const next: Record<string, number> = { ...c, [id]: 0 }
        if (id === TELLS_TAB && tellTabs) {
          for (const k of Object.keys(next)) if (isConvTab(k)) next[k] = 0
        }
        return next
      })
    },
    [tellTabs],
  )

  const linesRef = useRef(lines)
  linesRef.current = lines

  const close = useCallback(
    (id: string) => {
      if (!isConvTab(id)) return
      const peer = id.slice('tell:'.length)
      setConvs((c) => closeConv(c, peer, linesRef.current))
      setCounts((c) => {
        const next = { ...c }
        delete next[id]
        return next
      })
      if (activeRef.current === id) setActive(TELLS_TAB)
    },
    [],
  )

  const closeAllRead = useCallback(() => {
    setConvs((c) => {
      let out = c
      for (const [peer, rec] of Object.entries(c)) {
        const id = convTabId(peer)
        if (rec.open && id !== activeRef.current && unread(id) === 0) out = closeConv(out, peer, linesRef.current)
      }
      return out
    })
  }, [unread])

  const stateRef = useRef({ enabled, tellTabs })
  stateRef.current = { enabled, tellTabs }

  const onAppend = useCallback((appended: ChatLine[], isReplay: boolean) => {
    const { enabled: on, tellTabs: tt } = stateRef.current
    if (!on) return
    if (tt) {
      for (const l of appended) {
        if (l.channel === 'tell') setConvs((c) => openConv(c, l))
      }
    }
    if (isReplay) return
    const cur = activeRef.current
    const add: Record<string, number> = {}
    for (const l of appended) {
      if (l.direction === 'out') continue
      const id = countTabFor(l, tt)
      if (id === cur) continue
      if (tt && l.channel === 'tell' && cur === TELLS_TAB) continue
      add[id] = (add[id] ?? 0) + 1
    }
    if (Object.keys(add).length === 0) return
    setCounts((c) => {
      const next = { ...c }
      for (const [k, n] of Object.entries(add)) next[k] = (next[k] ?? 0) + n
      return next
    })
  }, [])

  const shown = useMemo(
    () => (enabled && active ? lines.filter((l) => lineInTab(active, l)) : lines),
    [enabled, active, lines],
  )

  return { enabled, tabs, active, select, close, closeAllRead, unread, lines: shown, onAppend }
}
