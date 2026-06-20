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
  note: string
  pvp: boolean
  tell_count: number
  last_tell_at: number
  group_count: number
  last_grouped_at: number
  // User-entered overrides for permanently-anonymous players. Effective_*
  // resolve display + DPS colours: a real /who value always wins, falling
  // back to the manual override only when nothing was ever seen un-anon.
  manual_class: string
  manual_level: number
  manual_race: string
  effective_class: string
  effective_level: number
  effective_race: string
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
  // Total rows matching the filters, ignoring limit/offset — drives the
  // header count and the "Show more" button.
  total: number
}

export interface PlayerHistoryResponse {
  history: PlayerLevelHistoryEntry[]
}
