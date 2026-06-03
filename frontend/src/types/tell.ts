// Mirrors backend internal/tells models. A "tell" is a direct player-to-player
// message; channel chatter and NPC replies are filtered out server-side.

export type TellDirection = 'in' | 'out'

export interface Tell {
  id: number
  character: string
  peer: string
  direction: TellDirection
  message: string
  zone: string
  ts: number // unix seconds
}

export interface TellConversation {
  peer: string
  count: number
  first_ts: number
  last_ts: number
  last_message: string
  last_direction: TellDirection
}

export interface TellConversationListResponse {
  conversations: TellConversation[]
}

export interface TellThreadResponse {
  messages: Tell[]
}
