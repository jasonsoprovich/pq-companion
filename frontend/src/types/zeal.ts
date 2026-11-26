export interface InventoryEntry {
  location: string
  name: string
  id: number
  count: number
  slots: number // bag capacity; 0 for non-containers
  icon?: number // joined in by API from items.icon
  // Full charge capacity, set by the API only for rechargeable click items
  // (clickeffect > 0, maxcharges > 1). When present, `count` is the current
  // charge count, so charges-remaining reads as count / max_charges.
  max_charges?: number
  // Total base vendor value in copper (price × count for true stacks), set by
  // the API. Absent when the item has no value.
  value?: number
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
  // Spell names from exports that carry a name column (modern /outputfile
  // format). Used as a fallback match when the exported spell id has drifted
  // from the bundled quarm.db id. Absent on id-only exports.
  names?: string[]
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

// Bandolier slot order matches backend zeal.BandolierSlotCount: index
// 0=Primary, 1=Secondary, 2=Range, 3=Ammo. 0 = empty slot.
export interface BandolierSet {
  name: string
  item_ids: number[] // length 4, 0 = empty slot
}

export interface BandolierFile {
  character: string
  exported_at: string // ISO datetime
  sets: BandolierSet[]
}

export interface ZealBandolierResponse {
  bandolier: BandolierFile | null
}

export interface AllBandoliersResponse {
  configured: boolean
  characters: BandolierFile[]
}

// BandolierItem is one selectable, owned item from the slot picker.
export interface BandolierItem {
  id: number
  name: string
  icon: number
  slots: number
  item_type: number
}

export interface BandolierSlotItemsResponse {
  items: BandolierItem[]
}

// Per-character preferred bandolier storage bag from zeal.ini [Zeal_<Char>].
// slot: 0 = disabled, 1..8 = pack slot. found=false when never set.
export interface BandolierBagStatus {
  found: boolean
  slot: number
}

// In-game social macros from <Char>_pq.proj.ini [Socials]. 10 pages × 12
// buttons, up to 5 command lines each. color is the EQ user-color palette index
// (resolved to a swatch via TextColors), not an RGB value.
export interface MacroButton {
  page: number // 1..10
  button: number // 1..12
  name: string
  color: number
  lines: string[] // length 5, '' = unused line (positions significant)
}

export interface MacroFile {
  character: string
  exported_at: string // ISO datetime
  buttons: MacroButton[]
}

export interface ZealMacrosResponse {
  macros: MacroFile | null
}

// Zeal Page 10 social keybinds from eqclient.ini. Values are as EQ writes
// them ("SHIFT+0", "l"); '' = unbound.
export interface KeyBind {
  slot: number // 1..12 = SocialPage10Slot<N> = Page10Button<N>
  primary: string
  alternate: string
}

export interface Page10Binds {
  found: boolean // false when eqclient.ini is missing
  per_character: boolean // Zeal's PerCharKeybinds
  section: string // KeyMaps or KeyMaps_<Name>
  modified_at: string // eqclient.ini mtime, sent back on save
  social: KeyBind[]
  // Upper-cased key → every bind in the section using it, for clash warnings.
  bound_to: Record<string, string[]>
}

export interface AllMacrosResponse {
  configured: boolean
  characters: MacroFile[]
}

// One resolved entry of the EQ user-color palette (best-effort swatch source).
export interface MacroColor {
  index: number
  r: number
  g: number
  b: number
}

export interface TextColorsResponse {
  configured: boolean
  colors: MacroColor[]
}

export interface ZealInstallStatus {
  eq_path: string
  installed: boolean
  eqgame_present: boolean
  asi_path?: string
  version?: string
  min_version?: string
  version_ok: boolean
  // Latest Zeal release the backend has discovered via GitHub. Empty when
  // offline or the lookup hasn't completed yet.
  latest_version?: string
  // True when version is known and >= min, but strictly behind latest_version.
  // Surfaces a soft "update available" notice in Settings only; the red
  // version_ok=false banner takes precedence.
  update_available: boolean
  // True when zeal.ini was readable AND contained an ExportOnCamp key.
  // Treat false as "unknown" — don't warn until Zeal has written its config.
  export_on_camp_found: boolean
  // True when zeal.ini's ExportOnCamp setting is enabled. When found && !enabled,
  // pq-companion shows an amber banner: most character-data features depend on
  // Zeal writing export files at /camp.
  export_on_camp: boolean
}

export type ZealPipeState = 'idle' | 'connected' | 'disconnected' | 'unsupported'

export interface ZealPipeStatus {
  state: ZealPipeState
  pipe_name?: string
  pid?: number
  character?: string
  last_error?: string
  connected_at?: string // RFC3339 timestamp
}

// Zeal settings checklist (GET /api/zeal/settings). Only features the
// install's Zeal supports are listed; see backend internal/zeal/caps.go.
export type ZealCapState = 'working' | 'on' | 'off' | 'unknown' | 'not_seen'

export interface ZealCapRow {
  id: string
  label: string
  benefit: string
  command?: string
  state: ZealCapState
  value?: string
  hint?: string
  recommended: boolean
  pending: boolean
}

export interface ZealSettings {
  eq_path: string
  installed: boolean
  version?: string
  ini_readable: boolean
  pipe_connected: boolean
  pipe_matches: boolean
  capabilities: ZealCapRow[]
}
