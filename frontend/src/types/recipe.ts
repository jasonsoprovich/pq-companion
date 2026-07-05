// Mirrors backend/internal/db/recipes.go.

export interface RecipeSummary {
  id: number
  name: string
  tradeskill: number
  trivial: number
  product_item_id: number
  product_icon: number
}

export interface RecipeEntry {
  item_id: number
  item_name: string
  icon: number
  role: 'container' | 'component' | 'product'
  count: number
  // Base vendor price (copper), present only when the item is merchant-sold.
  vendor_price?: number
  // True when the item is itself produced by at least one enabled recipe —
  // drives the ingredient drill-down affordance.
  craftable?: boolean
  // True when this container row is a combine-station type (Forge, Oven,
  // Enchanters Lexicon, …) rather than a specific inventory item. Such rows
  // have no icon or item-detail page.
  station?: boolean
}

export interface RecipeDetail extends RecipeSummary {
  skill_needed: number
  no_fail: boolean
  quest: boolean
  replace_container: boolean
  containers: RecipeEntry[]
  components: RecipeEntry[]
  products: RecipeEntry[]
}

export interface RecipeTradeskillCount {
  tradeskill: number
  count: number
}

// Mirrors backend db.TradeskillModifier. An item that boosts a tradeskill skill
// when worn; value is a percentage bonus. Only the single highest worn item
// applies (mods don't stack) — see internal/tradeskill.
export interface TradeskillModifier {
  item_id: number
  name: string
  icon: number
  value: number
}

// Mirrors backend internal/tradeskill.Result (GET /api/tradeskills/chance).
export interface TradeskillChance {
  raw_skill: number
  skill_mod: number  // item skill-mod % applied (max, not sum)
  eff_skill: number  // raw skill after the mod %, capped at 252
  success: number    // 0-100, one decimal
  failure: number
  at_trivial: boolean // eff skill >= trivial (no more skill-ups)
  no_fail: boolean
  // Raw skill (with no modifiers) needed to reach the 5% failure floor — the
  // canonical breakpoint. floor_reachable is false when it exceeds the 252 cap.
  floor_skill: number
  floor_reachable: boolean
  at_floor: boolean   // already at the 5% floor at this eff skill
}

// Mirrors backend tradeskillAAResponse (GET /api/characters/{id}/tradeskill-aa).
// The character's combine-failure-reduction Mastery AA for one discipline.
// applies is false for tradeskills the server has no such AA for (everything
// except Alchemy, Jewelry Making, and Make Poison). reduce_pct feeds the chance
// endpoint's `aa` param.
export interface TradeskillAA {
  applies: boolean
  name?: string
  eqmac_id?: number
  rank: number
  reduce_pct: number
}

// Mirrors backend internal/tradeskill.SkillUpResult
// (GET /api/characters/{id}/skillup-estimate). Estimated combines to raise a
// tradeskill toward a recipe's trivial.
export interface SkillUpEstimate {
  current_skill: number
  target_skill: number  // where skill-ups stop: min(trivial, class/level cap)
  trivial: number
  cap: number           // class/level cap (0 = unknown)
  difficulty: number
  trade_stat: number    // effective governing stat used
  stat_name: string     // attributes that drive it, e.g. "WIS/INT"
  points_to_go: number
  attempts_to_next: number    // combines to gain the next single point
  attempts_to_target: number  // combines to reach target_skill
  maxed: boolean        // already at/above target — no skill-ups from this recipe
  at_cap: boolean       // target is the class/level cap, not trivial
  impractical: boolean  // effectively can't skill up (no stat/difficulty)
}
