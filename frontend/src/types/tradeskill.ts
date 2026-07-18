// Mirrors backend internal/tsplan (Plan/Stage) plus the enrichment the
// tradeskillPlan handler wraps around it (internal/api/characters.go
// tradeskillPlanResponse). Costs are in copper (see cost_unit).

// Recommended = the hand-curated path derived from community tradeskill
// guides (empty for disciplines not curated yet). Custom = the player's own
// saved build-your-own path, sorted by trivial server-side.
export type TradeskillMode = 'recommended' | 'custom'

// One leg of a leveling plan: grind `recipe` from from_skill up to to_skill.
export interface LevelingStage {
  from_skill: number
  to_skill: number
  recipe_id: number
  recipe: string
  trivial: number
  combines: number
  cost: number // copper for this leg (0 when unknown)
  cost_known: boolean
  // Combine success % (0-100) at from_skill — the worst case for this stage,
  // since it only rises as skill climbs toward trivial. A distinct roll from
  // the skill-up chance `combines` is derived from.
  success_chance_pct: number
  container?: string
  no_fail?: boolean
  sub_combine_recipe_ids?: number[]
  notes?: string[]
}

// A crafted intermediate a stage's recipe depends on. cross_tradeskill is true
// when it belongs to a different, skill-gated discipline (e.g. a Blacksmithing
// path needing a Brewing intermediate).
export interface SubCombineInfo {
  recipe_id: number
  name: string
  tradeskill: number
  tradeskill_name: string
  trivial: number
  cross_tradeskill: boolean
}

export interface TradeskillLevelingPlan {
  mode: TradeskillMode
  start_skill: number
  target_skill: number // as requested
  reached_skill: number // where the plan actually ends (may be below target)

  stages: LevelingStage[]
  total_combines: number
  total_cost: number // copper
  cost_complete: boolean // false = TotalCost is a lower bound (farmed components)
  warnings?: string[]

  // Echoed resolved inputs, so the UI can explain the numbers.
  tradeskill: number
  skill_name: string
  class_cap: number
  stat_name: string
  stat_source: string // "base+gear" or "base"
  trade_stat: number
  difficulty: number
  aa_reduce_pct: number
  cost_unit: string // "copper"

  // Detail for every sub-combine referenced by a stage, keyed by recipe id
  // (string). Absent when the plan has no crafted sub-components.
  sub_combines?: Record<string, SubCombineInfo>
}
