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
