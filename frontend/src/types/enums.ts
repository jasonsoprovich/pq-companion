export interface SpecialAbilityMeta {
  name: string
  description?: string
}

export interface EnumsCatalog {
  special_abilities: Record<string, SpecialAbilityMeta>
  tradeskills: Record<string, string>
  item_types: Record<string, string>
  npc_classes: Record<string, string>
  npc_races: Record<string, string>
  item_slot_bits: Record<string, string>
  item_class_bits: Record<string, string>
  item_race_bits: Record<string, string>
  bane_bodies: Record<string, string>
  bane_races: Record<string, string>
  zone_expansions: Record<string, string>
  zone_types: Record<string, string>
  char_classes: Record<string, string>
  char_races: Record<string, string>
  npc_body_types: Record<string, string>
  spell_effects: Record<string, string>
  spell_resists: Record<string, string>
  spell_targets: Record<string, string>
  spell_skills: Record<string, string>
  spell_type_filter: Record<string, string>
}
