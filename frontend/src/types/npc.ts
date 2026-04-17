export interface NPC {
  id: number
  name: string
  last_name: string
  level: number
  race: number
  race_name: string
  class: number
  body_type: number

  hp: number
  mana: number
  min_dmg: number
  max_dmg: number
  attack_count: number

  // Resists / defense
  mr: number
  cr: number
  dr: number
  fr: number
  pr: number
  ac: number

  // Attributes
  str: number
  sta: number
  dex: number
  agi: number
  int: number
  wis: number
  cha: number

  // Behavior
  aggro_radius: number
  run_speed: number
  size: number
  raid_target: number
  rare_spawn: number

  // Links
  loottable_id: number
  merchant_id: number
  npc_spells_id: number
  npc_faction_id: number

  // Raw caret-delimited special abilities string
  special_abilities: string

  spell_scale: number
  heal_scale: number
  exp_pct: number
}

export interface SearchResult<T> {
  items: T[]
  total: number
}
