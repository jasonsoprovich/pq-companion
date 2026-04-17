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
  focus_effect: number
  focus_name: string

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
}

export interface SearchResult<T> {
  items: T[]
  total: number
}
