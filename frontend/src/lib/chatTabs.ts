/**
 * Chat window tabs: which tabs a window in the tabs layout shows, which
 * lines belong to each, and the tell conversation records behind the
 * per-player tabs. Pure helpers; useChatTabs holds the state.
 *
 * Tab ids: `ch:<channel>` for a channel (`ch:tell` is every tell) and
 * `tell:<peer>` (lowercased) for one tell conversation.
 */
import type { ChatLine, ChatWindowDef } from '../types/chatWindow'
import { CHAT_CHANNELS, capitalize, channelColor, channelLabel, isStandardChannel } from './chatWindow'

export const TELLS_TAB = 'ch:tell'
export const DEFAULT_MAX_TELL_TABS = 8

export interface ChatTab {
  id: string
  label: string
  color: string
  // Set on a tell conversation tab (lowercased peer).
  peer?: string
}

// One tell conversation. Open tabs carry their last activity (line ts);
// closed ones the ts of their newest line when closed, so a snapshot only
// reopens them for a newer line.
export interface ConvRecord {
  name: string
  open: boolean
  last: number
  closedAt?: number
}

export type Convs = Record<string, ConvRecord>

export function channelTabId(channel: string): string {
  return `ch:${channel}`
}

export function convTabId(peer: string): string {
  return `tell:${peer}`
}

export function isConvTab(id: string): boolean {
  return id.startsWith('tell:')
}

// The other player in a tell: the sender of an incoming one, the
// recipient of one you sent.
export function tellPeer(l: ChatLine): string {
  return (l.peer || l.speaker || '').toLowerCase()
}

export function tellTabsOn(def: ChatWindowDef): boolean {
  return def.appearance.layout === 'tabs' && !!def.appearance.tell_tabs && def.sources.channels.includes('tell')
}

// The fixed channel tabs: explicit sources (standard channels in the
// editor's order, then named ones as listed), then named channels seen in
// the lines when the window takes every named channel, then Other.
export function channelTabs(def: ChatWindowDef, lines: ChatLine[]): ChatTab[] {
  const src = def.sources.channels
  const keys: string[] = [
    ...CHAT_CHANNELS.map((c) => c.key).filter((k) => src.includes(k)),
    ...src.filter((k) => !isStandardChannel(k) && k !== 'other'),
  ]
  if (def.sources.all_named) {
    for (const l of lines) {
      const k = l.channel
      if (k !== 'other' && !isStandardChannel(k) && !keys.includes(k)) keys.push(k)
    }
  }
  if (def.sources.other) keys.push('other')
  return keys.map((k) => ({ id: channelTabId(k), label: channelLabel(k), color: channelColor(def, k) }))
}

// How many tabs the window's sources make before any conversation, for
// deciding whether the tabs layout is worth offering.
export function sourceTabCount(def: ChatWindowDef): number {
  return channelTabs(def, []).length + (def.sources.all_named ? 2 : 0)
}

export function tabsOffered(def: ChatWindowDef): boolean {
  if (sourceTabCount(def) >= 2) return true
  return def.sources.channels.includes('tell') && !def.sources.other && !def.sources.all_named
}

export function convTabs(def: ChatWindowDef, convs: Convs): ChatTab[] {
  const color = channelColor(def, 'tell')
  return Object.entries(convs)
    .filter(([, c]) => c.open)
    .sort((a, b) => b[1].last - a[1].last)
    .map(([peer, c]) => ({ id: convTabId(peer), label: c.name, color, peer }))
}

export function lineInTab(id: string, l: ChatLine): boolean {
  if (isConvTab(id)) return l.channel === 'tell' && tellPeer(l) === id.slice('tell:'.length)
  return l.channel === id.slice('ch:'.length)
}

// The tab an appended line counts toward: its conversation when tell tabs
// are on, else its channel.
export function countTabFor(l: ChatLine, tellTabs: boolean): string {
  if (tellTabs && l.channel === 'tell') {
    const p = tellPeer(l)
    if (p) return convTabId(p)
  }
  return channelTabId(l.channel)
}

function displayName(l: ChatLine): string {
  return capitalize(l.peer || l.speaker || '')
}

// openConv opens (or reopens) a conversation for a live tell, whatever its
// time. Returns the same object when nothing changed.
export function openConv(convs: Convs, l: ChatLine): Convs {
  const p = tellPeer(l)
  if (!p) return convs
  const c = convs[p]
  if (c?.open && c.last >= l.ts) return convs
  return { ...convs, [p]: { name: c?.name || displayName(l), open: true, last: Math.max(c?.last ?? 0, l.ts) } }
}

// discoverConvs applies a snapshot's tells: a new peer opens, a closed one
// reopens only for a line newer than when it was closed, an open one
// tracks its latest activity.
export function discoverConvs(convs: Convs, lines: ChatLine[]): Convs {
  let out = convs
  for (const l of lines) {
    if (l.channel !== 'tell') continue
    const p = tellPeer(l)
    if (!p) continue
    const c = out[p]
    if (c && !c.open && l.ts <= (c.closedAt ?? 0)) continue
    if (c?.open && c.last >= l.ts) continue
    out = { ...out, [p]: { name: c?.name || displayName(l), open: true, last: Math.max(c?.last ?? 0, l.ts) } }
  }
  return out
}

export function closeConv(convs: Convs, peer: string, lines: ChatLine[]): Convs {
  const c = convs[peer]
  if (!c || !c.open) return convs
  let newest = c.last
  for (const l of lines) {
    if (l.channel === 'tell' && tellPeer(l) === peer && l.ts > newest) newest = l.ts
  }
  return { ...convs, [peer]: { ...c, open: false, closedAt: newest } }
}

// enforceCap closes the least recently active conversations with nothing
// unread until no more than cap are open. Unread ones are never closed, so
// the cap can be exceeded until they're read.
export function enforceCap(
  convs: Convs,
  cap: number,
  unread: (peer: string) => number,
  keep: string | null,
  lines: ChatLine[],
): Convs {
  const open = Object.entries(convs).filter(([, c]) => c.open).sort((a, b) => a[1].last - b[1].last)
  let extra = open.length - cap
  let out = convs
  for (const [peer] of open) {
    if (extra <= 0) break
    if (peer === keep || unread(peer) > 0) continue
    out = closeConv(out, peer, lines)
    extra--
  }
  return out
}
