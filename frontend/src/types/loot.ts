// Mirrors backend internal/loot models. A loot entry is a "--Name has looted a
// X.--" line captured from the log.

export interface LootEntry {
  id: number
  character: string // local log owner
  player: string // the looter
  item: string
  zone: string
  npc: string // best-effort; currently always '' (not present in the log line)
  ts: number
  item_type: number // resolved server-side by item name; -1 if unmatched
}

export interface LootListResponse {
  loot: LootEntry[]
}

export interface LootMetaResponse {
  characters: string[]
  players: string[]
  zones: string[]
  active: string
}
