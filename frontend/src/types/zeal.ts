export interface InventoryEntry {
  location: string
  name: string
  id: number
  count: number
  slots: number // bag capacity; 0 for non-containers
  icon?: number // joined in by API from items.icon
}

export interface Inventory {
  character: string
  exported_at: string // ISO datetime
  entries: InventoryEntry[]
}

export interface Spellbook {
  character: string
  exported_at: string // ISO datetime
  spell_ids: number[]
}

export interface ZealInventoryResponse {
  inventory: Inventory | null
}

export interface ZealSpellbookResponse {
  spellbook: Spellbook | null
}

export interface AllInventoriesResponse {
  configured: boolean
  characters: Inventory[]
  shared_bank: InventoryEntry[]
}

export interface Spellset {
  name: string
  spell_ids: number[] // length 8, -1 = empty gem
}

export interface SpellsetFile {
  character: string
  exported_at: string // ISO datetime
  spellsets: Spellset[]
}

export interface ZealSpellsetsResponse {
  spellsets: SpellsetFile | null
}

export interface AllSpellsetsResponse {
  configured: boolean
  characters: SpellsetFile[]
}

export interface ZealInstallStatus {
  eq_path: string
  installed: boolean
  eqgame_present: boolean
  asi_path?: string
}
