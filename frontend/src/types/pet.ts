// Mirrors backend/internal/petoverlay (State, Stats, Immunities,
// HealthSnapshot) — the Pet overlay's pet:state payload.

export type PetMode = 'none' | 'pet' | 'broken'
export type PetKind = '' | 'charm' | 'summoned'

export interface PetImmunities {
  slow?: boolean
  mez?: boolean
  stun?: boolean
  snare_root?: boolean
  fear?: boolean
  haste?: boolean
  reverse_slow?: boolean
  ranged?: boolean
}

// PetStats is the pet's npc_types row scaled to its level. When the exact
// level is unknown (`level` absent) the scaled fields span level_min..max.
export interface PetStats {
  npc_id: number
  class: number
  class_name: string
  race: number
  race_name: string
  body_type: number
  level?: number
  level_min: number
  level_max: number
  hp_min: number
  hp_max: number
  min_hit: number
  max_hit_min: number
  max_hit_max: number
  attack_delay: number // tenths of a second
  dps_min: number
  dps_max: number
  mr: number
  fr: number
  cr: number
  pr: number
  dr: number
  summons: boolean
  gates: boolean
  caster: boolean
  immunities: PetImmunities
  offense?: string[]
}

// PetHealthSnapshot is the latest `/pet health` reply: HP and every effect on
// the pet (beneficial and detrimental), with no durations.
export interface PetHealthSnapshot {
  at: string
  hp_percent: number
  spells: string[]
}

export interface PetState {
  mode: PetMode
  kind: PetKind
  pet_id?: number
  name?: string
  hp_percent?: number
  hp_source?: 'zeal' | 'pet_health'
  hp_at?: string
  stats?: PetStats
  // > 1 when the npc_types row is an ambiguous same-name pick.
  variants?: number
  charm_spell?: string
  health?: PetHealthSnapshot
  broke_at?: string
  swap_at?: string
  zone?: string
  zone_name?: string
  zeal_limited: boolean
  updated_at: string
}
