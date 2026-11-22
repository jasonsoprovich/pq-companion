/**
 * Chat Windows helpers shared by the popout page, the hub card and the
 * editor: channel labels and colors, presets for "+ New chat window", and
 * the window key.
 */
import type { ChatLine, ChatRule, ChatWindow, ChatWindowDef } from '../types/chatWindow'

export const CHAT_WINDOW_COLOR = '#34d399'

export function chatWindowKey(id: string): string {
  return `chatWindow:${id}`
}

// The standard channels, in the order the editor lists them.
export const CHAT_CHANNELS: { key: string; label: string; tag: string }[] = [
  { key: 'tell', label: 'Tells', tag: 'Tell' },
  { key: 'guild', label: 'Guild', tag: 'Guild' },
  { key: 'group', label: 'Group', tag: 'Group' },
  { key: 'raid', label: 'Raid', tag: 'Raid' },
  { key: 'say', label: 'Say', tag: 'Say' },
  { key: 'ooc', label: 'OOC', tag: 'OOC' },
  { key: 'shout', label: 'Shout', tag: 'Shout' },
  { key: 'auction', label: 'Auction', tag: 'Auc' },
]

const STANDARD = new Set(CHAT_CHANNELS.map((c) => c.key))

export function isStandardChannel(key: string): boolean {
  return STANDARD.has(key)
}

// Default text colors, close to EQ's own chat colors.
export const DEFAULT_CHANNEL_COLORS: Record<string, string> = {
  tell: '#e879f9',
  guild: '#4ade80',
  group: '#7dd3fc',
  raid: '#fb923c',
  say: '#e5e7eb',
  ooc: '#a3e635',
  shout: '#f87171',
  auction: '#86efac',
  named: '#67e8f9',
  other: '#a1a1aa',
}

export function capitalize(s: string): string {
  return s ? s.charAt(0).toUpperCase() + s.slice(1) : s
}

export function channelLabel(key: string): string {
  return CHAT_CHANNELS.find((c) => c.key === key)?.label ?? (key === 'other' ? 'Other' : capitalize(key))
}

export function channelTag(key: string): string {
  return CHAT_CHANNELS.find((c) => c.key === key)?.tag ?? (key === 'other' ? '' : capitalize(key))
}

export function channelColor(def: ChatWindowDef, key: string): string {
  const own = def.appearance.channel_colors?.[key]
  if (own) return own
  if (key !== 'other' && !STANDARD.has(key)) {
    return def.appearance.channel_colors?.named ?? DEFAULT_CHANNEL_COLORS.named
  }
  return DEFAULT_CHANNEL_COLORS[key] ?? DEFAULT_CHANNEL_COLORS.other
}

export function lineColor(def: ChatWindowDef, l: ChatLine): string {
  return l.color || channelColor(def, l.channel)
}

export function formatTime(ts: number): string {
  const d = new Date(ts)
  const pad = (n: number): string => String(n).padStart(2, '0')
  return `${pad(d.getHours())}:${pad(d.getMinutes())}`
}

function ruleId(): string {
  return Math.random().toString(36).slice(2, 10)
}

// "Mentions me": keep (and highlight, and pin) any line with my name.
export function mentionsMeRule(): ChatRule {
  return {
    id: ruleId(),
    name: 'Mentions me',
    enabled: true,
    mode: 'simple',
    kind: 'contains',
    text: '{c}',
    color: '#facc15',
    pin_minutes: 2,
  }
}

export function blankRule(keep: boolean): ChatRule {
  return {
    id: ruleId(),
    name: '',
    enabled: true,
    mode: 'simple',
    kind: 'contains',
    text: '',
    ...(keep ? { color: '#facc15' } : {}),
  }
}

export type ChatPreset = 'guild' | 'raid' | 'tells' | 'blank'

export const CHAT_PRESETS: { id: ChatPreset; label: string; blurb: string }[] = [
  { id: 'guild', label: 'Guild chat', blurb: 'Guild and officer chat, your name highlighted.' },
  { id: 'raid', label: 'Raid callouts', blurb: 'Raid and group chat. Lines with your name are pinned for 2 minutes.' },
  { id: 'tells', label: 'Tells', blurb: 'Tells in and out, so none get lost in spam.' },
  { id: 'blank', label: 'Blank', blurb: 'Pick channels and rules yourself.' },
]

export function presetDef(preset: ChatPreset): ChatWindowDef {
  const base: ChatWindowDef = {
    name: 'Chat',
    enabled: true,
    sources: { channels: [] },
    keep: [],
    hide: [],
    appearance: { font_scale: 1, opacity: 0.85, show_timestamps: true, show_channel: true },
    max_lines: 300,
  }
  switch (preset) {
    case 'guild':
      return { ...base, name: 'Guild', sources: { channels: ['guild'] }, keep: [{ ...mentionsMeRule(), pin_minutes: 0 }] }
    case 'raid':
      return { ...base, name: 'Raid', sources: { channels: ['raid', 'group'] }, keep: [mentionsMeRule()] }
    case 'tells':
      return { ...base, name: 'Tells', sources: { channels: ['tell'] }, appearance: { ...base.appearance, show_channel: false } }
    default:
      return { ...base, sources: { channels: ['guild', 'raid', 'group', 'tell'] } }
  }
}

export function chatWindowBlurb(w: ChatWindow): string {
  if (!w.def.enabled) return 'Paused.'
  const src = [
    ...w.def.sources.channels.map(channelLabel),
    ...(w.def.sources.all_named ? ['all named channels'] : []),
    ...(w.def.sources.other ? ['other log lines'] : []),
  ]
  const rules = w.def.keep.length + w.def.hide.length
  return `${src.join(', ') || 'No sources'}${rules ? ` · ${rules} ${rules === 1 ? 'rule' : 'rules'}` : ''}`
}
