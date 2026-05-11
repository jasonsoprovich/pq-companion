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
  outdoor: number
  hotzone: number
  can_levitate: number
  can_bind: number
  exp_mod: number
  expansion: number
  npc_level_min: number
  npc_level_max: number
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
