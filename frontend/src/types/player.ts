export interface PlayerSighting {
  name: string
  class: string
  race: string
  guild: string
  last_seen_level: number
  last_seen_zone: string
  last_seen_at: number
  first_seen_at: number
  last_anonymous: boolean
  sightings_count: number
}

export interface PlayerLevelHistoryEntry {
  id: number
  name: string
  level: number
  class: string
  zone: string
  observed_at: number
}

export interface PlayerListResponse {
  players: PlayerSighting[]
}

export interface PlayerHistoryResponse {
  history: PlayerLevelHistoryEntry[]
}
