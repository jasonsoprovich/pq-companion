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

  // Dedicated invis-detection columns (authoritative source for codes 26/28).
  see_invis: number
  see_invis_undead: number

  spell_scale: number
  heal_scale: number
  exp_pct: number
}

export interface LootDropItem {
  item_id: number
  item_name: string
  item_icon?: number
  chance: number
  multiplier: number
}

export interface LootDrop {
  id: number
  name: string
  multiplier: number
  probability: number
  items: LootDropItem[]
}

export interface NPCLootTable {
  id: number
  name: string
  drops: LootDrop[]
  zone_wide_drops?: LootDrop[]
  zone_wide_label?: string
}

export interface NPCSpawnPoint {
  id: number
  zone: string
  zone_name: string
  x: number
  y: number
  z: number
  respawn_time: number
  fast_respawn_time: number
}

export interface SpawnGroupMember {
  npc_id: number
  name: string
  chance: number
}

export interface NPCSpawnGroup {
  id: number
  name: string
  respawn_time: number
  fast_respawn_time: number
  members: SpawnGroupMember[]
}

export interface NPCSpawns {
  spawn_points: NPCSpawnPoint[]
  spawn_groups: NPCSpawnGroup[]
}

export interface FactionHit {
  faction_id: number
  faction_name: string
  value: number
}

export interface NPCFaction {
  primary_faction_id: number
  primary_faction_name: string
  hits: FactionHit[]
}

export interface SearchResult<T> {
  items: T[]
  total: number
}

export interface NPCSpellEntry {
  spell_id: number
  spell_name: string
  type: number
  min_level: number
  max_level: number
  mana_cost: number
  recast_delay: number
  priority: number
  source_id: number
  source_name?: string
}

export interface NPCSpellProc {
  spell_id: number
  spell_name: string
  chance: number
}

export interface NPCSpells {
  npc_spells_id: number
  list_name: string
  attack_proc?: NPCSpellProc
  range_proc?: NPCSpellProc
  defensive_proc?: NPCSpellProc
  entries: NPCSpellEntry[]
  fail_recast: number
  engaged_b_self_chance: number
  engaged_b_other_chance: number
  engaged_d_chance: number
  pursue_d_chance: number
  idle_b_chance: number
}
