export interface Item {
  id: number
  name: string
  lore: string
  id_file: string
  item_class: number // 0=common, 1=container, 2=book
  item_type: number

  // Combat
  damage: number
  delay: number
  range: number
  ac: number
  bane_amt: number
  bane_body: number
  bane_race: number

  // Stats
  hp: number
  mana: number
  str: number
  sta: number
  agi: number
  dex: number
  wis: number
  int: number
  cha: number

  // Resists
  mr: number
  cr: number
  dr: number
  fr: number
  pr: number

  // Flags
  magic: number
  nodrop: number
  norent: number
  lore_flag: number

  // Equipment
  slots: number
  classes: number
  races: number
  weight: number
  size: number

  // Levels
  rec_level: number
  req_level: number

  // Effects
  click_effect: number
  click_name: string
  proc_effect: number
  proc_name: string
  worn_effect: number
  worn_name: string
  worn_level: number
  // Derived: effective worn haste % for SPA 11/119 worn effects (e.g. spell
  // 998 "Haste"). 0 / undefined when the worn effect is not a haste spell.
  worn_haste_pct?: number
  focus_effect: number
  focus_name: string
  // Limited-use charge count for click items. -1 (and occasionally 0) is the
  // sentinel for unlimited/permanent clickies; a positive value is a real cap.
  max_charges: number

  // Container
  bag_size: number
  bag_slots: number
  bag_type: number

  // Stack
  stackable: number
  stack_size: number

  price: number
  icon: number
  min_status: number

  // Duplicate-name collapse (set by GET /items/:id; absent in list views).
  // The dump ships several rows per item name; lists show only the canonical
  // one. variant_ids are the other rows sharing this name (hidden from lists,
  // fetchable by id). canonical_id points back to the main row when this item
  // is itself a variant.
  variant_ids?: number[]
  canonical_id?: number
}

export interface ItemSourceNPC {
  id: number
  name: string
  zone_name: string
  zone_short_name: string
  drop_rate?: number
}

export interface ItemForageZone {
  zone_short_name: string
  zone_name: string
  chance: number
}

export interface ItemGroundSpawnZone {
  zone_short_name: string
  zone_name: string
  name: string
  max_allowed: number
  respawn_timer: number
}

export interface ItemTradeskillEntry {
  recipe_id: number
  recipe_name: string
  tradeskill: number
  trivial: number
  role: 'product' | 'ingredient'
  count: number
}

export interface ItemSources {
  drops: ItemSourceNPC[]
  merchants: ItemSourceNPC[]
  forage_zones: ItemForageZone[]
  ground_spawns: ItemGroundSpawnZone[]
  tradeskills: ItemTradeskillEntry[]
}

// ItemRef is a lightweight item link (id + name) used in quest entries.
export interface ItemRef {
  id: number
  name: string
}

// ItemQuestRef is one quest's involvement with an item: the NPC and zone, plus
// the related items (turn-ins required for a reward, rewards granted for a
// turn-in). Derived from the Quarm quest scripts, not the item DB tables.
export interface ItemQuestRef {
  npc: string
  zone_short_name: string
  zone_name: string
  related_items?: ItemRef[]
}

export interface ItemQuests {
  rewarded_by: ItemQuestRef[]
  used_in: ItemQuestRef[]
}

export interface SearchResult<T> {
  items: T[]
  total: number
}
