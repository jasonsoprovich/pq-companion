// TypeScript types mirroring backend/internal/respawn/models.go

export interface RespawnTimer {
  id: string
  /** Display name exactly as it appeared in the kill line (e.g. "a gnoll"). */
  npc_name: string
  /** Disambiguates duplicate names within a zone (the "01", "02" suffix). */
  label_index: number
  /** npc_types.id the name resolved to, or 0 if unresolved. */
  npc_id?: number
  zone: string
  zone_name: string
  died_at: string
  respawn_at: string
  duration_seconds: number
  remaining_seconds: number
  /**
   * True when the name maps to more than one distinct respawn time in the
   * zone. The bar still counts to the single best estimate, but the UI flags
   * it and shows the min/max range.
   */
  ambiguous: boolean
  min_seconds?: number
  max_seconds?: number
}

export interface RespawnState {
  timers: RespawnTimer[]
  /** Player's current zone short_name, so the UI can emphasise local rows. */
  current_zone: string
  last_updated: string
}
