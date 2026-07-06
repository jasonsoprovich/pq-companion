// Mirrors backend internal/tsplan (Plan/Stage) plus the enrichment the
// tradeskillPlan handler wraps around it (internal/api/characters.go
// tradeskillPlanResponse). Costs are in copper (see cost_unit).

export type TradeskillObjective = 'fastest' | 'cheapest'

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
  container?: string
  no_fail?: boolean
  sub_combine_recipe_ids?: number[]
  notes?: string[]
}

export interface TradeskillLevelingPlan {
  objective: TradeskillObjective
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
}
