export interface Spell {
  id: number
  name: string

  // Flavor text
  you_cast: string
  other_casts: string
  cast_on_you: string
  cast_on_other: string
  spell_fades: string

  // Timing (milliseconds)
  cast_time: number
  recovery_time: number
  recast_time: number

  // Duration
  buff_duration: number
  buff_duration_formula: number

  mana: number
  range: number
  aoe_range: number
  target_type: number
  resist_type: number
  skill: number

  // Parallel arrays, 12 slots each
  effect_ids: number[]
  effect_base_values: number[]
  effect_limit_values: number[]
  effect_max_values: number[]

  // 15 classes; 255 = cannot cast
  class_levels: number[]

  icon: number
  new_icon: number

  is_discipline: number
  suspendable: number
  no_dispell: number
}
