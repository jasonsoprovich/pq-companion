export interface SpecialAbilityMeta {
  name: string
  description?: string
}

export interface EnumsCatalog {
  special_abilities: Record<string, SpecialAbilityMeta>
  tradeskills: Record<string, string>
  item_types: Record<string, string>
}
