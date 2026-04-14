export interface InventoryEntry {
  location: string
  name: string
  id: number
  count: number
  slots: number // bag capacity; 0 for non-containers
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
