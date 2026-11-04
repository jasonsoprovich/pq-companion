// MeasuredRegen mirrors regen.Snapshot — a character's HP/mana regen per
// server tick, measured live from Zeal (median of the last few clean ticks).
// A rate is absent until enough clean ticks have been seen.
export interface MeasuredRegen {
  character: string
  hp_per_tick?: number
  mana_per_tick?: number
  hp_ticks: number
  mana_ticks: number
  updated_at: string
}
