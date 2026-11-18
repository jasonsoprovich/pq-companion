// Starting points for the New Trigger template picker. Each template turns
// a few answers into a TriggerPrefill, which is either saved straight away
// or opened in the full trigger editor. Line patterns are written in the
// plain-text grammar (internal/trigger/plainpattern.go) and converted to
// regex by the backend.
import type { Action, TimerAlertThreshold, TimerType } from '../types/trigger'

// Everything the trigger editor can be seeded with. The From-spell flow's
// SpellTimerTriggerPrefill is a subset.
export interface TriggerPrefill {
  name: string
  pattern: string
  actions?: Action[]
  timerType?: TimerType
  timerDurationSecs?: number
  wornOffPattern?: string
  spellId?: number
  timerAlerts?: TimerAlertThreshold[]
}

export type TemplateId = 'text' | 'spell' | 'emote' | 'chat' | 'countdown' | 'blank' | 'pack'

export interface TriggerTemplate {
  id: TemplateId
  label: string
  description: string
}

export const TRIGGER_TEMPLATES: TriggerTemplate[] = [
  { id: 'text', label: 'Text alert', description: 'Speak or show a message when a line appears in your log.' },
  { id: 'spell', label: 'Spell timer', description: 'Pick a spell; its timer and fading warning are filled in.' },
  { id: 'emote', label: 'Mob emote warning', description: 'Paste a boss emote and get a big on-screen and spoken warning.' },
  { id: 'chat', label: 'Tell or chat alert', description: 'Know when someone sends you a tell or talks in a channel.' },
  { id: 'countdown', label: 'Countdown timer', description: 'Start a timer bar when a line appears.' },
  { id: 'blank', label: 'Start blank', description: 'Open the full editor with nothing filled in.' },
  { id: 'pack', label: 'Install a pack', description: 'Ready-made triggers for your class or raid targets.' },
]

export type ChatChannel = 'tell' | 'group' | 'guild' | 'raid' | 'ooc' | 'shout' | 'auction'

// Plain-text line for each channel, from the real log format, e.g.
// "Soandso tells the group, 'hi'".
export const CHAT_CHANNELS: { id: ChatChannel; label: string; line: string; spoken: string }[] = [
  { id: 'tell', label: 'Tells', line: "{name} tells you, '{msg}'", spoken: '{name} sent you a tell' },
  { id: 'group', label: 'Group', line: "{name} tells the group, '{msg}'", spoken: '{name} in group' },
  { id: 'guild', label: 'Guild', line: "{name} tells the guild, '{msg}'", spoken: '{name} in guild' },
  { id: 'raid', label: 'Raid', line: "{name} tells the raid, '{msg}'", spoken: '{name} in raid' },
  { id: 'ooc', label: 'OOC', line: "{name} says out of character, '{msg}'", spoken: '{name} in OOC' },
  { id: 'shout', label: 'Shout', line: "{name} shouts, '{msg}'", spoken: '{name} shouted' },
  { id: 'auction', label: 'Auction', line: "{name} auctions, '{msg}'", spoken: '{name} in auction' },
]

// chatLine narrows a channel line to one sender when from is set. The
// name is escaped for the plain grammar (braces doubled).
export function chatLine(channel: ChatChannel, from: string): string {
  const base = CHAT_CHANNELS.find((c) => c.id === channel)!.line
  const who = from.trim()
  return who ? base.replace('{name}', escapePlain(who)) : base
}

// escapePlain makes text match literally in the plain grammar.
export function escapePlain(text: string): string {
  return text.replace(/\{/g, '{{').replace(/\}/g, '}}')
}

export function overlayAction(text: string, color = ''): Action {
  return { type: 'overlay_text', text, duration_secs: 5, color, sound_path: '', volume: 0, voice: '' }
}

export function speakAction(text: string): Action {
  return { type: 'text_to_speech', text, duration_secs: 0, color: '', sound_path: '', volume: 0, voice: '' }
}
