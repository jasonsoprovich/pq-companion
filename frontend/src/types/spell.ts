export interface Spell {
  id: number
  name: string

  // Flavor text
  you_cast: string
  other_casts: string
  cast_on_you: string
  cast_on_other: string
  spell_fades: string

  // Timing (milliseconds)
  cast_time: number
  recovery_time: number
  recast_time: number

  // Duration
  buff_duration: number
  buff_duration_formula: number

  mana: number
  range: number
  aoe_range: number
  target_type: number
  resist_type: number
  skill: number

  // Parallel arrays, 12 slots each
  effect_ids: number[]
  effect_base_values: number[]
  effect_limit_values: number[]
  effect_max_values: number[]
  effect_formulas: number[]

  // 15 classes; 255 = cannot cast
  class_levels: number[]

  icon: number
  new_icon: number

  is_discipline: number
  suspendable: number
  no_dispell: number
  zone_type: number

  // Duplicate-name collapse (set by GET /spells/:id; absent in list views).
  // The dump ships several rows per spell name; lists show only the canonical
  // one. variant_ids are the other rows sharing this name (hidden from lists,
  // fetchable by id). canonical_id points back to the main row when this spell
  // is itself a variant.
  variant_ids?: number[]
  canonical_id?: number
}

// BuffStatDelta mirrors backend/internal/db/buffeffect.go. Returned by the
// /api/spells/stat-deltas batch endpoint — one entry per requested spell ID.
//
// `haste` is the raw melee haste % from SPA 11 (after subtracting the +100
// encoding) or SPA 119 (raw). The caller buckets v1/v2/v3 by source:
//   - worn equipment haste → v1
//   - cast buff/song/proc/clicky → v2 (or v3 for overhaste, e.g. Warsong
//     of the Vah Shir)
//
// `spell_haste` is SPA 127 raw % (cast-time reduction); the caller sums
// across spells and applies the 50% Quarm hardcap.
export interface BuffStatDelta {
  hp: number
  mana: number
  ac: number
  str: number
  sta: number
  agi: number
  dex: number
  wis: number
  int: number
  cha: number
  pr: number
  mr: number
  dr: number
  fr: number
  cr: number
  attack: number
  haste: number
  spell_haste: number
  regen: number
  mana_regen: number
  dmg_shield: number
}

export interface SpellItemRef {
  id: number
  name: string
  icon?: number
  effect_type?: string // "click" | "worn" | "proc" | "focus"
}

export interface SpellCrossRefs {
  scroll_items: SpellItemRef[]
  effect_items: SpellItemRef[]
}

// ── Shopping route ───────────────────────────────────────────────────────────

export interface ShoppingSpell {
  id: number
  name: string
}

export interface ShoppingVendor {
  id: number
  name: string
  x: number
  y: number
  spell_ids: number[]
}

export type ZoneAlignment = 'good' | 'neutral' | 'evil'

export interface ShoppingStop {
  zone_short: string
  zone_name: string
  reason: 'anchor' | 'greedy'
  alignment: ZoneAlignment
  spells: ShoppingSpell[]
  vendors: ShoppingVendor[]
}

export interface ShoppingRoute {
  stops: ShoppingStop[]
  unavailable: ShoppingSpell[]
  excluded_by_alignment: ShoppingSpell[]
  total_zones: number
  total_spells: number
}

export interface ShoppingRouteOptions {
  excludeAlignments?: ZoneAlignment[]
  startZone?: string
}
