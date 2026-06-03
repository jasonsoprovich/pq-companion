// Mirrors backend internal/chat models. A chat message is player speech on a
// tracked channel (tells + guild/raid/group/ooc/auction/shout and named custom
// channels). Channel chatter that isn't player speech and NPC tell replies are
// filtered out server-side.

export type ChatDirection = 'in' | 'out'

export interface ChatMessage {
  id: number
  character: string
  channel: string
  direction: ChatDirection
  peer: string // tell: other player; channel-in: speaker; channel-out: ''
  message: string
  zone: string
  ts: number
}

export interface ChatConversation {
  peer: string
  count: number
  first_ts: number
  last_ts: number
  last_message: string
  last_direction: ChatDirection
}

export interface ChatChannelsResponse {
  channels: string[]
  characters: string[]
  active: string
}

export interface ChatConversationListResponse {
  conversations: ChatConversation[]
}

export interface ChatMessageListResponse {
  messages: ChatMessage[]
}

// CHANNEL_LABELS maps known channel keys to display names. Named/custom
// channels (not in this map) are title-cased for display.
export const CHANNEL_LABELS: Record<string, string> = {
  tell: 'Tells',
  guild: 'Guild',
  raid: 'Raid',
  group: 'Group',
  ooc: 'OOC',
  auction: 'Auction',
  shout: 'Shout',
}

export function channelLabel(key: string): string {
  if (CHANNEL_LABELS[key]) return CHANNEL_LABELS[key]
  return key.charAt(0).toUpperCase() + key.slice(1)
}
