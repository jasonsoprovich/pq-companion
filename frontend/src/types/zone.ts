export interface Zone {
  id: number
  short_name: string
  long_name: string
  file_name: string
  zone_id_number: number
  safe_x: number
  safe_y: number
  safe_z: number
  min_level: number
  note: string
  // Non-empty when the server requires a zone flag to enter (zone.flag_needed;
  // the value is just the zone's short name, not a description).
  flag_needed: string
  outdoor: number
  hotzone: number
  can_levitate: number
  can_bind: number
  exp_mod: number
  expansion: number
  npc_level_min: number
  npc_level_max: number
  pull_limit: number
  graveyard?: ZoneGraveyard
}

export interface ZoneGraveyard {
  zone_id: number
  short_name: string
  long_name: string
  x: number
  y: number
  z: number
  timer_minutes: number
}

export interface ZoneConnection {
  zone_id: number
  short_name: string
  long_name: string
  expansion: number
}

export interface ZoneGroundSpawn {
  id: number
  item_id: number
  item_name: string
  name: string
  max_allowed: number
  respawn_timer: number
}

export interface ZoneForageItem {
  id: number
  item_id: number
  item_name: string
  chance: number
  level: number
}

// How likely an NPC is to be the one a spawn point rolls in a zone — its
// share of its spawn group(s), not a loot chance.
export interface ZoneNPCSpawn {
  npc_id: number
  min_pct: number
  max_pct: number
  points: number
  // Set when the NPC is in the zone's spawn data but can't currently spawn:
  // every entry is locked to another era, or its spawn weight is 0. The
  // pct/points fields are 0 then.
  inactive?: 'other_era' | 'zero_weight'
}

export interface ZoneDropItem {
  item_id: number
  item_name: string
  npc_id: number
  npc_name: string
  chance: number
}

export interface SearchResult<T> {
  items: T[]
  total: number
}
